package telegram

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/tools"
)

const webhookTestSecret = "webhook-secret-at-least-16b"

func newTestWebhook(t *testing.T, claimer *fakeClaimer, application *fakeWebhookApp, sender *fakeSender) *Webhook {
	t.Helper()
	webhook, err := NewWebhook(Config{
		TenantID: "00000000-0000-4000-8000-000000000001", WebhookSecret: webhookTestSecret,
	}, claimer, application, &fakeMapper{}, sender, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return webhook
}

func postUpdate(webhook *Webhook, secret, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/webhooks/v1/telegram", strings.NewReader(body))
	if secret != "" {
		request.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
	}
	response := httptest.NewRecorder()
	webhook.ServeHTTP(response, request)
	return response
}

const updateBody = `{"update_id":7001,"message":{"text":"I need a haircut","chat":{"id":42,"type":"private"},"from":{"first_name":"Marta"}}}`

func TestWebhookRejectsMissingOrWrongSecretWithoutProcessing(t *testing.T) {
	claimer := &fakeClaimer{}
	application := &fakeWebhookApp{}
	webhook := newTestWebhook(t, claimer, application, &fakeSender{})

	for _, secret := range []string{"", "wrong-secret-000000000"} {
		response := postUpdate(webhook, secret, updateBody)
		if response.Code != http.StatusNotFound {
			t.Fatalf("secret=%q status=%d, want 404", secret, response.Code)
		}
	}
	if claimer.calls.Load() != 0 || application.turns.Load() != 0 {
		t.Fatal("unverified request reached the dedupe store or the application")
	}
}

func TestWebhookRunsOneTurnAndRepliesToChat(t *testing.T) {
	claimer := &fakeClaimer{}
	application := &fakeWebhookApp{result: app.TurnResult{
		Message: "Here is a slot.", Outcome: "completed",
		PendingConfirmation: &tools.ConfirmationProposal{
			Title: "Confirm this booking",
			Facts: []tools.ConfirmationFact{{Label: "Service", Value: "Haircut"}},
		},
	}}
	sender := &fakeSender{}
	webhook := newTestWebhook(t, claimer, application, sender)

	response := postUpdate(webhook, webhookTestSecret, updateBody)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	if application.turns.Load() != 1 {
		t.Fatalf("agent turns=%d, want 1", application.turns.Load())
	}
	if application.lastClientMessageID != "tg-7001" {
		t.Fatalf("client message id=%q, want tg-7001", application.lastClientMessageID)
	}
	if sender.chatID != 42 || !strings.Contains(sender.text, "Here is a slot.") ||
		!strings.Contains(sender.text, "Service: Haircut") ||
		!strings.Contains(sender.text, `Reply "Yes, confirm"`) {
		t.Fatalf("reply=%q chat=%d", sender.text, sender.chatID)
	}
}

func TestWebhookRetryOfSameUpdateDoesNotRunSecondTurn(t *testing.T) {
	claimer := &fakeClaimer{}
	application := &fakeWebhookApp{result: app.TurnResult{Message: "ok", Outcome: "completed"}}
	sender := &fakeSender{}
	webhook := newTestWebhook(t, claimer, application, sender)

	first := postUpdate(webhook, webhookTestSecret, updateBody)
	second := postUpdate(webhook, webhookTestSecret, updateBody)
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("statuses=%d,%d — a redelivered update must still acknowledge", first.Code, second.Code)
	}
	if application.turns.Load() != 1 {
		t.Fatalf("agent turns=%d after redelivery, want exactly 1", application.turns.Load())
	}
	if sender.sends.Load() != 1 {
		t.Fatalf("replies=%d after redelivery, want exactly 1", sender.sends.Load())
	}
}

func TestWebhookIgnoresNonPrivateAndNonTextUpdates(t *testing.T) {
	claimer := &fakeClaimer{}
	application := &fakeWebhookApp{}
	webhook := newTestWebhook(t, claimer, application, &fakeSender{})

	for name, body := range map[string]string{
		"group chat":    `{"update_id":7002,"message":{"text":"hi","chat":{"id":9,"type":"group"}}}`,
		"no message":    `{"update_id":7003}`,
		"empty text":    `{"update_id":7004,"message":{"text":"  ","chat":{"id":9,"type":"private"}}}`,
		"not even json": `not json`,
	} {
		response := postUpdate(webhook, webhookTestSecret, body)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: status=%d, want acknowledged 200", name, response.Code)
		}
	}
	if application.turns.Load() != 0 {
		t.Fatalf("ignored updates ran %d turns", application.turns.Load())
	}
}

func TestSenderRetriesTransientFailuresAndHonorsPermanentRejection(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer api.Close()
	sender, err := NewBotAPISender(BotAPIConfig{Token: "test-token", BaseURL: api.URL, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(context.Background(), 42, "hello"); err != nil {
		t.Fatalf("transient failure was not retried: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("attempts=%d, want 2", calls.Load())
	}

	var permanentCalls atomic.Int32
	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		permanentCalls.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer rejecting.Close()
	blocked, err := NewBotAPISender(BotAPIConfig{Token: "test-token", BaseURL: rejecting.URL, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := blocked.Send(context.Background(), 42, "hello"); err == nil {
		t.Fatal("permanent rejection did not surface an error")
	}
	if permanentCalls.Load() != 1 {
		t.Fatalf("permanent rejection was retried %d times", permanentCalls.Load())
	}
}

/* ------------------------------- fakes ------------------------------- */

type fakeClaimer struct {
	calls   atomic.Int32
	claimed map[int64]bool
}

func (f *fakeClaimer) Exec(_ context.Context, _ string, arguments ...any) (pgconn.CommandTag, error) {
	f.calls.Add(1)
	if f.claimed == nil {
		f.claimed = make(map[int64]bool)
	}
	updateID := arguments[1].(int64)
	if f.claimed[updateID] {
		return pgconn.NewCommandTag("INSERT 0 0"), nil
	}
	f.claimed[updateID] = true
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

type fakeWebhookApp struct {
	turns               atomic.Int32
	lastClientMessageID string
	result              app.TurnResult
	err                 error
}

func (f *fakeWebhookApp) SendMessage(_ context.Context, _, _, clientMessageID string) (app.TurnResult, error) {
	f.turns.Add(1)
	f.lastClientMessageID = clientMessageID
	return f.result, f.err
}

type fakeMapper struct{}

func (fakeMapper) EnsureChannelConversation(_ context.Context, tenantID, channel, channelRef string, _ conversations.Profile, _ int) (conversations.Conversation, error) {
	return conversations.Conversation{
		TenantID: tenantID, ID: "conversation-" + channelRef, CustomerID: "customer-" + channelRef,
		Channel: channel, Status: "open",
	}, nil
}

type fakeSender struct {
	sends  atomic.Int32
	chatID int64
	text   string
}

func (f *fakeSender) Send(_ context.Context, chatID int64, text string) error {
	f.sends.Add(1)
	f.chatID = chatID
	f.text = text
	return nil
}
