package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type principalContextKey struct{}

// SessionValidator is the small session boundary used by HTTP middleware and
// makes authentication behavior independently testable from PostgreSQL.
type SessionValidator interface {
	ValidateSession(context.Context, string) (Principal, error)
}

// WithPrincipal attaches a validated, server-derived operator principal to a
// request context. It is public for trusted internal adapters only; external
// HTTP input must always go through Authenticate.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns the authenticated identity previously set by
// Authenticate. Missing and malformed principals are intentionally equivalent.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	if !ok || principal.TenantID == "" || principal.OperatorID == "" || !validRole(principal.Role) {
		return Principal{}, false
	}
	return principal, true
}

// Authenticate validates an opaque operator session and places its tenant
// identity in the request context. No caller-controlled tenant identifier is
// accepted for protected routes.
func Authenticate(validator SessionValidator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if validator == nil {
			writeAuthProblem(w, http.StatusInternalServerError, "authentication unavailable", "The operator service is unavailable")
			return
		}
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeUnauthorized(w)
			return
		}
		principal, err := validator.ValidateSession(r.Context(), token)
		if err != nil {
			if errors.Is(err, ErrSessionInvalid) {
				writeUnauthorized(w)
				return
			}
			writeAuthProblem(w, http.StatusInternalServerError, "authentication unavailable", "The operator service is unavailable")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

// RequireRole restricts a route after Authenticate has established a
// principal. A missing principal is rejected rather than being trusted.
func RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			writeUnauthorized(w)
			return
		}
		if principal.Role != role {
			writeAuthProblem(w, http.StatusForbidden, "forbidden", "Your operator role is not authorized for this action")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(header string) (string, bool) {
	if len(header) > 1024 {
		return "", false
	}
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || fields[1] == "" {
		return "", false
	}
	return fields[1], true
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="kontor-operator"`)
	writeAuthProblem(w, http.StatusUnauthorized, "unauthorized", "A valid operator session is required")
}

func writeAuthProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "about:blank", "title": title, "status": status, "detail": detail,
	})
}
