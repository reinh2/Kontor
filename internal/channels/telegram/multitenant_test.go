package telegram

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/tenants"
)

const multiTenantWebhookSecret = "multi-tenant-webhook-secret"

type unavailableTenantRuntime struct{}

func (unavailableTenantRuntime) TelegramFor(context.Context, string) (*app.Service, *conversations.Store, error) {
	return nil, nil, errors.New("runtime unavailable")
}

type multiTenantWebhookStore struct {
	resolvedSlug string
}

func (s *multiTenantWebhookStore) TenantBySlug(_ context.Context, slug string) (tenants.Tenant, error) {
	s.resolvedSlug = slug
	return tenants.Tenant{ID: "tenant-" + slug, Slug: slug}, nil
}

func (*multiTenantWebhookStore) TelegramCredentials(_ context.Context, tenantID string) (tenants.TelegramCredentials, error) {
	if tenantID != "tenant-north" {
		return tenants.TelegramCredentials{}, tenants.ErrNotFound
	}
	return tenants.TelegramCredentials{
		BotToken:      "test-token",
		WebhookDigest: sha256.Sum256([]byte(multiTenantWebhookSecret)),
	}, nil
}

func TestMultiTenantWebhookRetriesRuntimeFailureWithoutClaimingUpdate(t *testing.T) {
	claimer := &fakeClaimer{}
	store := &multiTenantWebhookStore{}
	webhook, err := NewMultiTenantWebhook(claimer, unavailableTenantRuntime{}, store, "", 50_000, nil)
	if err != nil {
		t.Fatalf("NewMultiTenantWebhook: %v", err)
	}
	routes := http.NewServeMux()
	routes.Handle("POST /webhooks/v1/telegram/{tenantSlug}", webhook)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/v1/telegram/north", strings.NewReader(updateBody))
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", multiTenantWebhookSecret)
	response := httptest.NewRecorder()
	routes.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want retriable 500", response.Code)
	}
	if store.resolvedSlug != "north" {
		t.Fatalf("resolved tenant slug=%q, want north", store.resolvedSlug)
	}
	if claimer.calls.Load() != 0 {
		t.Fatalf("runtime failure claimed %d update(s), want none", claimer.calls.Load())
	}
}

func TestMultiTenantWebhookRejectsInvalidTenantSecret(t *testing.T) {
	claimer := &fakeClaimer{}
	webhook, err := NewMultiTenantWebhook(claimer, unavailableTenantRuntime{}, &multiTenantWebhookStore{}, "", 50_000, nil)
	if err != nil {
		t.Fatalf("NewMultiTenantWebhook: %v", err)
	}
	routes := http.NewServeMux()
	routes.Handle("POST /webhooks/v1/telegram/{tenantSlug}", webhook)
	request := httptest.NewRequest(http.MethodPost, "/webhooks/v1/telegram/north", strings.NewReader(updateBody))
	request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-webhook-secret")
	response := httptest.NewRecorder()
	routes.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", response.Code)
	}
	if claimer.calls.Load() != 0 {
		t.Fatalf("invalid secret claimed %d update(s), want none", claimer.calls.Load())
	}
}
