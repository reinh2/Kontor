package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/bootstrap"
	"github.com/reinhlord/kontor/internal/channels/demohttp"
	"github.com/reinhlord/kontor/internal/channels/telegram"
	"github.com/reinhlord/kontor/internal/demo"
	"github.com/reinhlord/kontor/internal/platform/config"
	"github.com/reinhlord/kontor/internal/platform/database"
	"github.com/reinhlord/kontor/internal/platform/httpx"
	"github.com/reinhlord/kontor/internal/platform/logging"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "healthcheck" {
		if err := healthcheck(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.Environment)
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.ApplyMigrations(ctx, pool, migrations.Files, "."); err != nil {
		return err
	}
	if err := demo.EnsureFixedTenant(ctx, pool, demo.Tenant{
		ID: cfg.Tenant.ID, Slug: cfg.Tenant.Slug, Name: cfg.Tenant.Name, Timezone: cfg.Tenant.Timezone,
	}); err != nil {
		return err
	}
	if cfg.DemoMode {
		if err := demo.SeedCatalog(ctx, pool, cfg.Tenant.ID); err != nil {
			return err
		}
	}

	components, err := bootstrap.Build(ctx, cfg, pool, logger)
	if err != nil {
		return err
	}
	root := http.NewServeMux()
	root.Handle("/", demohttp.New(components.Application, components.Trace, pool, logger))
	if cfg.Telegram.Enabled() {
		sender, err := telegram.NewBotAPISender(telegram.BotAPIConfig{
			Token: cfg.Telegram.BotToken, BaseURL: cfg.Telegram.APIBaseURL,
		})
		if err != nil {
			return err
		}
		webhook, err := telegram.NewWebhook(telegram.Config{
			TenantID:      cfg.Tenant.ID,
			WebhookSecret: cfg.Telegram.WebhookSecret,
			TokenBudget:   int(cfg.Agent.ConversationTokenBudget),
		}, pool, components.Application, components.Conversations, sender, logger)
		if err != nil {
			return err
		}
		root.Handle("POST /webhooks/v1/telegram", webhook)
		logger.Info("telegram webhook channel enabled")
	}
	limiter := httpx.NewRateLimiter(cfg.HTTP.RateLimitPerMinute, cfg.HTTP.RateLimitBurst)
	handler := httpx.CORS(cfg.HTTP.AllowedOrigin, limiter.Middleware(root))

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		// SSE streams outlive any static write timeout; the stream handler
		// enforces its own per-write deadlines through ResponseController.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("api shutdown requested")
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful HTTP shutdown: %w", err)
	}
	return nil
}

func healthcheck(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: HTTP %d", response.StatusCode)
	}
	return nil
}
