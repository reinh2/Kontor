package operatorhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/reinhlord/kontor/internal/identity"
)

type testSessionValidator struct {
	principals map[string]identity.Principal
}

func (s testSessionValidator) ValidateSession(_ context.Context, token string) (identity.Principal, error) {
	principal, ok := s.principals[token]
	if !ok {
		return identity.Principal{}, identity.ErrSessionInvalid
	}
	return principal, nil
}

type sessionScopedBackend struct {
	tenantIDs []string
}

func (b *sessionScopedBackend) Dashboard(ctx context.Context, _ DashboardRequest) (Dashboard, error) {
	principal, ok := identity.PrincipalFromContext(ctx)
	if !ok {
		return Dashboard{}, identity.ErrSessionInvalid
	}
	b.tenantIDs = append(b.tenantIDs, principal.TenantID)
	return Dashboard{}, nil
}
func (*sessionScopedBackend) ListRuns(context.Context, ListRunsRequest) (RunPage, error) {
	return RunPage{}, nil
}
func (*sessionScopedBackend) GetRun(context.Context, string) (RunDetail, error) {
	return RunDetail{}, nil
}
func (*sessionScopedBackend) Calendar(context.Context, CalendarRequest) (Calendar, error) {
	return Calendar{}, nil
}
func (*sessionScopedBackend) ListCustomers(context.Context, CustomerListRequest) (CustomerList, error) {
	return CustomerList{}, nil
}
func (*sessionScopedBackend) CreateBooking(context.Context, CreateBookingCommand) (Booking, error) {
	return Booking{}, nil
}
func (*sessionScopedBackend) RescheduleBooking(context.Context, RescheduleBookingCommand) (Booking, error) {
	return Booking{}, nil
}
func (*sessionScopedBackend) CancelBooking(context.Context, CancelBookingCommand) (Booking, error) {
	return Booking{}, nil
}

func TestSessionAuthenticationReturnsPrincipalAndScopesBackend(t *testing.T) {
	validator := testSessionValidator{principals: map[string]identity.Principal{
		"session-a": {
			TenantID: "tenant-a", TenantName: "Salon A", Timezone: "Europe/Berlin", Currency: "EUR",
			OperatorID: "operator-a", Email: "owner@a.test", DisplayName: "Owner A", Role: identity.RoleOwner,
		},
		"session-b": {
			TenantID: "tenant-b", TenantName: "Salon B", Timezone: "America/New_York", Currency: "USD",
			OperatorID: "operator-b", Email: "staff@b.test", DisplayName: "Staff B", Role: identity.RoleStaff,
		},
	}}
	backend := &sessionScopedBackend{}
	handler, err := New(Config{Authenticator: validator, Now: func() time.Time { return time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC) }}, backend, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/operator/session", nil)
	sessionRequest.Header.Set("Authorization", "Bearer session-a")
	sessionResponse := httptest.NewRecorder()
	handler.ServeHTTP(sessionResponse, sessionRequest)
	if sessionResponse.Code != http.StatusOK || !containsSessionField(sessionResponse.Body.String(), `"tenant_id":"tenant-a"`) || !containsSessionField(sessionResponse.Body.String(), `"operator_name":"Owner A"`) || !containsSessionField(sessionResponse.Body.String(), `"role":"owner"`) {
		t.Fatalf("session status=%d body=%s", sessionResponse.Code, sessionResponse.Body.String())
	}

	for _, token := range []string{"session-a", "session-b"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/operator/dashboard", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("dashboard token %q status=%d", token, response.Code)
		}
	}
	if len(backend.tenantIDs) != 2 || backend.tenantIDs[0] != "tenant-a" || backend.tenantIDs[1] != "tenant-b" {
		t.Fatalf("backend tenant IDs=%v", backend.tenantIDs)
	}
}

func TestSessionAuthenticationRejectsUnknownTokenBeforeBackend(t *testing.T) {
	backend := &sessionScopedBackend{}
	handler, err := New(Config{Authenticator: testSessionValidator{}}, backend, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operator/dashboard", nil)
	request.Header.Set("Authorization", "Bearer unknown-session")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || len(backend.tenantIDs) != 0 {
		t.Fatalf("status=%d backend=%v", response.Code, backend.tenantIDs)
	}
}

func containsSessionField(value, field string) bool {
	for index := 0; index+len(field) <= len(value); index++ {
		if value[index:index+len(field)] == field {
			return true
		}
	}
	return false
}
