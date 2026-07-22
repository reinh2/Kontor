package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/agenttrace"
	"github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/platform/config"
	"github.com/reinhlord/kontor/internal/tenants"
)

// Runtime lazily builds a fully tenant-scoped agent graph after the HTTP host
// or Telegram path has resolved a tenant. Components retain their tenant ID;
// the map is never indexed by caller-provided data without a database lookup.
type Runtime struct {
	config  config.Config
	pool    *pgxpool.Pool
	logger  *slog.Logger
	tenants *tenants.Store

	mu         sync.Mutex
	components map[string]*Components
}

func NewRuntime(cfg config.Config, pool *pgxpool.Pool, tenantStore *tenants.Store, logger *slog.Logger) (*Runtime, error) {
	if pool == nil || tenantStore == nil {
		return nil, errors.New("bootstrap runtime: PostgreSQL pool and tenant store are required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{config: cfg, pool: pool, logger: logger, tenants: tenantStore, components: map[string]*Components{}}, nil
}

func (r *Runtime) componentsFor(ctx context.Context, tenantID string) (*Components, error) {
	if r == nil || tenantID == "" {
		return nil, errors.New("bootstrap runtime: tenant ID is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if components := r.components[tenantID]; components != nil {
		return components, nil
	}
	tenant, err := r.tenants.TenantByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	cfg := r.config
	cfg.Tenant = config.Tenant{
		ID: tenant.ID, Slug: tenant.Slug, Name: tenant.Name,
		Timezone: tenant.Timezone, Currency: tenant.Currency,
	}
	components, err := Build(ctx, cfg, r.pool, r.logger)
	if err != nil {
		return nil, err
	}
	r.components[tenantID] = components
	return components, nil
}

// ApplicationFor is intentionally a narrow cross-channel boundary: a caller
// can obtain services for the resolved tenant but cannot override their scope.
func (r *Runtime) ApplicationFor(ctx context.Context, tenantID string) (*app.Service, *conversations.Store, *agenttrace.Store, error) {
	components, err := r.componentsFor(ctx, tenantID)
	if err != nil {
		return nil, nil, nil, err
	}
	return components.Application, components.Conversations, components.Trace, nil
}

// TelegramFor exposes only the two tenant-scoped services the webhook needs.
func (r *Runtime) TelegramFor(ctx context.Context, tenantID string) (*app.Service, *conversations.Store, error) {
	components, err := r.componentsFor(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	return components.Application, components.Conversations, nil
}