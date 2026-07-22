package metrics

import (
	"net/http"
	"strconv"
	"time"
)

// HTTP bundles the server-side request metrics and the middleware that records
// them. Construct it once with NewHTTP and wrap the top-level handler.
type HTTP struct {
	requests    *CounterVec
	duration    *HistogramVec
	inFlight    *Gauge
	rateLimited *Counter
}

// NewHTTP registers the standard HTTP server metrics on r:
//
//	kontor_http_requests_total{method,code}      counter
//	kontor_http_request_duration_seconds{method} histogram
//	kontor_http_requests_in_flight               gauge
//	kontor_http_rate_limited_total               counter
func NewHTTP(r *Registry) *HTTP {
	return &HTTP{
		requests:    r.NewCounterVec("kontor_http_requests_total", "Total HTTP requests by method and response status code.", "method", "code"),
		duration:    r.NewHistogramVec("kontor_http_request_duration_seconds", "HTTP request latency in seconds by method.", DefBuckets, "method"),
		inFlight:    r.NewGauge("kontor_http_requests_in_flight", "In-flight HTTP requests currently being served."),
		rateLimited: r.NewCounter("kontor_http_rate_limited_total", "Requests rejected by the edge rate limiter."),
	}
}

// RateLimited increments the rate-limit rejection counter. Wire it to the
// limiter's rejection hook so overload is observable independently of status.
func (h *HTTP) RateLimited() { h.rateLimited.Inc() }

// HTTPConfig tunes which requests the middleware instruments.
type HTTPConfig struct {
	// Ignore skips instrumentation entirely for matching requests. Use it for
	// liveness and readiness probes, whose high, uniform frequency would
	// otherwise drown out real traffic in the counters.
	Ignore func(*http.Request) bool
	// LongLived still counts a matching request but omits it from the latency
	// histogram. Use it for SSE streams, whose multi-minute lifetimes would
	// pin every observation in the +Inf bucket and make the sum meaningless.
	LongLived func(*http.Request) bool
}

// Middleware records request count, in-flight gauge, and (unless the request is
// long-lived) latency for every request handled by next.
func (h *HTTP) Middleware(cfg HTTPConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.Ignore != nil && cfg.Ignore(r) {
			next.ServeHTTP(w, r)
			return
		}
		longLived := cfg.LongLived != nil && cfg.LongLived(r)

		h.inFlight.Inc()
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			h.inFlight.Dec()
			h.requests.With(r.Method, strconv.Itoa(recorder.status)).Inc()
			if !longLived {
				h.duration.With(r.Method).Observe(time.Since(start).Seconds())
			}
		}()
		next.ServeHTTP(recorder, r)
	})
}

// statusRecorder captures the response status code while remaining transparent
// to the handlers it wraps. It implements Flush so the SSE handler's
// w.(http.Flusher) assertion still succeeds, and Unwrap so
// http.ResponseController reaches the underlying writer for write deadlines.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.written = true
	return s.ResponseWriter.Write(b)
}

// Unwrap exposes the wrapped writer to http.ResponseController.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Flush forwards to the underlying flusher when present, keeping SSE streaming
// intact through the wrapper.
func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
