package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reinhlord/kontor/internal/platform/config"
	"github.com/reinhlord/kontor/internal/platform/httpx"
	"github.com/reinhlord/kontor/internal/platform/metrics"
)

func TestOperatorBranchIsOutsideWidgetCORS(t *testing.T) {
	public := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	operator := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := buildHTTPHandler(public, operator, httpx.NewRateLimiter(1000, 1000), "*")

	publicPreflight := httptest.NewRequest(http.MethodOptions, "/api/v1/demo/conversations", nil)
	publicPreflight.Header.Set("Origin", "https://foreign.example")
	publicPreflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(publicPreflightResponse, publicPreflight)
	if publicPreflightResponse.Code != http.StatusNoContent ||
		publicPreflightResponse.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("public preflight status=%d headers=%v", publicPreflightResponse.Code, publicPreflightResponse.Header())
	}

	operatorPreflight := httptest.NewRequest(http.MethodOptions, "/api/v1/operator/runs", nil)
	operatorPreflight.Header.Set("Origin", "https://foreign.example")
	operatorPreflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(operatorPreflightResponse, operatorPreflight)
	if operatorPreflightResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("operator preflight leaked widget CORS: %v", operatorPreflightResponse.Header())
	}

	operatorGet := httptest.NewRequest(http.MethodGet, "/api/v1/operator/runs", nil)
	operatorGetResponse := httptest.NewRecorder()
	handler.ServeHTTP(operatorGetResponse, operatorGet)
	if operatorGetResponse.Code != http.StatusOK || operatorGetResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("same-origin operator GET status=%d headers=%v", operatorGetResponse.Code, operatorGetResponse.Header())
	}
}

func TestDisabledOperatorAPIFallsThroughAsNotFound(t *testing.T) {
	public := http.NotFoundHandler()
	handler := buildHTTPHandler(public, nil, httpx.NewRateLimiter(1000, 1000), "*")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operator/runs", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("disabled operator status=%d", response.Code)
	}
}

func TestStage6TelegramRouteCapturesTenantSlug(t *testing.T) {
	webhook := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Tenant-Slug", r.PathValue("tenantSlug"))
		w.WriteHeader(http.StatusNoContent)
	})
	handler := buildStage6HTTPHandler(
		http.NotFoundHandler(),
		http.NotFoundHandler(),
		http.NotFoundHandler(),
		http.NotFoundHandler(),
		webhook,
		httpx.NewRateLimiter(1000, 1000),
	)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/v1/telegram/salon-nord", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("X-Tenant-Slug") != "salon-nord" {
		t.Fatalf("status=%d tenant slug=%q", response.Code, response.Header().Get("X-Tenant-Slug"))
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWithMetricsEndpointDisabledPassesThrough(t *testing.T) {
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-From", "app")
		w.WriteHeader(http.StatusTeapot)
	})
	registry := metrics.NewRegistry()
	handler := withMetricsEndpoint(app, registry, config.Metrics{Enabled: false}, discardLogger())

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusTeapot || response.Header().Get("X-From") != "app" {
		t.Fatalf("disabled metrics did not pass through: status=%d headers=%v", response.Code, response.Header())
	}
}

func TestWithMetricsEndpointEnabledServesExposition(t *testing.T) {
	app := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-From", "app")
		w.WriteHeader(http.StatusOK)
	})
	registry := metrics.NewRegistry()
	registry.NewCounter("probe_total", "Probe.").Inc()
	handler := withMetricsEndpoint(app, registry, config.Metrics{Enabled: true}, discardLogger())

	metricsResponse := httptest.NewRecorder()
	handler.ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metricsResponse.Code != http.StatusOK {
		t.Fatalf("metrics status=%d", metricsResponse.Code)
	}
	if metricsResponse.Header().Get("X-From") == "app" {
		t.Fatal("metrics request leaked into the application handler")
	}
	if !strings.Contains(metricsResponse.Body.String(), "probe_total 1") {
		t.Fatalf("exposition missing sample:\n%s", metricsResponse.Body.String())
	}

	appResponse := httptest.NewRecorder()
	handler.ServeHTTP(appResponse, httptest.NewRequest(http.MethodGet, "/api/v1/demo/conversations", nil))
	if appResponse.Header().Get("X-From") != "app" {
		t.Fatal("non-metrics request did not reach the application handler")
	}
}

func TestWithMetricsEndpointTokenGuard(t *testing.T) {
	registry := metrics.NewRegistry()
	registry.NewCounter("probe_total", "Probe.").Inc()
	const token = "metrics-scrape-token-0123456789"
	handler := withMetricsEndpoint(http.NotFoundHandler(), registry, config.Metrics{Enabled: true, Token: token}, discardLogger())

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"missing token", "", http.StatusUnauthorized},
		{"wrong token", "Bearer wrong-token-value-0123456789", http.StatusUnauthorized},
		{"correct token", "Bearer " + token, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tc.authHeader != "" {
				request.Header.Set("Authorization", tc.authHeader)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d", response.Code, tc.wantStatus)
			}
		})
	}
}
