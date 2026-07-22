// Package httpx provides the channel-edge HTTP middleware: CORS for the
// embeddable widget and per-client rate limiting in front of the bounded
// turn-admission queue.
package httpx

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CORS allows the configured origin to call the widget API from a browser.
// allowedOrigin is either a single origin or "*". The API authenticates with
// bearer capabilities, never cookies, so no Allow-Credentials is ever set and
// the wildcard stays safe for the zero-key demo.
func CORS(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowedOrigin == "*" || strings.EqualFold(origin, allowedOrigin)) {
			header := w.Header()
			if allowedOrigin == "*" {
				header.Set("Access-Control-Allow-Origin", "*")
			} else {
				header.Set("Access-Control-Allow-Origin", allowedOrigin)
				header.Add("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				header.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Last-Event-ID")
				header.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimiter is a token-bucket limiter keyed by client IP. It protects the
// admission queue from a single noisy client; the queue itself remains the
// backstop for aggregate overload.
type RateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*bucket
	perMinute   float64
	burst       float64
	lastCleanup time.Time
	now         func() time.Time
}

type bucket struct {
	tokens float64
	seen   time.Time
}

func NewRateLimiter(perMinute, burst int) *RateLimiter {
	if perMinute < 1 {
		perMinute = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &RateLimiter{
		buckets:   make(map[string]*bucket),
		perMinute: float64(perMinute),
		burst:     float64(burst),
		now:       time.Now,
	}
}

// Middleware enforces the limit for every request except liveness and
// readiness probes. Rejections are controlled 429 responses with Retry-After,
// mirroring how the deeper admission queue signals overload with 503.
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		if !l.allow(clientKey(r)) {
			w.Header().Set("Retry-After", "5")
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"about:blank","title":"rate limited","status":429,"detail":"Too many requests from this client; retry shortly"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *RateLimiter) allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastCleanup) > time.Minute {
		for existing, state := range l.buckets {
			if now.Sub(state.seen) > 10*time.Minute {
				delete(l.buckets, existing)
			}
		}
		l.lastCleanup = now
	}

	state, found := l.buckets[key]
	if !found {
		state = &bucket{tokens: l.burst}
		l.buckets[key] = state
	} else {
		elapsed := now.Sub(state.seen).Minutes()
		state.tokens += elapsed * l.perMinute
		if state.tokens > l.burst {
			state.tokens = l.burst
		}
	}
	state.seen = now
	if state.tokens < 1 {
		return false
	}
	state.tokens--
	return true
}

// clientKey prefers the first X-Forwarded-For hop because the container
// always sits behind the bundled nginx proxy; a direct caller falls back to
// the socket address.
func clientKey(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
