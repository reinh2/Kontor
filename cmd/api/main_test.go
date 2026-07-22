package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reinhlord/kontor/internal/platform/httpx"
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
