// Package agent implements Kontor's bounded LLM-to-tool loop.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/reinhlord/kontor/internal/llm"
)

const (
	maxToolArgumentBytes = 64 << 10
	maxToolResultBytes   = 256 << 10
)

var (
	ErrIterationLimit = errors.New("agent: iteration limit reached")
	ErrTurnTimeout    = errors.New("agent: turn deadline exceeded")
)

// Config contains hard safety limits for the agent loop.
type Config struct {
	MaxIterations          int
	TurnTimeout            time.Duration
	MaxOutputTokensPerCall int
	ConversationTokenLimit int
}

// Dependencies are the replaceable model, tool, trace, and budget boundaries.
type Dependencies struct {
	Model          llm.Adapter
	ToolExecutor   ToolExecutor
	Trace          TraceSink
	Budget         TokenBudget
	TokenEstimator TokenEstimator
}

// Runner executes bounded turns. It is safe for concurrent use when its
// injected dependencies are safe for concurrent use.
type Runner struct {
	config          Config
	model           llm.Adapter
	executor        ToolExecutor
	trace           TraceSink
	budget          TokenBudget
	tokenEstimator  TokenEstimator
	tools           []llm.ToolDefinition
	allowedToolName map[string]struct{}
	toolVersions    map[string]string
	now             func() time.Time
}

// NewRunner validates the tool registry and installs safe defaults for traces,
// token estimation, and the in-memory per-conversation hard budget.
func NewRunner(config Config, dependencies Dependencies, tools []llm.ToolDefinition) (*Runner, error) {
	if config.MaxIterations <= 0 {
		return nil, errors.New("agent: max iterations must be positive")
	}
	if config.TurnTimeout <= 0 {
		return nil, errors.New("agent: turn timeout must be positive")
	}
	if config.MaxOutputTokensPerCall <= 0 {
		return nil, errors.New("agent: max output tokens per call must be positive")
	}
	if config.ConversationTokenLimit <= 0 {
		return nil, errors.New("agent: conversation token limit must be positive")
	}
	if dependencies.Model == nil {
		return nil, errors.New("agent: model adapter is required")
	}
	if dependencies.Trace == nil {
		dependencies.Trace = noopTraceSink{}
	}
	if dependencies.TokenEstimator == nil {
		dependencies.TokenEstimator = ConservativeTokenEstimator{}
	}
	if dependencies.Budget == nil {
		budget, err := NewMemoryTokenBudget(config.ConversationTokenLimit)
		if err != nil {
			return nil, err
		}
		dependencies.Budget = budget
	}

	allowed := make(map[string]struct{}, len(tools))
	toolVersions := make(map[string]string, len(tools))
	toolCopy := make([]llm.ToolDefinition, len(tools))
	for i, tool := range tools {
		if tool.Name == "" {
			return nil, fmt.Errorf("agent: tool %d has no name", i)
		}
		if len(tool.Name) > 100 {
			return nil, fmt.Errorf("agent: tool %q exceeds the 100-byte name limit", tool.Name)
		}
		if _, exists := allowed[tool.Name]; exists {
			return nil, fmt.Errorf("agent: duplicate tool %q", tool.Name)
		}
		if tool.Version == "" {
			return nil, fmt.Errorf("agent: tool %q has no contract version", tool.Name)
		}
		if len(tool.Version) > 32 {
			return nil, fmt.Errorf("agent: tool %q contract version exceeds 32 bytes", tool.Name)
		}
		if len(tool.Parameters) == 0 || !json.Valid(tool.Parameters) {
			return nil, fmt.Errorf("agent: tool %q has invalid parameter schema", tool.Name)
		}
		allowed[tool.Name] = struct{}{}
		toolVersions[tool.Name] = tool.Version
		toolCopy[i] = tool
		toolCopy[i].Parameters = append(json.RawMessage(nil), tool.Parameters...)
	}

	return &Runner{
		config:          config,
		model:           dependencies.Model,
		executor:        dependencies.ToolExecutor,
		trace:           dependencies.Trace,
		budget:          dependencies.Budget,
		tokenEstimator:  dependencies.TokenEstimator,
		tools:           toolCopy,
		allowedToolName: allowed,
		toolVersions:    toolVersions,
		now:             time.Now,
	}, nil
}

// TurnRequest starts or continues one persisted conversation.
type TurnRequest struct {
	RunID          string
	ConversationID string
	Messages       []llm.Message
}

// TurnResult contains the final assistant message and all messages generated
// during this turn. Usage includes provider-reported tokens only.
type TurnResult struct {
	Message    llm.Message
	Messages   []llm.Message
	Iterations int
	Usage      llm.Usage
}

// Run performs LLM -> all returned tools -> LLM until the assistant emits no
// tool calls. Multiple calls from one model response are executed sequentially
// in response order and every result is appended before the next model call.
func (r *Runner) Run(ctx context.Context, request TurnRequest) (TurnResult, error) {
	if request.ConversationID == "" {
		return TurnResult{}, errors.New("agent: conversation ID is required")
	}
	turnContext, cancel := context.WithTimeout(ctx, r.config.TurnTimeout)
	defer cancel()

	history := cloneMessages(request.Messages)
	result := TurnResult{Messages: history}
	for iteration := 1; iteration <= r.config.MaxIterations; iteration++ {
		if err := turnContext.Err(); err != nil {
			return result, turnContextError(err)
		}

		modelRequest := llm.Request{
			Messages:        cloneMessages(history),
			Tools:           cloneToolDefinitions(r.tools),
			MaxOutputTokens: r.config.MaxOutputTokensPerCall,
		}
		reservedTokens, err := r.tokenEstimator.Estimate(modelRequest)
		if err != nil {
			return result, fmt.Errorf("agent: estimate model tokens: %w", err)
		}
		reservation, err := r.budget.Reserve(turnContext, request.ConversationID, reservedTokens)
		if err != nil {
			return result, err
		}

		modelStarted := r.now()
		response, modelErr := r.model.Complete(turnContext, modelRequest)
		modelFinished := r.now()
		chargedTokens := response.Usage.Total()
		if chargedTokens <= 0 {
			// A failed or usage-less provider request may still have consumed the
			// full allowance. Retaining the reservation keeps the cap hard.
			chargedTokens = reservation.ReservedTokens()
		}
		settleErr := reservation.Settle(context.WithoutCancel(turnContext), chargedTokens)
		if settleErr != nil {
			return result, fmt.Errorf("agent: settle token reservation: %w", settleErr)
		}
		modelStatus := ModelCallSucceeded
		modelErrorMessage := ""
		if modelErr != nil {
			modelStatus = ModelCallFailed
			modelErrorMessage = truncateUTF8(modelErr.Error(), 2000)
		}
		traceContext := turnContext
		if turnContext.Err() != nil {
			traceContext = context.WithoutCancel(turnContext)
		}
		if err := r.trace.RecordModelCall(traceContext, ModelCallTrace{
			RunID:                 request.RunID,
			ConversationID:        request.ConversationID,
			Iteration:             iteration,
			StartedAt:             modelStarted,
			FinishedAt:            modelFinished,
			Model:                 response.Model,
			FinishReason:          response.FinishReason,
			Usage:                 response.Usage,
			ReservedTokens:        reservedTokens,
			ChargedTokens:         chargedTokens,
			ReturnedToolCallCount: len(response.Message.ToolCalls),
			Status:                modelStatus,
			ErrorMessage:          modelErrorMessage,
		}); err != nil {
			return result, fmt.Errorf("agent: persist model trace: %w", err)
		}
		if modelErr != nil {
			if turnContext.Err() != nil {
				return result, turnContextError(turnContext.Err())
			}
			return result, fmt.Errorf("agent: model completion: %w", modelErr)
		}

		result.Iterations = iteration
		addUsage(&result.Usage, response.Usage)

		assistantMessage := cloneMessage(response.Message)
		if assistantMessage.Role == "" {
			assistantMessage.Role = llm.RoleAssistant
		}
		if assistantMessage.Role != llm.RoleAssistant {
			return result, fmt.Errorf("agent: model returned role %q", assistantMessage.Role)
		}
		normalizeToolCallIDs(assistantMessage.ToolCalls, iteration)
		history = append(history, assistantMessage)
		result.Messages = cloneMessages(history)
		if len(assistantMessage.ToolCalls) == 0 {
			result.Message = assistantMessage
			return result, nil
		}

		for callIndex, call := range assistantMessage.ToolCalls {
			if err := turnContext.Err(); err != nil {
				return result, turnContextError(err)
			}
			toolMessage, err := r.executeTool(turnContext, request, iteration, callIndex+1, len(assistantMessage.ToolCalls), call)
			if err != nil {
				return result, err
			}
			history = append(history, toolMessage)
			result.Messages = cloneMessages(history)
		}
	}
	return result, ErrIterationLimit
}

func (r *Runner) executeTool(
	ctx context.Context,
	turn TurnRequest,
	iteration int,
	callIndex int,
	callCount int,
	call llm.ToolCall,
) (llm.Message, error) {
	startedAt := r.now()
	traceArguments := append(json.RawMessage(nil), call.Arguments...)
	if len(traceArguments) > maxToolArgumentBytes || !json.Valid(traceArguments) {
		traceArguments = json.RawMessage(`{"redacted":"invalid or oversized tool arguments"}`)
	}
	traceToolName := truncateUTF8(call.Name, 100)
	if traceToolName == "" {
		traceToolName = "unknown_tool"
	}
	startTrace := ToolExecutionStartedTrace{
		RunID:           turn.RunID,
		ConversationID:  turn.ConversationID,
		Iteration:       iteration,
		CallIndex:       callIndex,
		CallCount:       callCount,
		CallID:          call.ID,
		ToolName:        traceToolName,
		ContractVersion: r.toolVersion(call.Name),
		Arguments:       traceArguments,
		StartedAt:       startedAt,
	}
	if err := r.trace.RecordToolExecutionStarted(ctx, startTrace); err != nil {
		return llm.Message{}, fmt.Errorf("agent: persist tool parent trace: %w", err)
	}

	execution := ToolExecution{}
	var executionErr error
	executorInvoked := false
	if _, allowed := r.allowedToolName[call.Name]; !allowed {
		execution = modelFacingToolError("TOOL_NOT_ALLOWED", "That tool is not available for this agent.")
		execution.Status = ToolStatusRefused
	} else if len(call.Arguments) > maxToolArgumentBytes || !isJSONObject(call.Arguments) {
		execution = modelFacingToolError("INVALID_ARGUMENT", "Tool arguments must be a valid JSON object.")
	} else if r.executor == nil {
		execution = modelFacingToolError("TOOL_UNAVAILABLE", "The tool is temporarily unavailable.")
	} else {
		executorInvoked = true
		execution, executionErr = r.executor.Execute(ctx, ToolRequest{
			RunID:          turn.RunID,
			ConversationID: turn.ConversationID,
			Iteration:      iteration,
			CallIndex:      callIndex,
			CallCount:      callCount,
			Call: llm.ToolCall{
				ID:        call.ID,
				Name:      call.Name,
				Arguments: append(json.RawMessage(nil), call.Arguments...),
			},
		})
		if executionErr != nil {
			execution.Content = modelFacingToolError("TOOL_EXECUTION_FAILED", "The tool could not be executed.").Content
			execution.IsError = true
			execution.Status = ToolStatusFailed
		}
	}
	finishedAt := r.now()
	if len(execution.Content) == 0 || len(execution.Content) > maxToolResultBytes || !json.Valid(execution.Content) {
		attempts := execution.Attempts
		execution = modelFacingToolError("INVALID_TOOL_RESULT", "The tool returned an invalid result.")
		execution.Attempts = attempts
	}

	if executorInvoked && len(execution.Attempts) == 0 {
		status := ToolStatusSucceeded
		if execution.IsError {
			status = ToolStatusFailed
		}
		execution.Attempts = []ToolAttempt{{
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Status:     status,
			Detail:     append(json.RawMessage(nil), execution.Content...),
		}}
	}
	for attemptIndex, attempt := range execution.Attempts {
		if attempt.StartedAt.IsZero() {
			attempt.StartedAt = startedAt
		}
		if attempt.FinishedAt.IsZero() {
			attempt.FinishedAt = finishedAt
		}
		if attempt.Status != ToolStatusSucceeded && attempt.Status != ToolStatusFailed {
			attempt.Status = ToolStatusSucceeded
			if execution.IsError {
				attempt.Status = ToolStatusFailed
			}
		}
		if len(attempt.Detail) == 0 || len(attempt.Detail) > maxToolResultBytes || !json.Valid(attempt.Detail) {
			attempt.Detail = json.RawMessage(`null`)
		}
		if err := r.trace.RecordToolAttempt(ctx, ToolAttemptTrace{
			RunID:           turn.RunID,
			ConversationID:  turn.ConversationID,
			Iteration:       iteration,
			CallIndex:       callIndex,
			CallID:          call.ID,
			ToolName:        traceToolName,
			ContractVersion: r.toolVersion(call.Name),
			AttemptNo:       attemptIndex + 1,
			StartedAt:       attempt.StartedAt,
			FinishedAt:      attempt.FinishedAt,
			Status:          attempt.Status,
			Detail:          append(json.RawMessage(nil), attempt.Detail...),
		}); err != nil {
			return llm.Message{}, fmt.Errorf("agent: persist tool attempt trace: %w", err)
		}
	}

	status := execution.Status
	if status != ToolStatusSucceeded && status != ToolStatusFailed &&
		status != ToolStatusRefused && status != ToolStatusConfirmationRequired {
		status = ToolStatusSucceeded
		if execution.IsError {
			status = ToolStatusFailed
		}
	}
	if err := r.trace.RecordToolExecutionCompleted(ctx, ToolExecutionCompletedTrace{
		RunID:           turn.RunID,
		ConversationID:  turn.ConversationID,
		Iteration:       iteration,
		CallIndex:       callIndex,
		CallID:          call.ID,
		ToolName:        traceToolName,
		ContractVersion: r.toolVersion(call.Name),
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		Status:          status,
		Result:          append(json.RawMessage(nil), execution.Content...),
		AttemptCount:    len(execution.Attempts),
	}); err != nil {
		return llm.Message{}, fmt.Errorf("agent: persist tool completion trace: %w", err)
	}

	return llm.Message{
		Role:       llm.RoleTool,
		Name:       call.Name,
		ToolCallID: call.ID,
		Content:    string(execution.Content),
	}, nil
}

func modelFacingToolError(code string, message string) ToolExecution {
	content, _ := json.Marshal(struct {
		Status string `json:"status"`
		Error  struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}{
		Status: "error",
		Error: struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		}{Code: code, Message: message, Retryable: false},
	})
	return ToolExecution{Content: content, IsError: true, Status: ToolStatusFailed}
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) >= 2 && trimmed[0] == '{' && json.Valid(trimmed)
}

func turnContextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrTurnTimeout, err)
	}
	return err
}

func addUsage(total *llm.Usage, usage llm.Usage) {
	total.InputTokens += usage.InputTokens
	total.OutputTokens += usage.OutputTokens
	total.TotalTokens += usage.Total()
}

func cloneMessages(messages []llm.Message) []llm.Message {
	cloned := make([]llm.Message, len(messages))
	for i := range messages {
		cloned[i] = cloneMessage(messages[i])
	}
	return cloned
}

func cloneMessage(message llm.Message) llm.Message {
	cloned := message
	cloned.ToolCalls = make([]llm.ToolCall, len(message.ToolCalls))
	for i, call := range message.ToolCalls {
		cloned.ToolCalls[i] = call
		cloned.ToolCalls[i].Arguments = append(json.RawMessage(nil), call.Arguments...)
	}
	return cloned
}

func cloneToolDefinitions(tools []llm.ToolDefinition) []llm.ToolDefinition {
	cloned := make([]llm.ToolDefinition, len(tools))
	for i, tool := range tools {
		cloned[i] = tool
		cloned[i].Parameters = append(json.RawMessage(nil), tool.Parameters...)
	}
	return cloned
}

func (r *Runner) toolVersion(name string) string {
	if version := r.toolVersions[name]; version != "" {
		return version
	}
	return "unregistered"
}

func normalizeToolCallIDs(calls []llm.ToolCall, iteration int) {
	seen := make(map[string]struct{}, len(calls))
	for i := range calls {
		if calls[i].ID == "" || len(calls[i].ID) > 200 {
			calls[i].ID = fmt.Sprintf("model-call-%d-%d", iteration, i+1)
		}
		if _, duplicate := seen[calls[i].ID]; duplicate {
			calls[i].ID = fmt.Sprintf("model-call-%d-%d", iteration, i+1)
		}
		seen[calls[i].ID] = struct{}{}
	}
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}
