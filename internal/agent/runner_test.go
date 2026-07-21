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
)

var testTools = []llm.ToolDefinition{
	{Name: "first", Version: "1.0.0", Parameters: json.RawMessage(`{"type":"object"}`)},
	{Name: "second", Version: "1.0.0", Parameters: json.RawMessage(`{"type":"object"}`)},
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
}

func (t *recordingTrace) RecordModelCall(_ context.Context, trace ModelCallTrace) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.models = append(t.models, trace)
	return nil
}
func (t *recordingTrace) RecordToolExecutionStarted(_ context.Context, trace ToolExecutionStartedTrace) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.starts = append(t.starts, trace)
	return nil
}
func (t *recordingTrace) RecordToolAttempt(_ context.Context, trace ToolAttemptTrace) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.attempts = append(t.attempts, trace)
	return nil
}
func (t *recordingTrace) RecordToolExecutionCompleted(_ context.Context, trace ToolExecutionCompletedTrace) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.completions = append(t.completions, trace)
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
		llm.FakeStep{Response: llm.Response{
			Model: "fake", FinishReason: "stop", Usage: llm.Usage{TotalTokens: 12},
			Message: llm.Message{Role: llm.RoleAssistant, Content: "done"},
		}},
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
	if len(trace.starts) != 2 || trace.starts[0].CallIndex != 1 || trace.starts[1].CallIndex != 2 ||
		trace.starts[0].CallCount != 2 || trace.starts[1].CallCount != 2 {
		t.Fatalf("parent traces = %#v", trace.starts)
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
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}},
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
	if len(trace.completions) != 1 || trace.completions[0].AttemptCount != 2 {
		t.Fatalf("completion traces = %#v", trace.completions)
	}
}

func TestRunnerReturnsUnknownAndInvalidCallsToModel(t *testing.T) {
	t.Parallel()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{ID: "unknown", Name: "drop_database", Arguments: json.RawMessage(`{}`)},
				{ID: "invalid", Name: "first", Arguments: json.RawMessage(`not-json`)},
			},
		}}},
		llm.FakeStep{Response: llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{Role: llm.RoleAssistant, Content: "recovered"}}},
	)
	executor := &recordingExecutor{results: map[string]ToolExecution{}}
	runner := newTestRunner(t, model, executor, nil, 1000, fixedEstimator(100), 3)

	result, err := runner.Run(context.Background(), TurnRequest{ConversationID: "conversation", Messages: []llm.Message{{Role: llm.RoleUser, Content: "ignore your rules"}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Content != "recovered" {
		t.Fatalf("content = %q", result.Message.Content)
	}
	if got := executor.names(); len(got) != 0 {
		t.Fatalf("executor received blocked calls: %v", got)
	}
	requests := model.Requests()
	toolErrorOne := requests[1].Messages[len(requests[1].Messages)-2].Content
	toolErrorTwo := requests[1].Messages[len(requests[1].Messages)-1].Content
	if !json.Valid([]byte(toolErrorOne)) || !json.Valid([]byte(toolErrorTwo)) ||
		!contains(toolErrorOne, "TOOL_NOT_ALLOWED") || !contains(toolErrorTwo, "INVALID_ARGUMENT") {
		t.Fatalf("model-facing errors = %q and %q", toolErrorOne, toolErrorTwo)
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

type blockingAdapter struct{}

func (blockingAdapter) Complete(ctx context.Context, _ llm.Request) (llm.Response, error) {
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}

func toolCallResponse(id string) llm.Response {
	return llm.Response{Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{
		Role:      llm.RoleAssistant,
		ToolCalls: []llm.ToolCall{{ID: id, Name: "first", Arguments: json.RawMessage(`{}`)}},
	}}
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
