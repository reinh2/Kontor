package demohttp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reinhlord/kontor/internal/agenttrace"
	"github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/conversations"
)

func TestCreateConversationReturnsCapabilityOnce(t *testing.T) {
	application := &fakeApplication{created: conversations.Conversation{
		ID: "conversation-1", CustomerID: "customer-1", TokenBudget: 50000,
		CapabilityToken: "opaque-conversation-secret",
	}}
	handler := newTestHandler(application, &fakeTraceReader{}, fakeReady{})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/demo/conversations",
		strings.NewReader(`{"display_name":"Ada","email":"ada@example.com"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control=%q, want no-store", response.Header().Get("Cache-Control"))
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["capability_token"] != "opaque-conversation-secret" {
		t.Fatalf("capability_token=%v", payload["capability_token"])
	}
	if payload["conversation_id"] != "conversation-1" {
		t.Fatalf("conversation_id=%v", payload["conversation_id"])
	}
}

func TestSendMessageRequiresCapabilityForTargetConversation(t *testing.T) {
	application := &fakeApplication{capabilities: map[string]string{
		"conversation-a": "token-a",
		"conversation-b": "token-b",
	}}
	handler := newTestHandler(application, &fakeTraceReader{}, fakeReady{})

	for _, test := range []struct {
		name          string
		authorization string
		wantStatus    int
		wantSent      int
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized, wantSent: 0},
		{name: "other conversation token", authorization: "Bearer token-a", wantStatus: http.StatusUnauthorized, wantSent: 0},
		{name: "matching token", authorization: "Bearer token-b", wantStatus: http.StatusOK, wantSent: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			application.sent = nil
			request := httptest.NewRequest(http.MethodPost, "/api/v1/demo/conversations/conversation-b/messages",
				strings.NewReader(`{"client_message_id":"client-1","text":"hello"}`))
			request.Header.Set("Content-Type", "application/json")
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if len(application.sent) != test.wantSent {
				t.Fatalf("send calls=%d want=%d", len(application.sent), test.wantSent)
			}
			if test.wantStatus == http.StatusUnauthorized && response.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("unauthorized response omitted WWW-Authenticate")
			}
		})
	}
}

func TestSendMessageMapsTurnOverloadToControlledServiceUnavailable(t *testing.T) {
	application := &fakeApplication{
		capabilities: map[string]string{"conversation-a": "token-a"},
		sendErr:      &app.TurnOverloadError{Waited: 100 * time.Millisecond},
	}
	handler := newTestHandler(application, &fakeTraceReader{}, fakeReady{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/demo/conversations/conversation-a/messages",
		strings.NewReader(`{"client_message_id":"client-1","text":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token-a")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After=%q, want 1", response.Header().Get("Retry-After"))
	}
	if strings.Contains(response.Body.String(), "100ms") || !strings.Contains(response.Body.String(), "service busy") {
		t.Fatalf("overload response exposed internals or omitted controlled title: %s", response.Body.String())
	}
}

func TestGetRunRequiresCapabilityForOwningConversation(t *testing.T) {
	application := &fakeApplication{capabilities: map[string]string{
		"conversation-a": "token-a",
		"conversation-b": "token-b",
	}}
	traces := &fakeTraceReader{runs: map[string]agenttrace.RunTrace{
		"run-b": {ID: "run-b", ConversationID: "conversation-b", Status: "completed"},
	}}
	handler := newTestHandler(application, traces, fakeReady{})

	wrong := httptest.NewRequest(http.MethodGet, "/api/v1/demo/runs/run-b", nil)
	wrong.Header.Set("Authorization", "Bearer token-a")
	wrongResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongResponse, wrong)
	if wrongResponse.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-token status=%d body=%s", wrongResponse.Code, wrongResponse.Body.String())
	}

	valid := httptest.NewRequest(http.MethodGet, "/api/v1/demo/runs/run-b", nil)
	valid.Header.Set("Authorization", "Bearer token-b")
	validResponse := httptest.NewRecorder()
	handler.ServeHTTP(validResponse, valid)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("valid-token status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
	if strings.Contains(validResponse.Body.String(), "token-b") {
		t.Fatal("trace response leaked capability token")
	}
}

func TestBearerTokenParsing(t *testing.T) {
	for _, test := range []struct {
		header string
		ok     bool
	}{
		{header: "Bearer opaque", ok: true},
		{header: "bearer opaque", ok: true},
		{header: ""},
		{header: "Basic opaque"},
		{header: "Bearer"},
		{header: "Bearer one two"},
	} {
		_, ok := bearerToken(test.header)
		if ok != test.ok {
			t.Errorf("bearerToken(%q) ok=%v, want %v", test.header, ok, test.ok)
		}
	}
}

func newTestHandler(application applicationService, trace traceReader, ready readinessChecker) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(application, trace, ready, logger)
}

type fakeApplication struct {
	created      conversations.Conversation
	capabilities map[string]string
	sent         []string
	sendErr      error
}

func (f *fakeApplication) CreateConversation(context.Context, conversations.Profile) (conversations.Conversation, error) {
	return f.created, nil
}

func (f *fakeApplication) VerifyConversationCapability(_ context.Context, conversationID, token string) error {
	expected, found := f.capabilities[conversationID]
	if !found {
		return conversations.ErrNotFound
	}
	if token != expected {
		return conversations.ErrUnauthorized
	}
	return nil
}

func (f *fakeApplication) SendMessage(_ context.Context, conversationID, _, _ string) (app.TurnResult, error) {
	f.sent = append(f.sent, conversationID)
	if f.sendErr != nil {
		return app.TurnResult{}, f.sendErr
	}
	return app.TurnResult{ConversationID: conversationID, Message: "ok", Outcome: "completed"}, nil
}

type fakeTraceReader struct {
	runs map[string]agenttrace.RunTrace
}

func (f *fakeTraceReader) GetRun(_ context.Context, runID string) (agenttrace.RunTrace, error) {
	run, found := f.runs[runID]
	if !found {
		return agenttrace.RunTrace{}, pgx.ErrNoRows
	}
	return run, nil
}

type fakeReady struct{ err error }

func (f fakeReady) Ping(context.Context) error { return f.err }
