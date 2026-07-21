package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/demo"
	"github.com/reinhlord/kontor/internal/platform/config"
	"github.com/reinhlord/kontor/internal/platform/database"
	"github.com/reinhlord/kontor/internal/platform/logging"
)

func main() {
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

	logger.Info("worker ready", "stage", 1, "note", "durable reminder jobs arrive in Stage 3")
	<-ctx.Done()
	logger.Info("worker drained")
	return nil
}
