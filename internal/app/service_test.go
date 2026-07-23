package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTurnAdmissionFailsWithTypedOverloadAfterBoundedWait(t *testing.T) {
	service := &Service{
		turnAdmission: make(chan struct{}, 1),
		admissionWait: 10 * time.Millisecond,
	}
	service.turnAdmission <- struct{}{}

	startedAt := time.Now()
	release, err := service.acquireTurnAdmission(context.Background())
	if release != nil {
		release()
		t.Fatal("overloaded admission unexpectedly returned a release function")
	}
	if !errors.Is(err, ErrTurnOverloaded) {
		t.Fatalf("admission error=%v, want ErrTurnOverloaded", err)
	}
	var overload *TurnOverloadError
	if !errors.As(err, &overload) || overload.Waited != 10*time.Millisecond {
		t.Fatalf("admission error type=%T value=%#v", err, overload)
	}
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("overload response took %s, want a bounded short wait", elapsed)
	}
}

func TestTurnAdmissionHonorsEarlierContextCancellation(t *testing.T) {
	service := &Service{
		turnAdmission: make(chan struct{}, 1),
		admissionWait: time.Second,
	}
	service.turnAdmission <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	release, err := service.acquireTurnAdmission(ctx)
	if release != nil {
		release()
		t.Fatal("cancelled admission unexpectedly returned a release function")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("admission error=%v, want context cancellation", err)
	}
}

func TestSafeErrorTruncatesAtUTF8Boundary(t *testing.T) {
	input := strings.Repeat("a", 1899) + "€" + strings.Repeat("z", 20)
	got := safeError(errors.New(input))
	if !utf8.ValidString(got) {
		t.Fatalf("safeError returned invalid UTF-8: %q", got[len(got)-8:])
	}
	if len(got) != 1899 || got != strings.Repeat("a", 1899) {
		t.Fatalf("safeError length=%d suffix=%q, want 1899 ASCII bytes", len(got), got[len(got)-8:])
	}
}

func TestSystemPromptPreservesBookingFactsAndDefinesSlotRange(t *testing.T) {
	service := &Service{config: Config{
		TenantName:     "Kontor test",
		TenantTimezone: "Europe/Berlin",
		Now: func() time.Time {
			return time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
		},
	}}
	prompt := service.systemPrompt()
	for _, want := range []string{
		"Preserve facts the customer already supplied",
		"Do not ask the customer to choose the year when this rule resolves it.",
		"date_to that is strictly after date_from",
		"following local day's 00:00 as date_to",
		"never default_api.list_services",
		"correct generated arguments from the known conversation facts",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}
}

func TestTurnEventPayloadOmitsPendingConfirmationWhenInactive(t *testing.T) {
	// The SSE event delivered by persistHandoff must carry
	// pending_confirmation_active: false so the widget removes stale cards.
	payload := turnEventPayload{
		RunID:                     "run-001",
		InboundMessageID:          "msg-001",
		MessageID:                 "msg-002",
		Message:                   "This conversation reached its safety budget.",
		Outcome:                   "budget_exhausted",
		PendingConfirmation:       nil,
		PendingConfirmationActive: false,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	active, exists := decoded["pending_confirmation_active"]
	if !exists {
		t.Fatal("pending_confirmation_active is missing from the serialized event")
	}
	if active != false {
		t.Fatalf("pending_confirmation_active=%v, want false", active)
	}
	if _, hasProposal := decoded["pending_confirmation"]; hasProposal {
		t.Fatal("pending_confirmation should be omitted when nil")
	}
}

func TestDeadLetterSQLContainsExplicitTypeCasts(t *testing.T) {
	// Regression: PostgreSQL cannot infer the type of a parameter used inside
	// jsonb_build_object without an explicit cast. The dead-letter INSERT must
	// use $5::text, $6::text, $7::text to avoid SQLSTATE 42P08.
	//
	// This test validates the source code structure. The actual SQL execution
	// is covered by the integration test TestStage1ProviderFailureIsSaveFirstAndDeadLettered.
	src, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(src)
	deadLetterStart := strings.Index(content, "INSERT INTO dead_letter_events")
	if deadLetterStart < 0 {
		t.Fatal("dead_letter_events INSERT not found in service.go")
	}
	snippet := content[deadLetterStart : deadLetterStart+600]
	// All parameters used inside jsonb_build_object must be explicitly typed.
	if !strings.Contains(snippet, "$5::text") {
		t.Fatal("$5 in dead-letter INSERT is not cast to ::text (causes SQLSTATE 42P08)")
	}
	if !strings.Contains(snippet, "$6::text") {
		t.Fatal("$6 in dead-letter INSERT is not cast to ::text")
	}
	if !strings.Contains(snippet, "$7::text") {
		t.Fatal("$7 in dead-letter INSERT is not cast to ::text")
	}
}
