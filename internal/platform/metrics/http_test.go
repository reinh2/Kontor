package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doRequest(t *testing.T, handler http.Handler, method, path string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

func TestHTTPMiddlewareRecordsCountDurationAndInFlight(t *testing.T) {
	r := NewRegistry()
	h := NewHTTP(r)
	handler := h.Middleware(HTTPConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	doRequest(t, handler, http.MethodPost, "/api/v1/demo/conversations")

	got := r.Gather()
	for _, want := range []string{
		`kontor_http_requests_total{method="POST",code="201"} 1`,
		`kontor_http_request_duration_seconds_count{method="POST"} 1`,
		"kontor_http_requests_in_flight 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestHTTPMiddlewareDefaultsStatusWhenOnlyWritten(t *testing.T) {
	r := NewRegistry()
	h := NewHTTP(r)
	handler := h.Middleware(HTTPConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	doRequest(t, handler, http.MethodGet, "/widget/v1/kontor.js")

	if !strings.Contains(r.Gather(), `kontor_http_requests_total{method="GET",code="200"} 1`) {
		t.Fatalf("implicit 200 not recorded:\n%s", r.Gather())
	}
}

func TestHTTPMiddlewareIgnoreSkipsInstrumentation(t *testing.T) {
	r := NewRegistry()
	h := NewHTTP(r)
	handler := h.Middleware(HTTPConfig{
		Ignore: func(req *http.Request) bool { return req.URL.Path == "/healthz" },
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	doRequest(t, handler, http.MethodGet, "/healthz")

	got := r.Gather()
	if strings.Contains(got, "kontor_http_requests_total{") {
		t.Fatalf("ignored request was counted:\n%s", got)
	}
	if !strings.Contains(got, "kontor_http_requests_in_flight 0") {
		t.Fatalf("in-flight gauge missing:\n%s", got)
	}
}

func TestHTTPMiddlewareLongLivedCountsButSkipsDuration(t *testing.T) {
	r := NewRegistry()
	h := NewHTTP(r)
	handler := h.Middleware(HTTPConfig{
		LongLived: func(_ *http.Request) bool { return true },
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	doRequest(t, handler, http.MethodGet, "/api/v1/demo/conversations/x/events")

	got := r.Gather()
	if !strings.Contains(got, `kontor_http_requests_total{method="GET",code="200"} 1`) {
		t.Fatalf("long-lived request was not counted:\n%s", got)
	}
	// No histogram child is created because Observe is never called.
	if strings.Contains(got, `kontor_http_request_duration_seconds_count{method="GET"}`) {
		t.Fatalf("long-lived request polluted the latency histogram:\n%s", got)
	}
}

// TestHTTPMiddlewarePreservesFlusher guards the SSE path: sse.go performs a
// direct w.(http.Flusher) assertion, so the wrapper must remain a Flusher.
func TestHTTPMiddlewarePreservesFlusher(t *testing.T) {
	r := NewRegistry()
	h := NewHTTP(r)
	var sawFlusher bool
	handler := h.Middleware(HTTPConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		sawFlusher = ok
		if ok {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("data: x\n\n"))
			flusher.Flush()
		}
	}))

	doRequest(t, handler, http.MethodGet, "/api/v1/demo/conversations/x/events")

	if !sawFlusher {
		t.Fatal("wrapped ResponseWriter is not an http.Flusher; SSE would break")
	}
}

// TestHTTPMiddlewarePreservesResponseController guards the write-deadline path:
// main.go relies on http.NewResponseController(w) reaching the real writer
// through Unwrap.
func TestHTTPMiddlewarePreservesResponseController(t *testing.T) {
	r := NewRegistry()
	h := NewHTTP(r)
	var flushErr error
	handler := h.Middleware(HTTPConfig{}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		flushErr = http.NewResponseController(w).Flush()
	}))

	doRequest(t, handler, http.MethodGet, "/api/v1/demo/conversations/x/events")

	if flushErr != nil {
		t.Fatalf("ResponseController.Flush through wrapper failed: %v", flushErr)
	}
}

func TestHTTPRateLimitedCounter(t *testing.T) {
	r := NewRegistry()
	h := NewHTTP(r)
	h.RateLimited()
	h.RateLimited()
	if !strings.Contains(r.Gather(), "kontor_http_rate_limited_total 2") {
		t.Fatalf("rate-limited counter wrong:\n%s", r.Gather())
	}
}
