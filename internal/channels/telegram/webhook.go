package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/tools"
)

// updateClaimer is the single write the webhook needs from PostgreSQL;
// *pgxpool.Pool satisfies it.
type updateClaimer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// applicationService is the slice of the application the webhook drives.
type applicationService interface {
	SendMessage(ctx context.Context, conversationID, text, clientMessageID string) (app.TurnResult, error)
}

// conversationMapper binds one Telegram chat to one Kontor conversation.
type conversationMapper interface {
	EnsureChannelConversation(ctx context.Context, tenantID, channel, channelRef string, profile conversations.Profile, tokenBudget int) (conversations.Conversation, error)
}

type Config struct {
	TenantID string
	// WebhookSecret must match the secret_token registered with setWebhook;
	// Telegram echoes it in X-Telegram-Bot-Api-Secret-Token on every call.
	WebhookSecret string
	TokenBudget   int
}

type Webhook struct {
	config Config
	pool   updateClaimer
	app    applicationService
	store  conversationMapper
	sender Sender
	logger *slog.Logger
}

func NewWebhook(
	config Config,
	pool updateClaimer,
	application applicationService,
	store conversationMapper,
	sender Sender,
	logger *slog.Logger,
) (*Webhook, error) {
	if config.TenantID == "" || config.WebhookSecret == "" {
		return nil, errors.New("telegram: tenant ID and webhook secret are required")
	}
	if pool == nil || application == nil || store == nil || sender == nil {
		return nil, errors.New("telegram: all webhook dependencies are required")
	}
	if config.TokenBudget < 1 {
		config.TokenBudget = 50_000
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Webhook{config: config, pool: pool, app: application, store: store, sender: sender, logger: logger}, nil
}

// update mirrors the subset of the Bot API payload the channel consumes.
type update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		From *struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Username  string `json:"username"`
		} `json:"from"`
	} `json:"message"`
}

// ServeHTTP processes one webhook call. It always acknowledges accepted,
// verified updates with 200 — Telegram redelivers non-2xx responses, and the
// durable update_id record already makes redelivery harmless.
func (h *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	presented := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if subtle.ConstantTimeCompare([]byte(presented), []byte(h.config.WebhookSecret)) != 1 {
		// An unverified caller learns nothing about the endpoint.
		w.WriteHeader(http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var payload update
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		// A malformed body would be redelivered forever; acknowledge and drop.
		h.logger.Warn("telegram update rejected", "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	if payload.UpdateID == 0 || payload.Message == nil ||
		strings.TrimSpace(payload.Message.Text) == "" || payload.Message.Chat.Type != "private" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Resolve (and, on first contact, create) the tenant-scoped conversation
	// before claiming the update. A mapping failure must return 500 *before* the
	// dedupe row is committed so Telegram can redeliver; otherwise a transient
	// error would claim the update and silently drop the customer's message.
	// EnsureChannelConversation is idempotent, so re-resolving a redelivered
	// update is harmless.
	chatID := payload.Message.Chat.ID
	conversation, err := h.store.EnsureChannelConversation(
		r.Context(), h.config.TenantID, "telegram", strconv.FormatInt(chatID, 10),
		telegramProfile(payload), h.config.TokenBudget,
	)
	if err != nil {
		h.logger.Error("telegram conversation mapping failed", "update_id", payload.UpdateID, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	fresh, err := h.claimUpdate(r.Context(), payload.UpdateID)
	if err != nil {
		// The claim failed before any turn ran; a retry is safe and desirable.
		h.logger.Error("telegram update claim failed", "update_id", payload.UpdateID, "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !fresh {
		w.WriteHeader(http.StatusOK)
		return
	}

	turn, err := h.app.SendMessage(
		r.Context(), conversation.ID, payload.Message.Text,
		fmt.Sprintf("tg-%d", payload.UpdateID),
	)
	reply := turn.Message
	if err != nil {
		if errors.Is(err, app.ErrTurnOverloaded) {
			reply = "I’m handling a lot of messages right now — please send that again in a moment."
		} else {
			h.logger.Error("telegram turn failed", "conversation_id", conversation.ID, "error", err)
			reply = "Something went wrong on our side. A person will follow up."
		}
	}
	if turn.PendingConfirmation != nil {
		reply = reply + "\n\n" + formatConfirmation(turn.PendingConfirmation.Title, turn.PendingConfirmation.Facts) +
			"\n\nReply \"Yes, confirm\" to confirm."
	}
	if sendErr := h.sender.Send(r.Context(), chatID, reply); sendErr != nil {
		// The turn outcome is durable; only delivery failed. Do not ask
		// Telegram to redeliver the inbound update — that cannot resend our
		// reply, the dedupe record would swallow it anyway.
		h.logger.Error("telegram reply delivery failed", "conversation_id", conversation.ID, "error", sendErr)
	}
	w.WriteHeader(http.StatusOK)
}

// claimUpdate records the update id; false means another delivery of the
// same update already claimed it.
func (h *Webhook) claimUpdate(ctx context.Context, updateID int64) (bool, error) {
	tag, err := h.pool.Exec(ctx, `
		INSERT INTO telegram_updates(tenant_id,update_id)
		VALUES($1,$2)
		ON CONFLICT (tenant_id,update_id) DO NOTHING`, h.config.TenantID, updateID)
	if err != nil {
		return false, fmt.Errorf("claim telegram update: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// telegramProfile derives a customer profile from the sender. Telegram
// exposes no email or phone, and the customer schema requires a contact, so
// the channel records a clearly synthetic, non-deliverable address under the
// reserved .invalid TLD until the customer shares real contact details.
func telegramProfile(payload update) conversations.Profile {
	name := ""
	if payload.Message.From != nil {
		name = strings.TrimSpace(payload.Message.From.FirstName + " " + payload.Message.From.LastName)
		if name == "" {
			name = payload.Message.From.Username
		}
	}
	if name == "" {
		name = "Telegram customer"
	}
	return conversations.Profile{
		DisplayName: name,
		Email:       fmt.Sprintf("telegram-%d@customers.invalid", payload.Message.Chat.ID),
	}
}

func formatConfirmation(title string, facts []tools.ConfirmationFact) string {
	lines := make([]string, 0, len(facts)+1)
	if title != "" {
		lines = append(lines, title)
	}
	for _, fact := range facts {
		lines = append(lines, fact.Label+": "+fact.Value)
	}
	return strings.Join(lines, "\n")
}
