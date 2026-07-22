package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubSessionValidator struct {
	principal Principal
	err       error
	token     string
}

func (s *stubSessionValidator) ValidateSession(_ context.Context, token string) (Principal, error) {
	s.token = token
	if s.err != nil {
		return Principal{}, s.err
	}
	return s.principal, nil
}

func TestAuthenticateAddsOnlyServerValidatedPrincipal(t *testing.T) {
	validator := &stubSessionValidator{principal: Principal{
		TenantID: "tenant-a", OperatorID: "operator-a", Role: RoleOwner,
	}}
	called := false
	handler := Authenticate(validator, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		principal, ok := PrincipalFromContext(r.Context())
		if !ok || principal.TenantID != "tenant-a" || principal.OperatorID != "operator-a" {
			t.Fatalf("principal = %#v, ok=%v", principal, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/operator/runs", nil)
	request.Header.Set("Authorization", "Bearer opaque-session-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
	if validator.token != "opaque-session-token" {
		t.Fatalf("validator token = %q", validator.token)
	}
}

func TestAuthenticateRejectsInvalidSessionsBeforeHandler(t *testing.T) {
	validator := &stubSessionValidator{err: ErrSessionInvalid}
	called := false
	handler := Authenticate(validator, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operator/runs", nil)
	request.Header.Set("Authorization", "Bearer revoked-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
	if response.Header().Get("WWW-Authenticate") != `Bearer realm="kontor-operator"` {
		t.Fatalf("WWW-Authenticate = %q", response.Header().Get("WWW-Authenticate"))
	}
}

func TestRequireRoleRejectsStaffWithoutCallingOwnerHandler(t *testing.T) {
	called := false
	handler := RequireRole(RoleOwner, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/operator/catalog/services", nil)
	request = request.WithContext(WithPrincipal(request.Context(), Principal{
		TenantID: "tenant-a", OperatorID: "operator-a", Role: RoleStaff,
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
}

func TestAuthenticateReportsValidatorFailureWithoutLeakingIt(t *testing.T) {
	validator := &stubSessionValidator{err: errors.New("database password leaked")}
	handler := Authenticate(validator, http.NotFoundHandler())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operator/runs", nil)
	request.Header.Set("Authorization", "Bearer opaque-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", response.Code)
	}
	if got := response.Body.String(); got == "" || contains(got, "database password leaked") {
		t.Fatalf("response leaked validator error: %q", got)
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
