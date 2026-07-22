package onboardinghttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reinhlord/kontor/internal/identity"
	"github.com/reinhlord/kontor/internal/tenants"
)

type stubTenantStore struct {
	provisioned      tenants.ProvisionInput
	serviceTenantIDs []string
	channelTenantIDs []string
	channels         map[string]tenants.ChannelConfig
}

func (s *stubTenantStore) Provision(_ context.Context, input tenants.ProvisionInput) (tenants.Tenant, error) {
	s.provisioned = input
	return tenants.Tenant{ID: "tenant-" + input.Slug, Slug: input.Slug, Name: input.Name}, nil
}

func (s *stubTenantStore) CreateService(_ context.Context, tenantID string, input tenants.ServiceInput) (tenants.Service, error) {
	s.serviceTenantIDs = append(s.serviceTenantIDs, tenantID)
	return tenants.Service{ID: "service-" + tenantID, Slug: input.Slug, Name: input.Name}, nil
}

func (s *stubTenantStore) CreateStaff(_ context.Context, tenantID string, input tenants.StaffInput) (tenants.Staff, error) {
	return tenants.Staff{ID: "staff-" + tenantID, Slug: input.Slug, DisplayName: input.DisplayName}, nil
}

func (s *stubTenantStore) AddAvailability(context.Context, string, string, []tenants.AvailabilityRuleInput) error {
	return nil
}

func (s *stubTenantStore) UpdateChannels(_ context.Context, tenantID string, config tenants.ChannelConfig) error {
	if s.channels == nil {
		s.channels = map[string]tenants.ChannelConfig{}
	}
	s.channels[tenantID] = config
	return nil
}

func (s *stubTenantStore) ChannelConfig(_ context.Context, tenantID string) (tenants.ChannelConfig, error) {
	s.channelTenantIDs = append(s.channelTenantIDs, tenantID)
	return s.channels[tenantID], nil
}

type stubIdentityStore struct {
	loginResult identity.LoginResult
	loginErr    error
	principals  map[string]identity.Principal
	loginArgs   []string
	revoked     []string
}

func (s *stubIdentityStore) Authenticate(_ context.Context, slug, email, password string) (identity.LoginResult, error) {
	s.loginArgs = []string{slug, email, password}
	if s.loginErr != nil {
		return identity.LoginResult{}, s.loginErr
	}
	return s.loginResult, nil
}

func (s *stubIdentityStore) RevokeSession(_ context.Context, token string) error {
	s.revoked = append(s.revoked, token)
	return nil
}

func (s *stubIdentityStore) CreateOperator(_ context.Context, input identity.CreateOperatorInput) (identity.Operator, error) {
	return identity.Operator{ID: "operator-" + input.TenantID, TenantID: input.TenantID, Email: input.Email, DisplayName: input.DisplayName, Role: input.Role}, nil
}

func (s *stubIdentityStore) ValidateSession(_ context.Context, token string) (identity.Principal, error) {
	principal, ok := s.principals[token]
	if !ok {
		return identity.Principal{}, identity.ErrSessionInvalid
	}
	return principal, nil
}

func TestLoginCreatesOpaqueSessionResponse(t *testing.T) {
	stores := &stubTenantStore{}
	identities := &stubIdentityStore{loginResult: identity.LoginResult{
		Token: "opaque-session", Principal: identity.Principal{
			TenantID: "tenant-north", TenantName: "North", Timezone: "Europe/Berlin", Currency: "EUR",
			OperatorID: "operator-north", Email: "owner@north.test", DisplayName: "North owner", Role: identity.RoleOwner,
			ExpiresAt: time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC),
		},
	}}
	handler, err := New(stores, identities)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/operator/login", strings.NewReader(`{"tenant_slug":"north","email":"OWNER@north.test","password":"correct-horse-battery-staple"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := identities.loginArgs; strings.Join(got, ",") != "north,OWNER@north.test,correct-horse-battery-staple" {
		t.Fatalf("login args=%q", got)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q", response.Header().Get("Cache-Control"))
	}
	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Session     struct {
			TenantID     string `json:"tenant_id"`
			OperatorID   string `json:"operator_id"`
			OperatorName string `json:"operator_name"`
			Role         string `json:"role"`
		} `json:"session"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.AccessToken != "opaque-session" || body.TokenType != "Bearer" || body.Session.TenantID != "tenant-north" || body.Session.OperatorID != "operator-north" || body.Session.OperatorName != "North owner" || body.Session.Role != identity.RoleOwner {
		t.Fatalf("login response=%#v", body)
	}
}

func TestOwnerMutationsAreSessionScopedAndStaffCannotMutate(t *testing.T) {
	stores := &stubTenantStore{channels: map[string]tenants.ChannelConfig{
		"tenant-a": {WidgetOrigin: "https://a.example"},
		"tenant-b": {WidgetOrigin: "https://b.example"},
	}}
	identities := &stubIdentityStore{principals: map[string]identity.Principal{
		"owner-a": {TenantID: "tenant-a", OperatorID: "operator-a", Role: identity.RoleOwner},
		"owner-b": {TenantID: "tenant-b", OperatorID: "operator-b", Role: identity.RoleOwner},
		"staff-a": {TenantID: "tenant-a", OperatorID: "operator-staff", Role: identity.RoleStaff},
	}}
	handler, err := New(stores, identities)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, token := range []string{"owner-a", "owner-b"} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/operator/catalog/services", strings.NewReader(`{"slug":"cut","name":"Cut"}`))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("owner %q status=%d body=%s", token, response.Code, response.Body.String())
		}
	}
	if got := strings.Join(stores.serviceTenantIDs, ","); got != "tenant-a,tenant-b" {
		t.Fatalf("service tenant IDs = %q, want each owner session tenant", got)
	}

	staffRequest := httptest.NewRequest(http.MethodPost, "/api/v1/operator/catalog/services", strings.NewReader(`{"slug":"cut","name":"Cut"}`))
	staffRequest.Header.Set("Authorization", "Bearer staff-a")
	staffRequest.Header.Set("Content-Type", "application/json")
	staffResponse := httptest.NewRecorder()
	handler.ServeHTTP(staffResponse, staffRequest)
	if staffResponse.Code != http.StatusForbidden || len(stores.serviceTenantIDs) != 2 {
		t.Fatalf("staff status=%d service calls=%v", staffResponse.Code, stores.serviceTenantIDs)
	}

	spoofRequest := httptest.NewRequest(http.MethodPost, "/api/v1/operator/catalog/services", strings.NewReader(`{"slug":"cut","name":"Cut","tenant_id":"tenant-b"}`))
	spoofRequest.Header.Set("Authorization", "Bearer owner-a")
	spoofRequest.Header.Set("Content-Type", "application/json")
	spoofResponse := httptest.NewRecorder()
	handler.ServeHTTP(spoofResponse, spoofRequest)
	if spoofResponse.Code != http.StatusBadRequest || len(stores.serviceTenantIDs) != 2 {
		t.Fatalf("spoofed tenant status=%d service calls=%v", spoofResponse.Code, stores.serviceTenantIDs)
	}
}

func TestOwnerChannelReadsRemainTenantScoped(t *testing.T) {
	stores := &stubTenantStore{channels: map[string]tenants.ChannelConfig{
		"tenant-a": {WidgetOrigin: "https://a.example"},
		"tenant-b": {WidgetOrigin: "https://b.example"},
	}}
	identities := &stubIdentityStore{principals: map[string]identity.Principal{
		"owner-a": {TenantID: "tenant-a", OperatorID: "operator-a", Role: identity.RoleOwner},
		"owner-b": {TenantID: "tenant-b", OperatorID: "operator-b", Role: identity.RoleOwner},
	}}
	handler, _ := New(stores, identities)
	for _, test := range []struct{ token, wantOrigin string }{{"owner-a", "https://a.example"}, {"owner-b", "https://b.example"}} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/operator/channels", nil)
		request.Header.Set("Authorization", "Bearer "+test.token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.wantOrigin) {
			t.Fatalf("token %q status=%d body=%s", test.token, response.Code, response.Body.String())
		}
	}
	if got := strings.Join(stores.channelTenantIDs, ","); got != "tenant-a,tenant-b" {
		t.Fatalf("channel tenant IDs=%q", got)
	}
}

func TestLogoutRevokesBearerSession(t *testing.T) {
	stores := &stubTenantStore{}
	identities := &stubIdentityStore{principals: map[string]identity.Principal{
		"session-a": {TenantID: "tenant-a", OperatorID: "operator-a", Role: identity.RoleOwner},
	}}
	handler, _ := New(stores, identities)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/operator/logout", nil)
	request.Header.Set("Authorization", "Bearer session-a")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || strings.Join(identities.revoked, ",") != "session-a" {
		t.Fatalf("status=%d revoked=%v", response.Code, identities.revoked)
	}
}

func TestProvisionAcceptsTelegramCredentialsButDoesNotReturnThem(t *testing.T) {
	stores := &stubTenantStore{}
	handler, err := New(stores, &stubIdentityStore{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const botToken = "123456:telegram-bot-token"
	const webhookSecret = "webhook-secret-at-least-16-bytes"
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants", strings.NewReader(`{
		"slug":"north","name":"North","timezone":"Europe/Berlin","currency":"EUR",
		"owner":{"email":"owner@north.test","display_name":"North owner","password":"correct-horse-battery-staple"},
		"channels":{"widget_origin":"https://north.example","telegram_enabled":true,"telegram_bot_token":"`+botToken+`","telegram_webhook_secret":"`+webhookSecret+`"}
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if stores.provisioned.Channels.TelegramBotToken != botToken || stores.provisioned.Channels.TelegramWebhookSecret != webhookSecret {
		t.Fatalf("provisioned credentials=%+v", stores.provisioned.Channels)
	}
	if strings.Contains(response.Body.String(), botToken) || strings.Contains(response.Body.String(), webhookSecret) {
		t.Fatalf("provisioning response exposed Telegram credentials: %s", response.Body.String())
	}
}

func TestOwnerChannelUpdateAcceptsTelegramCredentialsButChannelReadHidesThem(t *testing.T) {
	const botToken = "123456:telegram-bot-token"
	const webhookSecret = "webhook-secret-at-least-16-bytes"
	stores := &stubTenantStore{channels: map[string]tenants.ChannelConfig{}}
	identities := &stubIdentityStore{principals: map[string]identity.Principal{
		"owner-a": {TenantID: "tenant-a", OperatorID: "operator-a", Role: identity.RoleOwner},
	}}
	handler, err := New(stores, identities)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	update := httptest.NewRequest(http.MethodPut, "/api/v1/operator/channels", strings.NewReader(`{
		"widget_origin":"https://a.example","telegram_enabled":true,
		"telegram_bot_token":"`+botToken+`","telegram_webhook_secret":"`+webhookSecret+`"
	}`))
	update.Header.Set("Content-Type", "application/json")
	update.Header.Set("Authorization", "Bearer owner-a")
	updateResponse := httptest.NewRecorder()
	handler.ServeHTTP(updateResponse, update)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateResponse.Code, updateResponse.Body.String())
	}
	stored := stores.channels["tenant-a"]
	if stored.TelegramBotToken != botToken || stored.TelegramWebhookSecret != webhookSecret {
		t.Fatalf("stored credentials=%+v", stored)
	}

	read := httptest.NewRequest(http.MethodGet, "/api/v1/operator/channels", nil)
	read.Header.Set("Authorization", "Bearer owner-a")
	readResponse := httptest.NewRecorder()
	handler.ServeHTTP(readResponse, read)
	if readResponse.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", readResponse.Code, readResponse.Body.String())
	}
	if strings.Contains(readResponse.Body.String(), botToken) || strings.Contains(readResponse.Body.String(), webhookSecret) {
		t.Fatalf("channel read exposed Telegram credentials: %s", readResponse.Body.String())
	}
}
