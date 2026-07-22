package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCORSPreflightAndSimpleRequests(t *testing.T) {
	handler := CORS("https://customer.example", okHandler())

	preflight := httptest.NewRequest(http.MethodOptions, "/api/v1/demo/conversations", nil)
	preflight.Header.Set("Origin", "https://customer.example")
	preflightResponse := httptest.NewRecorder()
	handler.ServeHTTP(preflightResponse, preflight)
	if preflightResponse.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d", preflightResponse.Code)
	}
	if got := preflightResponse.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization, Last-Event-ID" {
		t.Fatalf("allow headers=%q", got)
	}

	foreign := httptest.NewRequest(http.MethodPost, "/api/v1/demo/conversations", nil)
	foreign.Header.Set("Origin", "https://attacker.example")
	foreignResponse := httptest.NewRecorder()
	handler.ServeHTTP(foreignResponse, foreign)
	if foreignResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("foreign origin received CORS approval")
	}

	wildcard := CORS("*", okHandler())
	simple := httptest.NewRequest(http.MethodGet, "/widget/v1/kontor.js", nil)
	simple.Header.Set("Origin", "https://anyone.example")
	simpleResponse := httptest.NewRecorder()
	wildcard.ServeHTTP(simpleResponse, simple)
	if simpleResponse.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("wildcard origin was not applied")
	}
	if simpleResponse.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("credentials must never be allowed")
	}
}

func TestRateLimiterAllowsBurstThenRejectsThenRefills(t *testing.T) {
	limiter := NewRateLimiter(60, 3)
	current := time.Unix(1_700_000_000, 0)
	limiter.now = func() time.Time { return current }
	handler := limiter.Middleware(okHandler())

	request := func(path string) int {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		r.RemoteAddr = "203.0.113.7:5511"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}

	for i := 0; i < 3; i++ {
		if code := request("/api/v1/demo/conversations"); code != http.StatusOK {
			t.Fatalf("burst request %d status=%d", i+1, code)
		}
	}
	if code := request("/api/v1/demo/conversations"); code != http.StatusTooManyRequests {
		t.Fatalf("exhausted bucket status=%d, want 429", code)
	}
	if code := request("/healthz"); code != http.StatusOK {
		t.Fatalf("health probe was rate limited: %d", code)
	}

	current = current.Add(2 * time.Second) // 60/min → 2 tokens back
	if code := request("/api/v1/demo/conversations"); code != http.StatusOK {
		t.Fatalf("refilled bucket status=%d", code)
	}
}

func TestRateLimiterSeparatesClientsAndPrefersForwardedFor(t *testing.T) {
	limiter := NewRateLimiter(60, 1)
	handler := limiter.Middleware(okHandler())

	send := func(forwardedFor, remote string) int {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/demo/conversations", nil)
		r.RemoteAddr = remote
		if forwardedFor != "" {
			r.Header.Set("X-Forwarded-For", forwardedFor)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}

	if send("198.51.100.1", "10.0.0.2:1000") != http.StatusOK {
		t.Fatal("first client rejected")
	}
	if send("198.51.100.1", "10.0.0.3:1000") != http.StatusTooManyRequests {
		t.Fatal("same forwarded client behind a different proxy hop was not limited")
	}
	if send("198.51.100.2", "10.0.0.2:1000") != http.StatusOK {
		t.Fatal("distinct forwarded client was limited")
	}
}
