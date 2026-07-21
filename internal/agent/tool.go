package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/reinhlord/kontor/internal/llm"
)

// ToolRequest is the trusted agent context plus the model-authored call. The
// executor must derive tenant/customer authority from the trusted identifiers,
// never from Arguments.
type ToolRequest struct {
	RunID          string
	ConversationID string
	Iteration      int
	CallIndex      int
	CallCount      int
	Call           llm.ToolCall
}

// ToolAttempt is one local or external attempt made inside a parent tool call.
// Attempts must be returned in execution order; the runner assigns stable,
// one-based AttemptNo values to trace records.
type ToolAttempt struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Status     ToolStatus
	Detail     json.RawMessage
}

// ToolExecution is the complete result of one parent model-requested call.
// Content is sent back to the model as a tool message.
type ToolExecution struct {
	Content  json.RawMessage
	IsError  bool
	Status   ToolStatus
	Attempts []ToolAttempt
}

// ToolExecutor is the sole capability boundary between model requests and
// server-owned tools. Validation, authorization, idempotency, and retries live
// behind this interface.
type ToolExecutor interface {
	Execute(ctx context.Context, request ToolRequest) (ToolExecution, error)
}

// ToolStatus is stored for both parent calls and their nested attempts.
type ToolStatus string

const (
	ToolStatusRunning              ToolStatus = "running"
	ToolStatusSucceeded            ToolStatus = "succeeded"
	ToolStatusFailed               ToolStatus = "failed"
	ToolStatusRefused              ToolStatus = "refused"
	ToolStatusConfirmationRequired ToolStatus = "confirmation_required"

	// Compatibility aliases use the database-aligned values above.
	ToolStatusSuccess = ToolStatusSucceeded
	ToolStatusError   = ToolStatusFailed
)
