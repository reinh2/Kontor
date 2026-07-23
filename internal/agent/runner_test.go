package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/reinhlord/kontor/internal/llm"
	toolapi "github.com/reinhlord/kontor/internal/tools"
)

var testTools = []llm.ToolDefinition{
	{Name: "first", Version: "1.0.0", Parameters: json.RawMessage(`{"type":"object"}`)},
	{Name: "second", Version: "1.0.0", Parameters: json.RawMessage(`{"type":"object"}`)},
	{Name: "escalate_to_human", Version: "1.0.0", Parameters: json.RawMessage(`{"type":"object"}`)},
	{Name: toolapi.ToolRespondToCustomer, Version: "1.0.0", Parameters: json.RawMessage(`{"type":"object"}`)},
}

type fixedEstimator int

func (e fixedEstimator) Estimate(llm.Request) (int, error) { return int(e), nil }

type recordingExecutor struct {
	mu       sync.Mutex
	requests []ToolRequest
	results  map[string]ToolExecution
	errors   map[string]error
}

func (e *recordingExecutor) Execute(_ context.Context, request ToolRequest) (ToolExecution, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.requests = append(e.requests, request)
	return e.results[request.Call.Name], e.errors[request.Call.Name]
}

func (e *recordingExecutor) names() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]string, len(e.requests))
	for i, request := range e.requests {
		result[i] = request.Call.Name
	}
	return result
}

type recordingTrace struct {
	mu          sync.Mutex
	models      []ModelCallTrace
	starts      []ToolExecutionStartedTrace
	attempts    []ToolAttemptTrace
	completions []ToolExecutionCompletedTrace
	running     map[string]struct{}
}

func (t *recordingTrace) RecordModelCall(ctx context.Context, trace ModelCallTrace) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.models = append(t.models, trace)
	return nil
}
func (t *recordingTrace) RecordToolExecutionStarted(ctx context.Context, trace ToolExecutionStartedTrace) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.starts = append(t.starts, trace)
	if t.running == nil {
		t.running = make(map[string]struct{})
	}
	t.running[trace.CallID] = struct{}{}
	return nil
}
func (t *recordingTrace) RecordToolAttempt(ctx context.Context, trace ToolAttemptTrace) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.attempts = append(t.attempts, trace)
	return nil
}
func (t *recordingTrace) RecordToolExecutionCompleted(ctx context.Context, trace ToolExecutionCompletedTrace) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.completions = append(t.completions, trace)
	delete(t.running, trace.CallID)
	return nil
}

func TestRunnerExecutesEveryToolCallInResponseOrder(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{
			Model: "fake", FinishReason: "tool_calls", Usage: llm.Usage{TotalTokens: 20},
			Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "call-1", Name: "first", Arguments: json.RawMessage(`{"value":1}`)},
				{ID: "call-2", Name: "second", Arguments: json.RawMessage(`{"value":2}`)},
			}},
		}},
		llm.FakeStep{Response: customerResponse("terminal-done", "done", toolapi.ResponseComplete)},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"first":  {Content: json.RawMessage(`{"result":1}`)},
		"second": {Content: json.RawMessage(`{"result":2}`)},
	}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 4)

	result, err := runner.Run(context.Background(), TurnRequest{
		RunID: "run-1", ConversationID: "conversation-1",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "do both"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "done" || result.Iterations != 2 {
		t.Fatalf("result = %#v", result)
	}
	if got := executor.names(); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("execution order = %v", got)
	}

	requests := model.Requests()
	if len(requests) != 2 {
		t.Fatalf("model calls = %d, want 2", len(requests))
	}
	secondHistory := requests[1].Messages
	if len(secondHistory) != 4 {
		t.Fatalf("second request messages = %d, want 4", len(secondHistory))
	}
	if secondHistory[1].Role != llm.RoleAssistant || len(secondHistory[1].ToolCalls) != 2 ||
		secondHistory[2].ToolCallID != "call-1" || secondHistory[3].ToolCallID != "call-2" {
		t.Fatalf("second request did not preserve tool batch order: %#v", secondHistory)
	}
	if len(trace.starts) != 3 || trace.starts[0].CallIndex != 1 || trace.starts[1].CallIndex != 2 ||
		trace.starts[0].CallCount != 2 || trace.starts[1].CallCount != 2 {
		t.Fatalf("parent traces = %#v", trace.starts)
	}
	if trace.starts[2].ToolName != toolapi.ToolRespondToCustomer || trace.starts[2].CallCount != 1 ||
		len(trace.attempts) != 2 || trace.completions[2].AttemptCount != 0 {
		t.Fatalf("terminal trace = starts=%#v attempts=%#v completions=%#v", trace.starts, trace.attempts, trace.completions)
	}
}

func TestRunnerRecoversFromSchemaInvalidCallWithClarification(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, FinishReason: "tool_calls", Message: llm.Message{
			Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: "invalid-create", Name: "first", Arguments: json.RawMessage(`{}`),
			}},
		}}},
		llm.FakeStep{Response: customerResponse("ask-contact", "Please share an email or phone number.", toolapi.ResponseClarificationNeeded)},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"first": {
			Content: json.RawMessage(`{"status":"error","error":{"code":"INVALID_ARGUMENT","resolution":"fix_arguments"}}`),
			IsError: true, Status: ToolStatusFailed,
		},
	}}
	runner := newTestRunner(t, model, executor, &recordingTrace{}, 1000, fixedEstimator(100), 4)
	result, err := runner.Run(context.Background(), TurnRequest{
		RunID: "schema-recovery", ConversationID: "conversation-1",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Book a haircut"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CustomerResponseDisposition != toolapi.ResponseClarificationNeeded ||
		result.Message.Content != "Please share an email or phone number." {
		t.Fatalf("schema recovery result=%#v", result)
	}
	if got := executor.names(); !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("schema-invalid call was retried instead of clarified: %v", got)
	}
	requests := model.Requests()
	if len(requests) != 2 || len(requests[1].Messages) == 0 ||
		!contains(requests[1].Messages[len(requests[1].Messages)-1].Content, `"resolution":"fix_arguments"`) {
		t.Fatalf("second model request did not receive the schema error: %#v", requests)
	}
}

func TestRunnerAcceptsSoleStructuredTerminalResponseOnFinalIteration(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(llm.FakeStep{Response: customerResponse(
		"terminal-clarification", "Which service would you like?", toolapi.ResponseClarificationNeeded,
	)})
	executor := &recordingExecutor{results: map[string]ToolExecution{}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 1)

	result, err := runner.Run(context.Background(), TurnRequest{
		RunID: "terminal-run", ConversationID: "conversation",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Book something"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "Which service would you like?" ||
		result.CustomerResponseDisposition != toolapi.ResponseClarificationNeeded ||
		result.CustomerResponseToolCallID != "terminal-clarification" || result.Iterations != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := executor.names(); len(got) != 0 {
		t.Fatalf("terminal control call reached executor: %v", got)
	}
	requests := model.Requests()
	if len(requests) != 1 || requests[0].ToolChoice != llm.ToolChoiceAuto {
		t.Fatalf("model requests = %#v", requests)
	}
	if len(trace.starts) != 1 || len(trace.attempts) != 0 || len(trace.completions) != 1 ||
		trace.completions[0].AttemptCount != 0 || trace.completions[0].Status != ToolStatusSucceeded {
		t.Fatalf("terminal trace = starts=%#v attempts=%#v completions=%#v", trace.starts, trace.attempts, trace.completions)
	}
}

func TestRunnerDiscardsUnstructuredTextAndCorrectsProtocol(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{
			Usage: llm.Usage{TotalTokens: 10},
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: `I am sorry, "colour" is not a valid service. 17744784-622a-447c-a42b-1207c146d15a`,
			},
		}},
		llm.FakeStep{Response: customerResponse(
			"terminal", "Which service would you like?", toolapi.ResponseClarificationNeeded,
		)},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 3)

	result, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "25 july colour on 09:00"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "Which service would you like?" || result.Iterations != 2 {
		t.Fatalf("unstructured text was delivered instead of corrected: %#v", result)
	}
	if contains(result.Message.Content, "17744784") {
		t.Fatalf("hallucinated identifier reached the customer: %q", result.Message.Content)
	}
	requests := model.Requests()
	if len(requests) != 2 || len(requests[1].Messages) == 0 {
		t.Fatalf("model requests = %#v", requests)
	}
	last := requests[1].Messages[len(requests[1].Messages)-1]
	if last.Role != llm.RoleUser || !contains(last.Content, "PROTOCOL ERROR") {
		t.Fatalf("second request did not carry the protocol correction: %#v", last)
	}
}

func TestRunnerStopsActingAfterAConfirmationProposal(t *testing.T) {
	t.Parallel()
	propose := llm.FakeStep{Response: llm.Response{
		Usage: llm.Usage{TotalTokens: 10}, FinishReason: "tool_calls",
		Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "propose", Name: "first", Arguments: json.RawMessage(`{}`)},
			{ID: "sibling", Name: "second", Arguments: json.RawMessage(`{}`)},
		}},
	}}
	// A real model kept calling tools after the gateway froze the proposal.
	keepActing := llm.FakeStep{Response: llm.Response{
		Usage: llm.Usage{TotalTokens: 10}, FinishReason: "tool_calls",
		Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "extra", Name: "second", Arguments: json.RawMessage(`{}`)},
		}},
	}}
	model := llm.NewFakeAdapter(propose, keepActing, llm.FakeStep{Response: customerResponse(
		"terminal", "Shall I book Colour with Nadia P. on 25 July at 09:00?", toolapi.ResponseComplete,
	)})
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"first":  {Content: json.RawMessage(`{"status":"confirmation_required"}`), Status: ToolStatusConfirmationRequired},
		"second": {Content: json.RawMessage(`{"status":"success"}`), Status: ToolStatusSucceeded},
	}}
	runner := newTestRunner(t, model, executor, &recordingTrace{}, 1000, fixedEstimator(100), 6)

	result, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "book it"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Only the proposing call may run: the sibling in its batch and every later
	// call belong to the customer's decision, not the model's.
	if got := executor.names(); !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("tools executed after the proposal: %v", got)
	}
	if result.Message.Content != "Shall I book Colour with Nadia P. on 25 July at 09:00?" {
		t.Fatalf("result = %#v", result)
	}
	requests := model.Requests()
	if len(requests) != 3 {
		t.Fatalf("model calls = %d, want 3", len(requests))
	}
	last := requests[2].Messages[len(requests[2].Messages)-1]
	if !contains(last.Content, "already produced a proposal") {
		t.Fatalf("third request did not carry the proposal correction: %#v", last)
	}
}

func TestRunnerFailsClosedOnRepeatedUnstructuredText(t *testing.T) {
	t.Parallel()
	textOnly := llm.FakeStep{Response: llm.Response{
		Usage:   llm.Usage{TotalTokens: 10},
		Message: llm.Message{Role: llm.RoleAssistant, Content: "Please choose from the available services."},
	}}
	model := llm.NewFakeAdapter(textOnly, textOnly, textOnly, textOnly)
	executor := &recordingExecutor{results: map[string]ToolExecution{}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 6)

	result, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "unclear"}},
	})
	if !errors.Is(err, ErrTerminalProtocol) {
		t.Fatalf("Run error = %v, want ErrTerminalProtocol", err)
	}
	if result.Message.Content != "" {
		t.Fatalf("failed turn produced a customer-facing message: %#v", result.Message)
	}
	if len(model.Requests()) != maxProtocolCorrections+1 {
		t.Fatalf("model was re-prompted %d time(s), want %d", len(model.Requests()), maxProtocolCorrections+1)
	}
	if len(executor.names()) != 0 || len(trace.starts) != 0 || len(trace.completions) != 0 {
		t.Fatalf("unstructured response reached tools: executor=%v starts=%#v completions=%#v", executor.names(), trace.starts, trace.completions)
	}
}

func TestRunnerRejectsInvalidStructuredTerminalArguments(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(llm.FakeStep{Response: llm.Response{
		Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
			ID: "invalid-terminal", Name: toolapi.ToolRespondToCustomer,
			Arguments: json.RawMessage(`{"disposition":"complete","message":"ok","extra":true}`),
		}}},
	}})
	executor := &recordingExecutor{results: map[string]ToolExecution{}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 2)

	_, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}},
	})
	if !errors.Is(err, ErrTerminalProtocol) {
		t.Fatalf("Run error = %v, want ErrTerminalProtocol", err)
	}
	if len(executor.names()) != 0 || len(trace.starts) != 1 || len(trace.attempts) != 0 ||
		len(trace.completions) != 1 || trace.completions[0].Status != ToolStatusFailed || trace.completions[0].AttemptCount != 0 {
		t.Fatalf("invalid terminal trace/execution: executor=%v starts=%#v attempts=%#v completions=%#v",
			executor.names(), trace.starts, trace.attempts, trace.completions)
	}
}

func TestRunnerRejectsMixedTerminalBatchBeforeExecutingAnySibling(t *testing.T) {
	t.Parallel()
	for _, terminalFirst := range []bool{true, false} {
		terminalFirst := terminalFirst
		t.Run(map[bool]string{true: "terminal_first", false: "terminal_last"}[terminalFirst], func(t *testing.T) {
			t.Parallel()
			terminal := llm.ToolCall{
				ID: "terminal", Name: toolapi.ToolRespondToCustomer,
				Arguments: json.RawMessage(`{"disposition":"complete","message":"done"}`),
			}
			mutation := llm.ToolCall{ID: "mutation", Name: "first", Arguments: json.RawMessage(`{}`)}
			calls := []llm.ToolCall{mutation, terminal}
			if terminalFirst {
				calls = []llm.ToolCall{terminal, mutation}
			}
			model := llm.NewFakeAdapter(llm.FakeStep{Response: llm.Response{
				Usage:   llm.Usage{TotalTokens: 10},
				Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: calls},
			}})
			executor := &recordingExecutor{results: map[string]ToolExecution{
				"first": {Content: json.RawMessage(`{"status":"success"}`), Status: ToolStatusSucceeded},
			}}
			trace := &recordingTrace{}
			runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 2)

			_, err := runner.Run(context.Background(), TurnRequest{
				ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}},
			})
			if !errors.Is(err, ErrTerminalProtocol) {
				t.Fatalf("Run error = %v, want ErrTerminalProtocol", err)
			}
			if len(executor.names()) != 0 || len(trace.starts) != 0 || len(trace.attempts) != 0 || len(trace.completions) != 0 {
				t.Fatalf("mixed batch executed or traced tools: executor=%v starts=%#v attempts=%#v completions=%#v",
					executor.names(), trace.starts, trace.attempts, trace.completions)
			}
		})
	}
}

func TestNormalizeToolCallIDsAvoidsProviderAndFallbackCollisions(t *testing.T) {
	t.Parallel()
	original := []llm.ToolCall{
		{ID: "model-call-7-3", Name: "one"},
		{ID: "model-call-7-3-2", Name: "two"},
		{ID: "model-call-7-3", Name: "three"},
		{ID: "", Name: "four"},
		{ID: "provider", Name: "five"},
		{ID: "provider", Name: "six"},
		{ID: "model-call-7-4", Name: "seven"},
	}
	wantIDs := []string{
		"model-call-7-3",
		"model-call-7-3-2",
		"model-call-7-3-3",
		"model-call-7-4",
		"provider",
		"model-call-7-6",
		"model-call-7-7",
	}

	for run := 0; run < 2; run++ {
		calls := append([]llm.ToolCall(nil), original...)
		normalizeToolCallIDs(calls, 7, make(map[string]struct{}))

		gotIDs := make([]string, len(calls))
		seen := make(map[string]struct{}, len(calls))
		for i, call := range calls {
			gotIDs[i] = call.ID
			if call.Name != original[i].Name {
				t.Fatalf("call order changed at index %d: got %q, want %q", i, call.Name, original[i].Name)
			}
			if _, duplicate := seen[call.ID]; duplicate {
				t.Fatalf("normalized ID %q is duplicated: %v", call.ID, gotIDs)
			}
			seen[call.ID] = struct{}{}
		}
		if !reflect.DeepEqual(gotIDs, wantIDs) {
			t.Fatalf("normalized IDs = %v, want %v", gotIDs, wantIDs)
		}
	}
}

func TestRunnerTracesNestedAttemptsWithOneBasedAttemptNumbers(t *testing.T) {
	t.Parallel()
	now := time.Now()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
			Role:      llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{ID: "retry-call", Name: "first", Arguments: json.RawMessage(`{}`)}},
		}}},
		llm.FakeStep{Response: customerResponse("terminal-ok", "ok", toolapi.ResponseComplete)},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"first": {
			Content: json.RawMessage(`{"status":"success"}`),
			Attempts: []ToolAttempt{
				{StartedAt: now, FinishedAt: now.Add(time.Millisecond), Status: ToolStatusFailed, Detail: json.RawMessage(`{"status":503}`)},
				{StartedAt: now.Add(time.Second), FinishedAt: now.Add(time.Second + time.Millisecond), Status: ToolStatusSucceeded, Detail: json.RawMessage(`{"count":3}`)},
			},
		},
	}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 3)

	if _, err := runner.Run(context.Background(), TurnRequest{ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}}}); err != nil {
		t.Fatal(err)
	}
	if len(trace.attempts) != 2 {
		t.Fatalf("attempt traces = %d, want 2", len(trace.attempts))
	}
	if trace.attempts[0].AttemptNo != 1 || trace.attempts[1].AttemptNo != 2 ||
		trace.attempts[0].CallID != "retry-call" || trace.attempts[1].CallID != "retry-call" {
		t.Fatalf("attempt traces = %#v", trace.attempts)
	}
	if len(trace.completions) != 2 || trace.completions[0].AttemptCount != 2 ||
		trace.completions[1].ToolName != toolapi.ToolRespondToCustomer || trace.completions[1].AttemptCount != 0 {
		t.Fatalf("completion traces = %#v", trace.completions)
	}
}

func TestRunnerRecoversFromUnknownCallWithClarification(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
			Role:      llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{ID: "unknown", Name: "default_api.list_available_slots", Arguments: json.RawMessage(`{}`)}},
		}}},
		llm.FakeStep{Response: customerResponse("terminal-clarification", "Which service would you like?", toolapi.ResponseClarificationNeeded)},
	)
	executor := &recordingExecutor{}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 3)

	result, err := runner.Run(context.Background(), TurnRequest{ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "25 July colour at 09:00"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "Which service would you like?" {
		t.Fatalf("content = %q", result.Message.Content)
	}
	if result.ToolRefused || result.HumanEscalated {
		t.Fatalf("unknown tool call should remain recoverable: %#v", result)
	}
	if got := executor.names(); len(got) != 0 {
		t.Fatalf("executor received blocked calls: %v", got)
	}
	if got := len(model.Requests()); got != 2 {
		t.Fatalf("provider calls = %d, want 2 after the recoverable tool error", got)
	}
	if len(trace.completions) != 2 || trace.completions[0].Status != ToolStatusFailed ||
		!contains(string(trace.completions[0].Result), "TOOL_NOT_ALLOWED") ||
		trace.completions[1].ToolName != toolapi.ToolRespondToCustomer {
		t.Fatalf("recovery trace = %#v", trace.completions)
	}
}

func TestRunnerNormalizesDefaultAPINamespaceForRegisteredTool(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
			Role:      llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{ID: "namespaced", Name: "default_api.first", Arguments: json.RawMessage(`{}`)}},
		}}},
		llm.FakeStep{Response: customerResponse("terminal", "done", toolapi.ResponseComplete)},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"first": {Content: json.RawMessage(`{"status":"success"}`), Status: ToolStatusSucceeded},
	}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 3)

	result, err := runner.Run(context.Background(), TurnRequest{ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "done" {
		t.Fatalf("content = %q", result.Message.Content)
	}
	if got := executor.names(); !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("executor tool names = %v, want [first]", got)
	}
	if len(trace.completions) != 2 || trace.completions[0].ToolName != "first" || trace.completions[0].Status != ToolStatusSucceeded {
		t.Fatalf("tool trace = %#v", trace.completions)
	}
}

func TestRunnerEscalatesExecutorPolicyRefusal(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{ID: "policy-refusal", Name: "first", Arguments: json.RawMessage(`{}`)}},
	}}})
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"first": {Content: json.RawMessage(`{"status":"error"}`), IsError: true, Status: ToolStatusRefused},
	}}
	runner := newTestRunner(t, model, executor, nil, 1000, fixedEstimator(100), 3)

	result, err := runner.Run(context.Background(), TurnRequest{ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "forbidden request"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ToolRefused || result.HumanEscalated {
		t.Fatalf("policy refusal was not escalated: %#v", result)
	}
}

func TestRunnerSkipsMutationAfterSuccessfulHumanEscalation(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(llm.FakeStep{Response: llm.Response{
		Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: "handoff", Name: "escalate_to_human", Arguments: json.RawMessage(`{}`)},
				{ID: "mutation", Name: "first", Arguments: json.RawMessage(`{}`)},
			},
		},
	}})
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"escalate_to_human": {Content: json.RawMessage(`{"status":"success"}`), Status: ToolStatusSucceeded},
		"first":             {Content: json.RawMessage(`{"status":"success"}`), Status: ToolStatusSucceeded},
	}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 3)

	result, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "human please"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.HumanEscalated || result.ToolRefused {
		t.Fatalf("escalation flags = %#v", result)
	}
	if got := executor.names(); !reflect.DeepEqual(got, []string{"escalate_to_human"}) {
		t.Fatalf("executed tools = %v, want handoff only", got)
	}
	if len(model.Requests()) != 1 || len(trace.completions) != 2 ||
		trace.completions[1].Status != ToolStatusRefused || trace.completions[1].AttemptCount != 0 ||
		!contains(string(trace.completions[1].Result), "SKIPPED_AFTER_HANDOFF") {
		t.Fatalf("terminal handoff trace = %#v", trace.completions)
	}
}

func TestRunnerNormalizesToolCallIDsAcrossIterations(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: toolCallResponse("provider-reused-id")},
		llm.FakeStep{Response: toolCallResponse("provider-reused-id")},
		llm.FakeStep{Response: customerResponse("terminal-cross-iteration", "done", toolapi.ResponseComplete)},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"first": {Content: json.RawMessage(`{"status":"success"}`), Status: ToolStatusSucceeded},
	}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 4)

	result, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "repeat"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "done" || len(trace.starts) != 3 {
		t.Fatalf("result=%#v starts=%#v", result, trace.starts)
	}
	if trace.starts[0].CallID != "provider-reused-id" || trace.starts[1].CallID == "provider-reused-id" ||
		trace.starts[0].CallID == trace.starts[1].CallID {
		t.Fatalf("cross-iteration call IDs = %q, %q", trace.starts[0].CallID, trace.starts[1].CallID)
	}
}

func TestRunnerSurfacesSuccessfulHumanEscalation(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
			Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: "escalate-call", Name: "escalate_to_human", Arguments: json.RawMessage(`{}`),
			}},
		}}},
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
			Role: llm.RoleAssistant, Content: "A person will follow up.",
		}}},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"escalate_to_human": {
			Content: json.RawMessage(`{"status":"success"}`), Status: ToolStatusSucceeded,
		},
	}}
	runner := newTestRunner(t, model, executor, nil, 1000, fixedEstimator(100), 3)
	result, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "human please"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.HumanEscalated || result.ToolRefused {
		t.Fatalf("escalation flags=%#v", result)
	}
}

func TestRunnerRefusesModelCallBeyondConversationBudget(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 60}, Message: llm.Message{
			Role:      llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{ID: "call", Name: "first", Arguments: json.RawMessage(`{}`)}},
		}}},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{"first": {Content: json.RawMessage(`{}`)}}}
	runner := newTestRunner(t, model, executor, nil, 100, fixedEstimator(80), 3)

	_, err := runner.Run(context.Background(), TurnRequest{ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}}})
	if !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("Run error = %v, want ErrTokenBudgetExceeded", err)
	}
	if got := len(model.Requests()); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
}

func TestRunnerChargesFullReservationWhenNoUsageIsReported(t *testing.T) {
	t.Parallel()
	terminal := customerResponse("terminal-ambiguous-usage", "done", toolapi.ResponseComplete)
	terminal.Usage = llm.Usage{}
	terminal.UsageIncomplete = true
	model := llm.NewFakeAdapter(llm.FakeStep{Response: terminal})
	budget, err := NewMemoryTokenBudget(1_000)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(Config{
		MaxIterations: 2, TurnTimeout: time.Second,
		MaxOutputTokensPerCall: 100, ConversationTokenLimit: 1_000,
	}, Dependencies{Model: model, Budget: budget, TokenEstimator: fixedEstimator(300)}, testTools)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "ambiguous-usage", Messages: []llm.Message{{Role: llm.RoleUser, Content: "go"}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := budget.Accounted("ambiguous-usage"); got != 300 {
		t.Fatalf("accounted tokens=%d, want full reservation 300", got)
	}
}

func TestChargeForModelCall(t *testing.T) {
	t.Parallel()
	const reserved = 30_000
	for _, test := range []struct {
		name     string
		response llm.Response
		modelErr error
		want     int
	}{
		{
			name:     "clean usage is charged as reported",
			response: llm.Response{Usage: llm.Usage{InputTokens: 10_000, OutputTokens: 400}},
			want:     10_400,
		},
		{
			name:     "provider error writes off the reservation",
			response: llm.Response{Usage: llm.Usage{InputTokens: 10_000}},
			modelErr: errors.New("provider down"),
			want:     reserved,
		},
		{
			name:     "no reported usage writes off the reservation",
			response: llm.Response{UsageIncomplete: true},
			want:     reserved,
		},
		{
			// The observed regression: one failed attempt used to bill 30 000 for
			// a call whose successful attempt reported 10 400.
			name: "one unaccounted attempt costs its prompt, not the reservation",
			response: llm.Response{
				Usage:           llm.Usage{InputTokens: 10_000, OutputTokens: 400},
				UsageIncomplete: true, UnknownUsageAttempts: 1,
			},
			want: 20_400,
		},
		{
			name: "unaccounted attempts never exceed the reservation",
			response: llm.Response{
				Usage:           llm.Usage{InputTokens: 10_000, OutputTokens: 400},
				UsageIncomplete: true, UnknownUsageAttempts: 5,
			},
			want: reserved,
		},
		{
			name: "a total-only provider prices each unaccounted attempt at the total",
			response: llm.Response{
				Usage:           llm.Usage{TotalTokens: 1_000},
				UsageIncomplete: true, UnknownUsageAttempts: 2,
			},
			want: 3_000,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := chargeForModelCall(test.response, test.modelErr, reserved); got != test.want {
				t.Fatalf("charge = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRunnerKeepsBookingCommittedSignalAcrossLaterProviderFailure(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
			Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: "committed", Name: "create_booking", Arguments: json.RawMessage(`{}`),
			}},
		}}},
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 3}}, Err: errors.New("provider failed after commit")},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"create_booking": {
			Content: json.RawMessage(`{"status":"success"}`), Status: ToolStatusSucceeded,
			SideEffectCommitted: true,
		},
	}}
	definitions := append(cloneToolDefinitions(testTools), llm.ToolDefinition{
		Name: "create_booking", Version: "1.0.0", Parameters: json.RawMessage(`{"type":"object"}`),
	})
	budget, err := NewMemoryTokenBudget(1_000)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(Config{
		MaxIterations: 3, TurnTimeout: time.Second,
		MaxOutputTokensPerCall: 100, ConversationTokenLimit: 1_000,
	}, Dependencies{
		Model: model, ToolExecutor: executor, Budget: budget, TokenEstimator: fixedEstimator(100),
	}, definitions)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "book"}},
	})
	if err == nil || !result.BookingCommitted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestRunnerEnforcesIterationLimit(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: toolCallResponse("call-1")},
		llm.FakeStep{Response: toolCallResponse("call-2")},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{"first": {Content: json.RawMessage(`{}`)}}}
	runner := newTestRunner(t, model, executor, nil, 1000, fixedEstimator(100), 2)

	_, err := runner.Run(context.Background(), TurnRequest{ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "loop"}}})
	if !errors.Is(err, ErrIterationLimit) {
		t.Fatalf("Run error = %v, want ErrIterationLimit", err)
	}
	if got := len(model.Requests()); got != 2 {
		t.Fatalf("provider calls = %d, want 2", got)
	}
}

func TestRunnerDoesNotExecuteToolBatchOnFinalIteration(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(llm.FakeStep{Response: toolCallResponse("must-not-run")})
	executor := &recordingExecutor{results: map[string]ToolExecution{
		"first": {Content: json.RawMessage(`{"side_effect":"created"}`)},
	}}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 1)

	result, err := runner.Run(context.Background(), TurnRequest{
		ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "act"}},
	})
	if !errors.Is(err, ErrIterationLimit) {
		t.Fatalf("Run error = %v, want ErrIterationLimit", err)
	}
	if got := executor.names(); len(got) != 0 {
		t.Fatalf("final-iteration tools executed: %v", got)
	}
	if result.Iterations != 1 || len(result.Messages) != 2 || len(result.Messages[1].ToolCalls) != 1 {
		t.Fatalf("result did not retain the model request: %#v", result)
	}

	trace.mu.Lock()
	defer trace.mu.Unlock()
	if len(trace.models) != 1 || trace.models[0].ReturnedToolCallCount != 1 {
		t.Fatalf("model traces = %#v", trace.models)
	}
	if len(trace.starts) != 0 || len(trace.attempts) != 0 || len(trace.completions) != 0 || len(trace.running) != 0 {
		t.Fatalf("tool trace claimed an unexecuted side effect: starts=%d attempts=%d completions=%d running=%d",
			len(trace.starts), len(trace.attempts), len(trace.completions), len(trace.running))
	}
}

func TestRunnerEnforcesWholeTurnTimeout(t *testing.T) {
	t.Parallel()
	model := blockingAdapter{}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, nil, trace, 1000, fixedEstimator(100), 2)
	runner.config.TurnTimeout = 20 * time.Millisecond

	_, err := runner.Run(context.Background(), TurnRequest{ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "wait"}}})
	if !errors.Is(err, ErrTurnTimeout) {
		t.Fatalf("Run error = %v, want ErrTurnTimeout", err)
	}
	if len(trace.models) != 1 || trace.models[0].Status != ModelCallFailed || trace.models[0].ErrorMessage == "" {
		t.Fatalf("failed model trace = %#v", trace.models)
	}
}

func TestRunnerClosesToolTraceAfterWholeTurnTimeout(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(llm.FakeStep{Response: toolCallResponse("timed-out-tool")})
	executor := deadlineReturningExecutor{}
	trace := &recordingTrace{}
	runner := newTestRunner(t, model, executor, trace, 1000, fixedEstimator(100), 2)
	runner.config.TurnTimeout = 40 * time.Millisecond

	type outcome struct {
		err     error
		elapsed time.Duration
	}
	done := make(chan outcome, 1)
	go func() {
		started := time.Now()
		_, err := runner.Run(context.Background(), TurnRequest{
			RunID: "timed-out-run", ConversationID: "conversation",
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "block in tool"}},
		})
		done <- outcome{err: err, elapsed: time.Since(started)}
	}()

	var result outcome
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the whole-turn timeout")
	}
	if !errors.Is(result.err, ErrTurnTimeout) {
		t.Fatalf("Run error = %v, want ErrTurnTimeout", result.err)
	}
	if result.elapsed < runner.config.TurnTimeout/2 {
		t.Fatalf("Run returned before the blocking tool timeout: elapsed %v", result.elapsed)
	}

	trace.mu.Lock()
	defer trace.mu.Unlock()
	if len(trace.starts) != 1 || len(trace.completions) != 1 {
		t.Fatalf("parent traces: starts=%d completions=%d", len(trace.starts), len(trace.completions))
	}
	if trace.starts[0].CallID != trace.completions[0].CallID || len(trace.running) != 0 {
		t.Fatalf("parent remains running: started=%q completed=%q running=%v",
			trace.starts[0].CallID, trace.completions[0].CallID, trace.running)
	}
	if trace.completions[0].Status != ToolStatusFailed || trace.completions[0].AttemptCount != 2 {
		t.Fatalf("completion trace = %#v", trace.completions[0])
	}
	if len(trace.attempts) != 2 || trace.attempts[0].AttemptNo != 1 || trace.attempts[1].AttemptNo != 2 {
		t.Fatalf("attempt traces = %#v", trace.attempts)
	}
}

type blockingAdapter struct{}

func (blockingAdapter) Complete(ctx context.Context, _ llm.Request) (llm.Response, error) {
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}

type deadlineReturningExecutor struct{}

func (deadlineReturningExecutor) Execute(ctx context.Context, _ ToolRequest) (ToolExecution, error) {
	started := time.Now()
	<-ctx.Done()
	finished := time.Now()
	return ToolExecution{
		Content: json.RawMessage(`{"status":"error"}`),
		IsError: true,
		Status:  ToolStatusFailed,
		Attempts: []ToolAttempt{
			{StartedAt: started, FinishedAt: finished, Status: ToolStatusFailed, Detail: json.RawMessage(`{"attempt":1}`)},
			{StartedAt: started, FinishedAt: finished, Status: ToolStatusFailed, Detail: json.RawMessage(`{"attempt":2}`)},
		},
	}, ctx.Err()
}

func toolCallResponse(id string) llm.Response {
	return llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{ID: id, Name: "first", Arguments: json.RawMessage(`{}`)}},
	}}
}

func customerResponse(id, message string, disposition toolapi.CustomerResponseDisposition) llm.Response {
	arguments, _ := json.Marshal(toolapi.RespondToCustomerArguments{
		Disposition: disposition,
		Message:     message,
	})
	return llm.Response{
		Usage: llm.Usage{TotalTokens: 10}, FinishReason: "tool_calls",
		Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
			ID: id, Name: toolapi.ToolRespondToCustomer, Arguments: arguments,
		}}},
	}
}

func newTestRunner(
	t *testing.T,
	model llm.Adapter,
	executor ToolExecutor,
	trace TraceSink,
	budgetLimit int,
	estimator TokenEstimator,
	iterations int,
) *Runner {
	t.Helper()
	budget, err := NewMemoryTokenBudget(budgetLimit)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(Config{
		MaxIterations:          iterations,
		TurnTimeout:            time.Second,
		MaxOutputTokensPerCall: 100,
		ConversationTokenLimit: budgetLimit,
	}, Dependencies{
		Model: model, ToolExecutor: executor, Trace: trace, Budget: budget, TokenEstimator: estimator,
	}, testTools)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func contains(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
