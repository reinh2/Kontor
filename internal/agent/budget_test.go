package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/reinhlord/kontor/internal/llm"
	"github.com/reinhlord/kontor/internal/tools"
)

func TestConservativeTokenEstimatorReservesWorstCaseProviderRetries(t *testing.T) {
	t.Parallel()
	request := llm.Request{
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
		Tools:           []llm.ToolDefinition{{Name: "first", Parameters: json.RawMessage(`{"type":"object"}`)}},
		MaxOutputTokens: 100,
	}
	oneAttempt, err := (ConservativeTokenEstimator{}).Estimate(request)
	if err != nil {
		t.Fatal(err)
	}
	threeAttempts, err := (ConservativeTokenEstimator{ProviderAttempts: 3}).Estimate(request)
	if err != nil {
		t.Fatal(err)
	}
	if threeAttempts != oneAttempt*3 {
		t.Fatalf("retry reservation = %d, want %d", threeAttempts, oneAttempt*3)
	}
}

func TestMemoryTokenBudgetSettlesAndEnforcesPerConversationCap(t *testing.T) {
	t.Parallel()
	budget, err := NewMemoryTokenBudget(100)
	if err != nil {
		t.Fatal(err)
	}

	reservation, err := budget.Reserve(context.Background(), "conversation-a", 80)
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.Settle(context.Background(), 30); err != nil {
		t.Fatal(err)
	}
	if got := budget.Accounted("conversation-a"); got != 30 {
		t.Fatalf("accounted = %d, want 30", got)
	}
	if _, err := budget.Reserve(context.Background(), "conversation-a", 71); !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("Reserve error = %v, want ErrTokenBudgetExceeded", err)
	}
	if _, err := budget.Reserve(context.Background(), "conversation-b", 100); err != nil {
		t.Fatalf("independent conversation reserve: %v", err)
	}
}

func TestMemoryTokenBudgetCountsConcurrentReservations(t *testing.T) {
	t.Parallel()
	budget, err := NewMemoryTokenBudget(100)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wait.Done()
			<-start
			_, reserveErr := budget.Reserve(context.Background(), "same", 60)
			results <- reserveErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes, refusals := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrTokenBudgetExceeded):
			refusals++
		default:
			t.Fatalf("unexpected reservation error: %v", result)
		}
	}
	if successes != 1 || refusals != 1 {
		t.Fatalf("successes=%d refusals=%d, want 1 and 1", successes, refusals)
	}
}

func TestMemoryTokenBudgetKeepsReservationWhenUsageExceedsIt(t *testing.T) {
	t.Parallel()
	budget, err := NewMemoryTokenBudget(100)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := budget.Reserve(context.Background(), "conversation", 40)
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.Settle(context.Background(), 41); !errors.Is(err, ErrUsageExceedsReservation) {
		t.Fatalf("Settle error = %v, want ErrUsageExceedsReservation", err)
	}
	if got := budget.Accounted("conversation"); got != 40 {
		t.Fatalf("accounted = %d, want full reservation 40", got)
	}
}

func TestConservativeTokenEstimatorRealisticToolSchemaNotInflated(t *testing.T) {
	t.Parallel()
	// Simulate a realistic OpenRouter-sized request: 10 tool definitions with
	// ~500 bytes of JSON schema each, 5 messages totaling ~2KB, max output 1024.
	tools := make([]llm.ToolDefinition, 10)
	for i := range tools {
		// ~500 bytes per schema — representative of real contract definitions.
		schema := `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["service_id","date_from","date_to"],"properties":{"service_id":{"type":"string","format":"uuid"},"staff_id":{"type":"string","format":"uuid"},"date_from":{"type":"string","format":"date-time","pattern":"(Z|[+-][0-9]{2}:[0-9]{2})$"},"date_to":{"type":"string","format":"date-time","pattern":"(Z|[+-][0-9]{2}:[0-9]{2})$"}},"additionalProperties":false}`
		tools[i] = llm.ToolDefinition{
			Name:        "tool_" + string(rune('a'+i)),
			Description: "A description of this tool that is moderately long for testing purposes.",
			Parameters:  json.RawMessage(schema),
		}
	}
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: "You are a booking assistant. Current time is 2026-07-23T10:00:00+02:00."},
		{Role: llm.RoleSystem, Content: "Customer contact on file: yes."},
		{Role: llm.RoleUser, Content: "I'd like to book a haircut tomorrow at 2pm please."},
		{Role: llm.RoleAssistant, Content: "Let me check available slots for you."},
		{Role: llm.RoleUser, Content: "Yes, book it."},
	}
	request := llm.Request{
		Messages:        messages,
		Tools:           tools,
		MaxOutputTokens: 1024,
	}
	estimate, err := (ConservativeTokenEstimator{}).Estimate(request)
	if err != nil {
		t.Fatal(err)
	}
	// With 10 tools × ~500 bytes + messages ~500 bytes = ~5500 bytes content.
	// At 3 bytes/token, that's ~1833 tokens + 1024 max_output + 256 base +
	// 5×64 messages + 10×64 tools = ~4073.
	// The key invariant: a single turn must NOT require more than 10000 tokens
	// from a 50000-token budget; the old 1:1 estimator would produce ~7000+.
	if estimate > 10_000 {
		t.Fatalf("estimate=%d exceeds 10000 for a normal-sized request (would exhaust 50k budget in <5 turns)", estimate)
	}
	// Sanity: should still be conservative (above trivial sum of max_output).
	if estimate < 2_500 {
		t.Fatalf("estimate=%d is too low, should include content + overhead", estimate)
	}
}

func TestConservativeTokenEstimatorKeepsRealProviderRequestWithinBudget(t *testing.T) {
	t.Parallel()
	definitions := tools.Definitions()
	registeredTools := make([]llm.ToolDefinition, len(definitions))
	for i, definition := range definitions {
		registeredTools[i] = llm.ToolDefinition{
			Name:        definition.Name,
			Version:     definition.Version,
			Description: definition.Description,
			Parameters:  definition.Parameters,
		}
	}

	request := llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You are Kontor, the action-taking front desk for Salon Nord."},
			{Role: llm.RoleSystem, Content: "Authenticated customer contact on file: yes."},
			{Role: llm.RoleUser, Content: "25 July colour at 09:00"},
		},
		Tools:           registeredTools,
		MaxOutputTokens: 800,
	}
	estimate, err := (ConservativeTokenEstimator{ProviderAttempts: 3}).Estimate(request)
	if err != nil {
		t.Fatal(err)
	}
	// OpenAI and OpenRouter may retry a request up to three times. Even with
	// that full reservation, a normal first booking request must leave ample
	// capacity in the default 50,000-token conversation budget.
	if estimate >= 15_000 {
		t.Fatalf("estimate=%d, want <15000 for the real tool set with three provider attempts", estimate)
	}
}

func TestConservativeTokenEstimatorBytesPerTokenCustomizable(t *testing.T) {
	t.Parallel()
	request := llm.Request{
		Messages:        []llm.Message{{Role: llm.RoleUser, Content: "hello world"}},
		Tools:           []llm.ToolDefinition{{Name: "tool", Parameters: json.RawMessage(`{"type":"object"}`)}},
		MaxOutputTokens: 100,
	}
	defaultEst, err := (ConservativeTokenEstimator{}).Estimate(request)
	if err != nil {
		t.Fatal(err)
	}
	// With BytesPerToken=1 (old behavior), estimate should be larger.
	rawEst, err := (ConservativeTokenEstimator{BytesPerToken: 1}).Estimate(request)
	if err != nil {
		t.Fatal(err)
	}
	if rawEst <= defaultEst {
		t.Fatalf("BytesPerToken=1 estimate=%d should exceed default estimate=%d", rawEst, defaultEst)
	}
}

func TestConservativeTokenEstimatorProviderFailureChargesFullReservation(t *testing.T) {
	t.Parallel()
	budget, err := NewMemoryTokenBudget(10_000)
	if err != nil {
		t.Fatal(err)
	}
	// Reserve tokens as the runner would.
	reservation, err := budget.Reserve(context.Background(), "conv-failure", 3000)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate provider failure: settle with the full reservation (chargedTokens = reservation).
	if err := reservation.Settle(context.Background(), reservation.ReservedTokens()); err != nil {
		t.Fatal(err)
	}
	// The budget must be charged the full reservation, not zero.
	if got := budget.Accounted("conv-failure"); got != 3000 {
		t.Fatalf("accounted after provider failure=%d, want full reservation 3000", got)
	}
	// Remaining capacity: 10000 - 3000 = 7000. A second 7001-token reservation must fail.
	if _, err := budget.Reserve(context.Background(), "conv-failure", 7001); !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("over-limit reservation err=%v, want ErrTokenBudgetExceeded", err)
	}
	// But 7000 should still succeed.
	if _, err := budget.Reserve(context.Background(), "conv-failure", 7000); err != nil {
		t.Fatalf("within-limit reservation err=%v", err)
	}
}
