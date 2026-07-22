package tenanthttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reinhlord/kontor/internal/tenants"
)

type stubHostResolver struct {
	byHost map[string]tenants.Tenant
}

func (s stubHostResolver) TenantForHost(_ context.Context, host, suffix string) (tenants.Tenant, error) {
	if suffix != "kontor.example" {
		return tenants.Tenant{}, errors.New("unexpected suffix")
	}
	tenant, ok := s.byHost[host]
	if !ok {
		return tenants.Tenant{}, tenants.ErrNotFound
	}
	return tenant, nil
}

func TestPublicTenantScopesEachHostToItsOwnTenant(t *testing.T) {
	resolver := stubHostResolver{byHost: map[string]tenants.Tenant{
		"north.kontor.example": {ID: "tenant-north", Slug: "north", WidgetOrigin: "https://north.example/"},
		"south.kontor.example": {ID: "tenant-south", Slug: "south", WidgetOrigin: "https://south.example/"},
	}}
	handler := PublicTenant(resolver, "kontor.example", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := tenants.FromContext(r.Context())
		if !ok {
			t.Fatal("tenant missing from request context")
		}
		w.Header().Set("X-Tenant-ID", tenant.ID)
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, test := range []struct{ host, origin, tenantID string }{
		{"north.kontor.example", "https://north.example", "tenant-north"},
		{"south.kontor.example", "https://south.example", "tenant-south"},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/demo/conversations", nil)
		request.Host = test.host
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || response.Header().Get("X-Tenant-ID") != test.tenantID {
			t.Fatalf("host %q got status=%d tenant=%q", test.host, response.Code, response.Header().Get("X-Tenant-ID"))
		}
		if response.Header().Get("Access-Control-Allow-Origin") != test.origin {
			t.Fatalf("host %q allowed origin=%q", test.host, response.Header().Get("Access-Control-Allow-Origin"))
		}
	}
}

func TestPublicTenantRejectsUnknownHostAndWrongOriginBeforeNextHandler(t *testing.T) {
	resolver := stubHostResolver{byHost: map[string]tenants.Tenant{
		"north.kontor.example": {ID: "tenant-north", WidgetOrigin: "https://north.example/"},
	}}
	called := false
	handler := PublicTenant(resolver, "kontor.example", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	for _, test := range []struct {
		host, origin string
		want         int
	}{
		{"unknown.kontor.example", "https://unknown.example", http.StatusNotFound},
		{"north.kontor.example", "https://south.example", http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/demo/conversations", nil)
		request.Host = test.host
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.want || called {
			t.Fatalf("host %q status=%d called=%v", test.host, response.Code, called)
		}
	}
}

func TestPublicTenantRespondsToValidatedPreflightWithoutCallingNext(t *testing.T) {
	resolver := stubHostResolver{byHost: map[string]tenants.Tenant{
		"north.kontor.example": {ID: "tenant-north", WidgetOrigin: "https://north.example/"},
	}}
	called := false
	handler := PublicTenant(resolver, "kontor.example", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/demo/conversations", nil)
	request.Host = "north.kontor.example"
	request.Header.Set("Origin", "https://north.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "https://north.example" {
		t.Fatalf("allow origin = %q", response.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestPublicTenantCanonicalizesDefaultOriginPorts(t *testing.T) {
	resolver := stubHostResolver{byHost: map[string]tenants.Tenant{
		"north.kontor.example": {ID: "tenant-north", WidgetOrigin: "https://north.example:0443"},
	}}
	called := false
	handler := PublicTenant(resolver, "kontor.example", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, method := range []string{http.MethodPost, http.MethodOptions} {
		request := httptest.NewRequest(method, "/api/v1/demo/conversations", nil)
		request.Host = "north.kontor.example"
		request.Header.Set("Origin", "https://north.example")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "https://north.example" {
			t.Fatalf("method %s status=%d allowed origin=%q", method, response.Code, response.Header().Get("Access-Control-Allow-Origin"))
		}
	}
	if !called {
		t.Fatal("validated non-preflight request did not reach the tenant handler")
	}
}
