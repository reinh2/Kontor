package tenants

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type tenantContextKey struct{}

// WithTenant attaches a host-resolved public tenant to a request context. It is
// used only after TenantForHost succeeded; callers must never construct this
// from a user-supplied query or body field.
func WithTenant(ctx context.Context, tenant Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenant)
}

func FromContext(ctx context.Context) (Tenant, bool) {
	tenant, ok := ctx.Value(tenantContextKey{}).(Tenant)
	return tenant, ok && tenant.ID != "" && tenant.Slug != ""
}

// TenantByID supports server-side runtime construction after a request has
// already been resolved to an immutable tenant ID.
func (s *Store) TenantByID(ctx context.Context, tenantID string) (Tenant, error) {
	if s == nil || s.pool == nil || tenantID == "" {
		return Tenant{}, ErrNotFound
	}
	var tenant Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT t.id::text,t.slug,t.name,t.timezone,t.currency,
		       COALESCE(c.widget_origin,''),t.created_at,t.updated_at
		FROM tenants t
		LEFT JOIN tenant_channels c ON c.tenant_id=t.id
		WHERE t.id=$1`, tenantID).Scan(
		&tenant.ID, &tenant.Slug, &tenant.Name, &tenant.Timezone, &tenant.Currency,
		&tenant.WidgetOrigin, &tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("tenants: get tenant by ID: %w", err)
	}
	return tenant, nil
}
