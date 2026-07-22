// Package agent implements Kontor's bounded LLM-to-tool loop.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reinhlord/kontor/internal/llm"
	"github.com/reinhlord/kontor/internal/tools"
)

const (
	maxToolArgumentBytes = 64 << 10
	maxToolResultBytes   = 256 << 10
)

var (
	ErrIterationLimit   = errors.New("agent: iteration limit reached")
	ErrTurnTimeout      = errors.New("agent: turn deadline exceeded")
	ErrTerminalProtocol = errors.New("agent: terminal response protocol violated")
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
func NewRunner(config Config, dependencies Dependencies, toolDefinitions []llm.ToolDefinition) (*Runner, error) {
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

	allowed := make(map[string]struct{}, len(toolDefinitions))
	toolVersions := make(map[string]string, len(toolDefinitions))
	toolCopy := make([]llm.ToolDefinition, len(toolDefinitions))
	for i, tool := range toolDefinitions {
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
	if _, exists := allowed[tools.ToolRespondToCustomer]; !exists {
		return nil, fmt.Errorf("agent: required terminal tool %q is not registered", tools.ToolRespondToCustomer)
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
	// ToolRefused is sticky for the whole turn so the application can create a
	// durable human escalation even if the model later emits a normal reply.
	ToolRefused    bool
	HumanEscalated bool
	// BookingCommitted remains true even if a later model/trace operation fails,
	// allowing the application to acknowledge the already-durable side effect.
	BookingCommitted bool
	// CustomerResponseDisposition and CustomerResponseToolCallID are populated
	// only after the runner validates a sole respond_to_customer terminal call.
	// The application uses this server-validated signal to update its durable
	// consecutive-clarification policy while persisting Message.
	CustomerResponseDisposition tools.CustomerResponseDisposition
	CustomerResponseToolCallID  string
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
	usedToolCallIDs := make(map[string]struct{})
	for iteration := 1; iteration <= r.config.MaxIterations; iteration++ {
		if err := turnContext.Err(); err != nil {
			return result, turnContextError(err)
		}

		modelRequest := llm.Request{
			Messages:        cloneMessages(history),
			Tools:           cloneToolDefinitions(r.tools),
			ToolChoice:      llm.ToolChoiceRequired,
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
		if modelErr != nil || response.UsageIncomplete || chargedTokens <= 0 {
			// Failed calls and ambiguous provider retries may have consumed tokens
			// without reporting them. Charge the complete worst-case reservation
			// so the persistent conversation cap remains hard.
			chargedTokens = reservation.ReservedTokens()
		}
		settleContext, settleCancel := boundedPersistenceContext(turnContext)
		settleErr := reservation.Settle(settleContext, chargedTokens)
		settleCancel()
		if settleErr != nil {
			return result, fmt.Errorf("agent: settle token reservation: %w", settleErr)
		}
		modelStatus := ModelCallSucceeded
		modelErrorMessage := ""
		if modelErr != nil {
			modelStatus = ModelCallFailed
			modelErrorMessage = truncateUTF8(modelErr.Error(), 2000)
		}
		traceContext, traceCancel := boundedPersistenceContext(turnContext)
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
			traceCancel()
			return result, fmt.Errorf("agent: persist model trace: %w", err)
		}
		traceCancel()
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
		normalizeToolCallIDs(assistantMessage.ToolCalls, iteration, usedToolCallIDs)
		history = append(history, assistantMessage)
		result.Messages = cloneMessages(history)
		if len(assistantMessage.ToolCalls) == 0 {
			return result, fmt.Errorf("%w: model returned unstructured terminal text", ErrTerminalProtocol)
		}

		terminalCallIndex := -1
		for callIndex, call := range assistantMessage.ToolCalls {
			if call.Name == tools.ToolRespondToCustomer {
				terminalCallIndex = callIndex
				break
			}
		}
		if terminalCallIndex >= 0 {
			// Preflight the complete batch before executing any sibling. A terminal
			// response can never share a model response with a domain side effect.
			if len(assistantMessage.ToolCalls) != 1 || terminalCallIndex != 0 {
				return result, fmt.Errorf("%w: %s must be the only tool call in its response", ErrTerminalProtocol, tools.ToolRespondToCustomer)
			}
			if strings.TrimSpace(assistantMessage.Content) != "" {
				return result, fmt.Errorf("%w: %s cannot include separate assistant content", ErrTerminalProtocol, tools.ToolRespondToCustomer)
			}
			call := assistantMessage.ToolCalls[0]
			toolMessage, arguments, terminalErr := r.executeCustomerResponse(turnContext, request, iteration, call)
			if toolMessage.Role != "" {
				history = append(history, toolMessage)
				result.Messages = cloneMessages(history)
			}
			if terminalErr != nil {
				return result, terminalErr
			}
			result.Message = llm.Message{Role: llm.RoleAssistant, Content: arguments.Message}
			result.CustomerResponseDisposition = arguments.Disposition
			result.CustomerResponseToolCallID = call.ID
			history = append(history, result.Message)
			result.Messages = cloneMessages(history)
			return result, nil
		}
		// A tool result must always be followed by another model response. Do not
		// start a side effect when the iteration cap leaves no room for that
		// response; the successful model trace already records the requested batch.
		if iteration == r.config.MaxIterations {
			return result, ErrIterationLimit
		}

		terminalHandoff := false
		for callIndex, call := range assistantMessage.ToolCalls {
			if err := turnContext.Err(); err != nil {
				return result, turnContextError(err)
			}
			if terminalHandoff {
				toolMessage, err := r.recordSkippedTool(turnContext, request, iteration, callIndex+1, len(assistantMessage.ToolCalls), call)
				if err != nil {
					return result, err
				}
				history = append(history, toolMessage)
				result.Messages = cloneMessages(history)
				continue
			}
			toolMessage, toolStatus, sideEffectCommitted, err := r.executeTool(turnContext, request, iteration, callIndex+1, len(assistantMessage.ToolCalls), call)
			if sideEffectCommitted && call.Name == "create_booking" {
				result.BookingCommitted = true
			}
			if err != nil {
				return result, err
			}
			if toolStatus == ToolStatusRefused {
				result.ToolRefused = true
				terminalHandoff = true
			}
			if toolStatus == ToolStatusSucceeded && call.Name == "escalate_to_human" {
				result.HumanEscalated = true
				terminalHandoff = true
			}
			history = append(history, toolMessage)
			result.Messages = cloneMessages(history)
		}
		if terminalHandoff {
			content := "I’ve handed this conversation to a person. The automated agent will not take further actions."
			if result.ToolRefused && !result.HumanEscalated {
				content = "I couldn’t perform that action safely, so I’ve handed this conversation to a person."
			}
			result.Message = llm.Message{Role: llm.RoleAssistant, Content: content}
			history = append(history, result.Message)
			result.Messages = cloneMessages(history)
			return result, nil
		}
	}
	return result, ErrIterationLimit
}

// executeCustomerResponse validates and traces the runner-local terminal
// control call. It deliberately records no attempts and never invokes the
// injected ToolExecutor or the tools Gateway.
func (r *Runner) executeCustomerResponse(
	ctx context.Context,
	turn TurnRequest,
	iteration int,
	call llm.ToolCall,
) (llm.Message, tools.RespondToCustomerArguments, error) {
	startedAt := r.now()
	traceArguments := append(json.RawMessage(nil), call.Arguments...)
	if len(traceArguments) > maxToolArgumentBytes || !json.Valid(traceArguments) {
		traceArguments = json.RawMessage(`{"redacted":"invalid or oversized terminal arguments"}`)
	}
	startTrace := ToolExecutionStartedTrace{
		RunID:           turn.RunID,
		ConversationID:  turn.ConversationID,
		Iteration:       iteration,
		CallIndex:       1,
		CallCount:       1,
		CallID:          call.ID,
		ToolName:        tools.ToolRespondToCustomer,
		ContractVersion: r.toolVersion(tools.ToolRespondToCustomer),
		Arguments:       traceArguments,
		StartedAt:       startedAt,
	}
	if err := r.trace.RecordToolExecutionStarted(ctx, startTrace); err != nil {
		return llm.Message{}, tools.RespondToCustomerArguments{}, fmt.Errorf("agent: persist terminal response parent trace: %w", err)
	}

	arguments, parseErr := tools.ParseRespondToCustomerArguments(call.Arguments)
	if len(call.Arguments) > maxToolArgumentBytes {
		parseErr = errors.New("terminal response arguments exceed the size limit")
	}
	status := ToolStatusSucceeded
	resultPayload := map[string]any{
		"status": "success",
		"data":   arguments,
	}
	if parseErr != nil {
		status = ToolStatusFailed
		resultPayload = map[string]any{
			"status": "error",
			"error": map[string]any{
				"code":    "INVALID_ARGUMENT",
				"message": "The terminal response did not match the required contract.",
			},
		}
	}
	encodedResult, marshalErr := json.Marshal(resultPayload)
	if marshalErr != nil {
		return llm.Message{}, tools.RespondToCustomerArguments{}, fmt.Errorf("agent: encode terminal response result: %w", marshalErr)
	}
	toolMessage := llm.Message{
		Role:       llm.RoleTool,
		Name:       tools.ToolRespondToCustomer,
		ToolCallID: call.ID,
		Content:    string(encodedResult),
	}
	finishedAt := r.now()
	traceContext, traceCancel := boundedPersistenceContext(ctx)
	defer traceCancel()
	if err := r.trace.RecordToolExecutionCompleted(traceContext, ToolExecutionCompletedTrace{
		RunID:           turn.RunID,
		ConversationID:  turn.ConversationID,
		Iteration:       iteration,
		CallIndex:       1,
		CallID:          call.ID,
		ToolName:        tools.ToolRespondToCustomer,
		ContractVersion: r.toolVersion(tools.ToolRespondToCustomer),
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		Status:          status,
		Result:          encodedResult,
		AttemptCount:    0,
	}); err != nil {
		return toolMessage, tools.RespondToCustomerArguments{}, fmt.Errorf("agent: persist terminal response completion trace: %w", err)
	}
	if parseErr != nil {
		return toolMessage, tools.RespondToCustomerArguments{}, fmt.Errorf("%w: invalid %s arguments", ErrTerminalProtocol, tools.ToolRespondToCustomer)
	}
	if err := ctx.Err(); err != nil {
		return toolMessage, tools.RespondToCustomerArguments{}, turnContextError(err)
	}
	return toolMessage, arguments, nil
}

func (r *Runner) executeTool(
	ctx context.Context,
	turn TurnRequest,
	iteration int,
	callIndex int,
	callCount int,
	call llm.ToolCall,
) (llm.Message, ToolStatus, bool, error) {
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
		return llm.Message{}, "", false, fmt.Errorf("agent: persist tool parent trace: %w", err)
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
		sideEffectCommitted := execution.SideEffectCommitted
		execution = modelFacingToolError("INVALID_TOOL_RESULT", "The tool returned an invalid result.")
		execution.Attempts = attempts
		execution.SideEffectCommitted = sideEffectCommitted
	}
	status := execution.Status
	if status != ToolStatusSucceeded && status != ToolStatusFailed &&
		status != ToolStatusRefused && status != ToolStatusConfirmationRequired {
		status = ToolStatusSucceeded
		if execution.IsError {
			status = ToolStatusFailed
		}
	}
	toolMessage := llm.Message{
		Role:       llm.RoleTool,
		Name:       call.Name,
		ToolCallID: call.ID,
		Content:    string(execution.Content),
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
	traceContext, traceCancel := boundedPersistenceContext(ctx)
	defer traceCancel()
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
		if err := r.trace.RecordToolAttempt(traceContext, ToolAttemptTrace{
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
			return toolMessage, status, execution.SideEffectCommitted, fmt.Errorf("agent: persist tool attempt trace: %w", err)
		}
	}
	if err := r.trace.RecordToolExecutionCompleted(traceContext, ToolExecutionCompletedTrace{
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
		return toolMessage, status, execution.SideEffectCommitted, fmt.Errorf("agent: persist tool completion trace: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return toolMessage, status, execution.SideEffectCommitted, turnContextError(err)
	}

	return toolMessage, status, execution.SideEffectCommitted, nil
}

func (r *Runner) recordSkippedTool(
	ctx context.Context,
	turn TurnRequest,
	iteration, callIndex, callCount int,
	call llm.ToolCall,
) (llm.Message, error) {
	startedAt := r.now()
	arguments := append(json.RawMessage(nil), call.Arguments...)
	if len(arguments) > maxToolArgumentBytes || !json.Valid(arguments) {
		arguments = json.RawMessage(`{"redacted":"invalid or oversized tool arguments"}`)
	}
	toolName := truncateUTF8(call.Name, 100)
	if toolName == "" {
		toolName = "unknown_tool"
	}
	if err := r.trace.RecordToolExecutionStarted(ctx, ToolExecutionStartedTrace{
		RunID: turn.RunID, ConversationID: turn.ConversationID, Iteration: iteration,
		CallIndex: callIndex, CallCount: callCount, CallID: call.ID, ToolName: toolName,
		ContractVersion: r.toolVersion(call.Name), Arguments: arguments, StartedAt: startedAt,
	}); err != nil {
		return llm.Message{}, fmt.Errorf("agent: persist skipped tool parent trace: %w", err)
	}
	execution := modelFacingToolError("SKIPPED_AFTER_HANDOFF", "The call was not executed because this response already triggered a human hand-off.")
	execution.Status = ToolStatusRefused
	finishedAt := r.now()
	if err := r.trace.RecordToolExecutionCompleted(ctx, ToolExecutionCompletedTrace{
		RunID: turn.RunID, ConversationID: turn.ConversationID, Iteration: iteration,
		CallIndex: callIndex, CallID: call.ID, ToolName: toolName,
		ContractVersion: r.toolVersion(call.Name), StartedAt: startedAt, FinishedAt: finishedAt,
		Status: ToolStatusRefused, Result: execution.Content,
	}); err != nil {
		return llm.Message{}, fmt.Errorf("agent: persist skipped tool completion trace: %w", err)
	}
	return llm.Message{
		Role: llm.RoleTool, Name: call.Name, ToolCallID: call.ID, Content: string(execution.Content),
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

func normalizeToolCallIDs(calls []llm.ToolCall, iteration int, seen map[string]struct{}) {
	if seen == nil {
		seen = make(map[string]struct{}, len(calls))
	}
	for i := range calls {
		candidate := calls[i].ID
		_, duplicate := seen[candidate]
		if candidate == "" || len(candidate) > 200 || duplicate {
			base := fmt.Sprintf("model-call-%d-%d", iteration, i+1)
			candidate = base
			for suffix := 2; ; suffix++ {
				if _, exists := seen[candidate]; !exists {
					break
				}
				candidate = fmt.Sprintf("%s-%d", base, suffix)
			}
		}
		calls[i].ID = candidate
		seen[candidate] = struct{}{}
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

func boundedPersistenceContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent.Err() == nil {
		return parent, func() {}
	}
	return context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
}
