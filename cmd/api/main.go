package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/bootstrap"
	"github.com/reinhlord/kontor/internal/channels/demohttp"
	"github.com/reinhlord/kontor/internal/channels/onboardinghttp"
	"github.com/reinhlord/kontor/internal/channels/operatorhttp"
	"github.com/reinhlord/kontor/internal/channels/telegram"
	"github.com/reinhlord/kontor/internal/channels/tenanthttp"
	"github.com/reinhlord/kontor/internal/demo"
	"github.com/reinhlord/kontor/internal/identity"
	"github.com/reinhlord/kontor/internal/platform/config"
	"github.com/reinhlord/kontor/internal/platform/database"
	"github.com/reinhlord/kontor/internal/platform/httpx"
	"github.com/reinhlord/kontor/internal/platform/logging"
	"github.com/reinhlord/kontor/internal/platform/metrics"
	"github.com/reinhlord/kontor/internal/tenants"
)

// version is the build version stamped via -ldflags "-X main.version=...".
// It defaults to "dev" for local and test builds and surfaces in the
// kontor_build_info metric.
var version = "dev"

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

const legacyComposeDemoWidgetOrigin = "http://salon-nord.localhost:8080"

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
	if cfg.DemoMode {
		if err := demo.EnsureFixedTenant(ctx, pool, demo.Tenant{
			ID: cfg.Tenant.ID, Slug: cfg.Tenant.Slug, Name: cfg.Tenant.Name,
			Timezone: cfg.Tenant.Timezone, Currency: cfg.Tenant.Currency,
		}); err != nil {
			return err
		}
		if err := demo.SeedCatalog(ctx, pool, cfg.Tenant.ID, cfg.Tenant.Currency); err != nil {
			return err
		}
	}

	tenantStore, err := tenants.NewStore(pool, tenants.Config{ChannelEncryptionKey: cfg.Tenancy.ChannelEncryptionKey})
	if err != nil {
		return err
	}
	identityStore, err := identity.NewStore(pool, identity.Config{SessionTTL: cfg.Operator.SessionTTL})
	if err != nil {
		return err
	}
	legacyBootstrap, err := config.LoadLegacyTenantBootstrap(cfg.DemoMode)
	if err != nil {
		return err
	}
	if legacyBootstrap.Enabled {
		bootstrapCtx, cancelBootstrap := context.WithTimeout(ctx, 10*time.Second)
		defer cancelBootstrap()
		result, err := tenantStore.BootstrapLegacyTenant(bootstrapCtx, tenants.LegacyBootstrapInput{
			TenantID: legacyBootstrap.TenantID, TenantSlug: legacyBootstrap.TenantSlug,
			WidgetOrigin: legacyBootstrap.WidgetOrigin,
			Owner: tenants.OwnerInput{
				Email: legacyBootstrap.OwnerEmail, DisplayName: legacyBootstrap.OwnerDisplayName,
				Password: legacyBootstrap.OwnerPassword,
			},
			Telegram: tenants.ChannelConfig{
				TelegramEnabled:       cfg.Telegram.Enabled(),
				TelegramBotToken:      cfg.Telegram.BotToken,
				TelegramWebhookSecret: cfg.Telegram.WebhookSecret,
			},
		})
		if err != nil {
			return fmt.Errorf("bootstrap legacy Stage 6 tenant: %w", err)
		}
		logger.Info("legacy Stage 6 tenant bootstrap completed", "tenant_id", legacyBootstrap.TenantID, "applied", result.Applied)
	}
	if cfg.DemoMode {
		channels, err := tenantStore.ChannelConfig(ctx, cfg.Tenant.ID)
		if err != nil {
			return err
		}
		shouldSetWidgetOrigin := channels.WidgetOrigin == "" ||
			(channels.WidgetOrigin == legacyComposeDemoWidgetOrigin && cfg.Demo.WidgetOrigin != legacyComposeDemoWidgetOrigin)
		if shouldSetWidgetOrigin {
			if err := tenantStore.UpdateChannels(ctx, cfg.Tenant.ID, tenants.ChannelConfig{WidgetOrigin: cfg.Demo.WidgetOrigin}); err != nil {
				return err
			}
		}
		if _, err := identityStore.EnsureOperator(ctx, identity.CreateOperatorInput{
			TenantID: cfg.Tenant.ID, Email: cfg.Demo.OwnerEmail, DisplayName: "Demo owner",
			Password: cfg.Demo.OwnerPassword, Role: identity.RoleOwner,
		}); err != nil {
			return err
		}
	}

	runtime, err := bootstrap.NewRuntime(cfg, pool, tenantStore, logger)
	if err != nil {
		return err
	}
	publicRoutes, err := demohttp.NewMultiTenant(runtime, pool, logger)
	if err != nil {
		return err
	}
	onboardingHandler, err := onboardinghttp.New(tenantStore, identityStore)
	if err != nil {
		return err
	}
	operatorStore, err := operatorhttp.NewMultiTenantPostgreSQL(pool)
	if err != nil {
		return err
	}
	operatorHandler, err := operatorhttp.New(operatorhttp.Config{Authenticator: identityStore}, operatorStore, logger)
	if err != nil {
		return err
	}
	webhook, err := telegram.NewMultiTenantWebhook(pool, runtime, tenantStore, cfg.Telegram.APIBaseURL, int(cfg.Agent.ConversationTokenBudget), logger)
	if err != nil {
		return err
	}

	limiter := httpx.NewRateLimiter(cfg.HTTP.RateLimitPerMinute, cfg.HTTP.RateLimitBurst)
	limiter.SetTrustForwardedFor(cfg.HTTP.TrustForwardedFor)
	registry := metrics.NewRegistry()
	metrics.RegisterProcessInfo(registry, version)
	httpMetrics := metrics.NewHTTP(registry)
	limiter.SetOnReject(httpMetrics.RateLimited)

	tenantPublic := tenanthttp.PublicTenant(tenantStore, cfg.Tenancy.HostSuffix, publicRoutes)
	appHandler := buildStage6HTTPHandler(publicRoutes, tenantPublic, operatorHandler, onboardingHandler, webhook, limiter)
	instrumented := httpMetrics.Middleware(metrics.HTTPConfig{
		Ignore:    isHealthProbe,
		LongLived: isEventStream,
	}, appHandler)
	handler := withMetricsEndpoint(instrumented, registry, cfg.Metrics, logger)

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
		logger.Info("api listening", "addr", cfg.HTTPAddr, "tenant_host_suffix", cfg.Tenancy.HostSuffix)
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

// buildStage6HTTPHandler keeps identity/onboarding, widget, webhook, and
// operator routes on separate edges. In particular, widget CORS never leaks to
// operator or provisioning routes, and host resolution occurs before any
// customer conversation is read.
func buildStage6HTTPHandler(unscopedRoutes, tenantPublic, operatorHandler, onboardingHandler, webhook http.Handler, limiter *httpx.RateLimiter) http.Handler {
	routes := http.NewServeMux()
	edge := limiter.Middleware
	routes.Handle("/api/v1/tenants", edge(onboardingHandler))
	routes.Handle("/api/v1/operator/login", edge(onboardingHandler))
	routes.Handle("/api/v1/operator/logout", edge(onboardingHandler))
	routes.Handle("/api/v1/operator/channels", edge(onboardingHandler))
	routes.Handle("/api/v1/operator/operators", edge(onboardingHandler))
	routes.Handle("/api/v1/operator/catalog/", edge(onboardingHandler))
	routes.Handle("/api/v1/operator/staff", edge(onboardingHandler))
	routes.Handle("/api/v1/operator/staff/", edge(onboardingHandler))
	routes.Handle("/api/v1/operator", edge(operatorHandler))
	routes.Handle("/api/v1/operator/", edge(operatorHandler))
	routes.Handle("POST /webhooks/v1/telegram/{tenantSlug}", edge(webhook))
	routes.Handle("/api/v1/demo/", edge(tenantPublic))
	routes.Handle("/widget/", edge(tenantPublic))
	routes.Handle("/", edge(unscopedRoutes))
	return routes
}

// withMetricsEndpoint mounts the Prometheus exposition endpoint alongside the
// instrumented application handler. The endpoint is opt-in and deliberately
// sits outside the rate limiter and the instrumentation wrapper. When a token
// is configured it is required as a bearer credential, because /metrics is
// outside the operator-session and widget-CORS edges and can expose internal
// signals to anyone who can reach it.
func withMetricsEndpoint(app http.Handler, registry *metrics.Registry, cfg config.Metrics, logger *slog.Logger) http.Handler {
	if !cfg.Enabled {
		return app
	}
	logger.Info("metrics endpoint enabled", "path", "/metrics", "token_required", cfg.Token != "")
	routes := http.NewServeMux()
	routes.Handle("GET /metrics", guardMetricsToken(cfg.Token, registry.Handler()))
	routes.Handle("/", app)
	return routes
}

// guardMetricsToken requires a matching bearer token when one is configured. An
// empty token leaves the endpoint open, which is only safe when the scrape port
// is network-restricted; the startup log records which mode is active.
func guardMetricsToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	expected := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), expected) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isHealthProbe matches the liveness and readiness endpoints, which are skipped
// by instrumentation so their uniform, high-frequency traffic does not swamp
// the request counters.
func isHealthProbe(r *http.Request) bool {
	return r.URL.Path == "/healthz" || r.URL.Path == "/readyz"
}

// isEventStream matches the SSE endpoint (GET .../conversations/{id}/events).
// Its connections live for minutes, so they are counted but excluded from the
// latency histogram, which would otherwise report every stream as a +Inf outlier.
func isEventStream(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		strings.HasPrefix(r.URL.Path, "/api/v1/demo/conversations/") &&
		strings.HasSuffix(r.URL.Path, "/events")
}

// buildHTTPHandler is retained for the Stage 5 route-boundary unit test. The
// Stage 6 server uses buildStage6HTTPHandler above.
func buildHTTPHandler(publicRoutes, operatorHandler http.Handler, limiter *httpx.RateLimiter, allowedOrigin string) http.Handler {
	routes := http.NewServeMux()
	if operatorHandler != nil {
		operatorEdge := limiter.Middleware(operatorHandler)
		routes.Handle("/api/v1/operator", operatorEdge)
		routes.Handle("/api/v1/operator/", operatorEdge)
	}
	routes.Handle("/", httpx.CORS(allowedOrigin, limiter.Middleware(publicRoutes)))
	return routes
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
