package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/reinhlord/kontor/internal/llm"
)

// ModelCallTrace records one model iteration and its budget charge.
type ModelCallTrace struct {
	RunID                 string
	ConversationID        string
	Iteration             int
	StartedAt             time.Time
	FinishedAt            time.Time
	Model                 string
	FinishReason          string
	Usage                 llm.Usage
	ReservedTokens        int
	ChargedTokens         int
	ReturnedToolCallCount int
	Status                ModelCallStatus
	ErrorMessage          string
}

// ModelCallStatus distinguishes completed provider calls from failures.
type ModelCallStatus string

const (
	ModelCallSucceeded ModelCallStatus = "succeeded"
	ModelCallFailed    ModelCallStatus = "failed"
)

// ToolExecutionStartedTrace is the parent record for a model-requested tool
// call. CallIndex and CallCount preserve order within a multi-call response.
type ToolExecutionStartedTrace struct {
	RunID           string
	ConversationID  string
	Iteration       int
	CallIndex       int
	CallCount       int
	CallID          string
	ToolName        string
	ContractVersion string
	Arguments       json.RawMessage
	StartedAt       time.Time
}

// ToolAttemptTrace is nested below its parent call through CallID. AttemptNo
// is one-based and monotonically increasing for each parent call.
type ToolAttemptTrace struct {
	RunID           string
	ConversationID  string
	Iteration       int
	CallIndex       int
	CallID          string
	ToolName        string
	ContractVersion string
	AttemptNo       int
	StartedAt       time.Time
	FinishedAt      time.Time
	Status          ToolStatus
	Detail          json.RawMessage
}

// ToolExecutionCompletedTrace closes the parent call after all attempts have
// been recorded.
type ToolExecutionCompletedTrace struct {
	RunID           string
	ConversationID  string
	Iteration       int
	CallIndex       int
	CallID          string
	ToolName        string
	ContractVersion string
	StartedAt       time.Time
	FinishedAt      time.Time
	Status          ToolStatus
	Result          json.RawMessage
	AttemptCount    int
}

// TraceSink persists the causal trace. A tool parent is recorded before the
// executor is invoked, attempts are then nested under that parent, and the
// parent is closed last.
type TraceSink interface {
	RecordModelCall(ctx context.Context, trace ModelCallTrace) error
	RecordToolExecutionStarted(ctx context.Context, trace ToolExecutionStartedTrace) error
	RecordToolAttempt(ctx context.Context, trace ToolAttemptTrace) error
	RecordToolExecutionCompleted(ctx context.Context, trace ToolExecutionCompletedTrace) error
}

type noopTraceSink struct{}

func (noopTraceSink) RecordModelCall(context.Context, ModelCallTrace) error { return nil }
func (noopTraceSink) RecordToolExecutionStarted(context.Context, ToolExecutionStartedTrace) error {
	return nil
}
func (noopTraceSink) RecordToolAttempt(context.Context, ToolAttemptTrace) error { return nil }
func (noopTraceSink) RecordToolExecutionCompleted(context.Context, ToolExecutionCompletedTrace) error {
	return nil
}
