// Package llm defines the provider-neutral boundary used by the agent loop.
package llm

import (
	"context"
	"encoding/json"
)

// Role is a chat message role understood by supported model providers.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a provider-neutral chat message. Assistant messages can contain
// more than one ToolCall. Tool responses identify the call they answer through
// ToolCallID.
type Message struct {
	Role       Role
	Content    string
	Name       string
	ToolCallID string
	ToolCalls  []ToolCall
}

// ToolCall is a function call requested by the model. Arguments intentionally
// remain raw JSON: validation and authorization belong to the server-side tool
// gateway, never to the model adapter.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolDefinition describes one server-owned function exposed to the model.
type ToolDefinition struct {
	Name        string
	Version     string
	Description string
	Parameters  json.RawMessage
}

// Request contains one non-streaming model turn.
type Request struct {
	Messages        []Message
	Tools           []ToolDefinition
	MaxOutputTokens int
}

// Usage is the provider-reported token usage for a request.
type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

// Total returns the provider's total when present and otherwise derives it
// from the input and output fields.
func (u Usage) Total() int {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.InputTokens + u.OutputTokens
}

// Response contains the first completion choice returned by a provider.
type Response struct {
	ID           string
	Model        string
	Message      Message
	FinishReason string
	Usage        Usage
}

// Adapter is implemented by real and deterministic model providers.
type Adapter interface {
	Complete(ctx context.Context, request Request) (Response, error)
}
