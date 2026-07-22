// Package tenanthttp resolves public widget traffic from the request host and
// applies the resolved tenant's channel CORS policy.
package tenanthttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/reinhlord/kontor/internal/tenants"
)

type hostResolver interface {
	TenantForHost(context.Context, string, string) (tenants.Tenant, error)
}

// PublicTenant derives the public tenant strictly from the tenant subdomain
// (for example salon-nord.kontor.example). It rejects unknown hosts before the
// widget reaches conversation data and verifies browser Origin against that
// tenant's stored widget configuration.
func PublicTenant(resolver hostResolver, hostSuffix string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if resolver == nil {
			writeProblem(w, http.StatusInternalServerError, "tenant resolution unavailable", "The tenant service is unavailable")
			return
		}
		tenant, err := resolver.TenantForHost(r.Context(), r.Host, hostSuffix)
		if err != nil {
			if errors.Is(err, tenants.ErrNotFound) {
				writeProblem(w, http.StatusNotFound, "tenant not found", "This host is not configured for Kontor")
				return
			}
			writeProblem(w, http.StatusInternalServerError, "tenant resolution unavailable", "The tenant service is unavailable")
			return
		}
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			requestOrigin, requestOriginErr := tenants.CanonicalWidgetOrigin(origin)
			allowedOrigin, allowedOriginErr := tenants.CanonicalWidgetOrigin(tenant.WidgetOrigin)
			if requestOriginErr != nil || allowedOriginErr != nil || !strings.EqualFold(requestOrigin, allowedOrigin) {
				writeProblem(w, http.StatusForbidden, "origin not allowed", "This browser origin is not configured for the tenant widget")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Last-Event-ID")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(tenants.WithTenant(r.Context(), tenant)))
	})
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "about:blank", "title": title, "status": status, "detail": detail,
	})
}
