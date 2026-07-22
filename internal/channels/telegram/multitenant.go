package telegram

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/tenants"
)

type tenantRuntime interface {
	TelegramFor(context.Context, string) (*app.Service, *conversations.Store, error)
}

type telegramTenantStore interface {
	TenantBySlug(context.Context, string) (tenants.Tenant, error)
	TelegramCredentials(context.Context, string) (tenants.TelegramCredentials, error)
}

// MultiTenantWebhook resolves the tenant from the URL slug, authenticates the
// webhook against that tenant's digest-only secret, and then obtains a tenant
// scoped application graph. Telegram cannot select another tenant through its
// body, chat ID, or a header.
type MultiTenantWebhook struct {
	pool        updateClaimer
	runtime     tenantRuntime
	tenants     telegramTenantStore
	apiBaseURL  string
	tokenBudget int
	logger      *slog.Logger
}

func NewMultiTenantWebhook(pool updateClaimer, runtime tenantRuntime, tenantStore telegramTenantStore, apiBaseURL string, tokenBudget int, logger *slog.Logger) (*MultiTenantWebhook, error) {
	if pool == nil || runtime == nil || tenantStore == nil {
		return nil, errors.New("telegram: tenant runtime, store, and pool are required")
	}
	if tokenBudget < 1 {
		tokenBudget = 50_000
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MultiTenantWebhook{pool: pool, runtime: runtime, tenants: tenantStore, apiBaseURL: apiBaseURL, tokenBudget: tokenBudget, logger: logger}, nil
}

func (h *MultiTenantWebhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	slug := strings.TrimSpace(r.PathValue("tenantSlug"))
	tenant, err := h.tenants.TenantBySlug(r.Context(), slug)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	credentials, err := h.tenants.TelegramCredentials(r.Context(), tenant.ID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	presented := sha256.Sum256([]byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")))
	if subtle.ConstantTimeCompare(presented[:], credentials.WebhookDigest[:]) != 1 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var payload update
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.logger.Warn("telegram update rejected", "tenant_id", tenant.ID, "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	if payload.UpdateID == 0 || payload.Message == nil || strings.TrimSpace(payload.Message.Text) == "" || payload.Message.Chat.Type != "private" {
		w.WriteHeader(http.StatusOK)
		return
	}
	application, conversationsStore, err := h.runtime.TelegramFor(r.Context(), tenant.ID)
	if err != nil {
		h.logger.Error("tenant runtime unavailable", "tenant_id", tenant.ID, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	conversation, err := conversationsStore.EnsureChannelConversation(
		r.Context(), tenant.ID, "telegram", strconv.FormatInt(payload.Message.Chat.ID, 10), telegramProfile(payload), h.tokenBudget,
	)
	if err != nil {
		h.logger.Error("telegram conversation mapping failed", "tenant_id", tenant.ID, "update_id", payload.UpdateID, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fresh, err := h.claimUpdate(r.Context(), tenant.ID, payload.UpdateID)
	if err != nil {
		h.logger.Error("telegram update claim failed", "tenant_id", tenant.ID, "update_id", payload.UpdateID, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !fresh {
		w.WriteHeader(http.StatusOK)
		return
	}
	turn, turnErr := application.SendMessage(r.Context(), conversation.ID, payload.Message.Text, fmt.Sprintf("tg-%d", payload.UpdateID))
	reply := turn.Message
	if turnErr != nil {
		if errors.Is(turnErr, app.ErrTurnOverloaded) {
			reply = "I’m handling a lot of messages right now — please send that again in a moment."
		} else {
			h.logger.Error("telegram turn failed", "tenant_id", tenant.ID, "conversation_id", conversation.ID, "error", turnErr)
			reply = "Something went wrong on our side. A person will follow up."
		}
	}
	if turn.PendingConfirmation != nil {
		reply += "\n\n" + formatConfirmation(turn.PendingConfirmation.Title, turn.PendingConfirmation.Facts) + "\n\nReply \"Yes, confirm\" to confirm."
	}
	sender, err := NewBotAPISender(BotAPIConfig{Token: credentials.BotToken, BaseURL: h.apiBaseURL})
	if err != nil {
		h.logger.Error("telegram sender unavailable", "tenant_id", tenant.ID, "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := sender.Send(r.Context(), payload.Message.Chat.ID, reply); err != nil {
		h.logger.Error("telegram reply delivery failed", "tenant_id", tenant.ID, "conversation_id", conversation.ID, "error", err)
	}
	w.WriteHeader(http.StatusOK)
}

func (h *MultiTenantWebhook) claimUpdate(ctx context.Context, tenantID string, updateID int64) (bool, error) {
	tag, err := h.pool.Exec(ctx, `
		INSERT INTO telegram_updates(tenant_id,update_id)
		VALUES($1,$2)
		ON CONFLICT (tenant_id,update_id) DO NOTHING`, tenantID, updateID)
	if err != nil {
		return false, fmt.Errorf("claim telegram update: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
