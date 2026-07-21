package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOpenRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"
	defaultOpenRouterTimeout  = 20 * time.Second
	maxOpenRouterResponseSize = 4 << 20
)

// OpenRouterConfig is supplied by the application configuration layer. The
// adapter deliberately does not read environment variables itself.
type OpenRouterConfig struct {
	APIKey      string
	Model       string
	Endpoint    string
	HTTPReferer string
	AppTitle    string
	Timeout     time.Duration
	HTTPClient  *http.Client
}

// OpenRouterAdapter implements the normalized OpenRouter Chat Completions API.
type OpenRouterAdapter struct {
	apiKey      string
	model       string
	endpoint    string
	httpReferer string
	appTitle    string
	timeout     time.Duration
	client      *http.Client
}

// NewOpenRouterAdapter validates config and constructs an adapter. API keys
// and model selection are explicit inputs so tests and applications remain
// independent of process-global environment state.
func NewOpenRouterAdapter(config OpenRouterConfig) (*OpenRouterAdapter, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, errors.New("openrouter: API key is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("openrouter: model is required")
	}
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = defaultOpenRouterEndpoint
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultOpenRouterTimeout
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	return &OpenRouterAdapter{
		apiKey:      config.APIKey,
		model:       config.Model,
		endpoint:    endpoint,
		httpReferer: config.HTTPReferer,
		appTitle:    config.AppTitle,
		timeout:     timeout,
		client:      client,
	}, nil
}

type openRouterRequest struct {
	Model               string              `json:"model"`
	Messages            []openRouterMessage `json:"messages"`
	Tools               []openRouterTool    `json:"tools,omitempty"`
	ToolChoice          string              `json:"tool_choice,omitempty"`
	ParallelToolCalls   bool                `json:"parallel_tool_calls"`
	MaxCompletionTokens int                 `json:"max_completion_tokens,omitempty"`
}

type openRouterMessage struct {
	Role       Role                 `json:"role"`
	Content    *string              `json:"content,omitempty"`
	Name       string               `json:"name,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	ToolCalls  []openRouterToolCall `json:"tool_calls,omitempty"`
}

type openRouterTool struct {
	Type     string                 `json:"type"`
	Function openRouterFunctionTool `json:"function"`
}

type openRouterFunctionTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openRouterToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openRouterFunctionCall `json:"function"`
}

type openRouterFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openRouterResponse struct {
	ID      string              `json:"id"`
	Model   string              `json:"model"`
	Choices []openRouterChoice  `json:"choices"`
	Usage   openRouterUsage     `json:"usage"`
	Error   *openRouterAPIError `json:"error,omitempty"`
}

type openRouterChoice struct {
	FinishReason string               `json:"finish_reason"`
	Message      openRouterRawMessage `json:"message"`
	Error        *openRouterAPIError  `json:"error,omitempty"`
}

type openRouterRawMessage struct {
	Role      Role                 `json:"role"`
	Content   json.RawMessage      `json:"content"`
	ToolCalls []openRouterToolCall `json:"tool_calls"`
}

type openRouterUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openRouterAPIError struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
}

// OpenRouterError is a sanitized provider/API failure. It never includes the
// request body or authorization header.
type OpenRouterError struct {
	StatusCode int
	Code       string
	Message    string
	RetryAfter time.Duration
}

func (e *OpenRouterError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("openrouter: HTTP %d: %s", e.StatusCode, e.Message)
	}
	return "openrouter: " + e.Message
}

// Retryable reports whether the HTTP status conventionally permits a retry.
func (e *OpenRouterError) Retryable() bool {
	return e.StatusCode == http.StatusRequestTimeout ||
		e.StatusCode == http.StatusTooManyRequests ||
		e.StatusCode == http.StatusBadGateway ||
		e.StatusCode == http.StatusServiceUnavailable ||
		e.StatusCode == http.StatusGatewayTimeout
}

// Complete sends one non-streaming Chat Completions request. OpenRouter is
// always asked to permit parallel tool calls; the agent runner intentionally
// executes a returned batch in response order.
func (a *OpenRouterAdapter) Complete(ctx context.Context, request Request) (Response, error) {
	wireRequest, err := a.toOpenRouterRequest(request)
	if err != nil {
		return Response{}, err
	}
	body, err := json.Marshal(wireRequest)
	if err != nil {
		return Response{}, fmt.Errorf("openrouter: encode request: %w", err)
	}

	requestContext, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("openrouter: create request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	if a.httpReferer != "" {
		httpRequest.Header.Set("HTTP-Referer", a.httpReferer)
	}
	if a.appTitle != "" {
		httpRequest.Header.Set("X-OpenRouter-Title", a.appTitle)
	}

	httpResponse, err := a.client.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("openrouter: send request: %w", err)
	}
	defer httpResponse.Body.Close()

	limitedBody := io.LimitReader(httpResponse.Body, maxOpenRouterResponseSize+1)
	responseBody, err := io.ReadAll(limitedBody)
	if err != nil {
		return Response{}, fmt.Errorf("openrouter: read response: %w", err)
	}
	if len(responseBody) > maxOpenRouterResponseSize {
		return Response{}, errors.New("openrouter: response exceeds size limit")
	}

	var wireResponse openRouterResponse
	decodeErr := json.Unmarshal(responseBody, &wireResponse)
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		if decodeErr != nil {
			return Response{}, newOpenRouterError(httpResponse, nil)
		}
		return partialOpenRouterResponse(wireResponse), newOpenRouterError(httpResponse, wireResponse.Error)
	}
	if decodeErr != nil {
		return Response{}, fmt.Errorf("openrouter: decode response: %w", decodeErr)
	}
	if wireResponse.Error != nil {
		return partialOpenRouterResponse(wireResponse), newOpenRouterError(httpResponse, wireResponse.Error)
	}
	if len(wireResponse.Choices) == 0 {
		return Response{}, errors.New("openrouter: response contains no choices")
	}

	choice := wireResponse.Choices[0]
	if choice.Error != nil {
		return partialOpenRouterResponse(wireResponse), newOpenRouterError(httpResponse, choice.Error)
	}
	if choice.FinishReason == "error" {
		return partialOpenRouterResponse(wireResponse), &OpenRouterError{StatusCode: httpResponse.StatusCode, Message: "provider returned an error finish reason"}
	}
	message, err := fromOpenRouterMessage(choice.Message)
	if err != nil {
		return Response{}, err
	}
	return Response{
		ID:           wireResponse.ID,
		Model:        wireResponse.Model,
		Message:      message,
		FinishReason: choice.FinishReason,
		Usage: Usage{
			InputTokens:  wireResponse.Usage.PromptTokens,
			OutputTokens: wireResponse.Usage.CompletionTokens,
			TotalTokens:  wireResponse.Usage.TotalTokens,
		},
	}, nil
}

func partialOpenRouterResponse(wire openRouterResponse) Response {
	return Response{
		ID:    wire.ID,
		Model: wire.Model,
		Usage: Usage{
			InputTokens:  wire.Usage.PromptTokens,
			OutputTokens: wire.Usage.CompletionTokens,
			TotalTokens:  wire.Usage.TotalTokens,
		},
	}
}

func (a *OpenRouterAdapter) toOpenRouterRequest(request Request) (openRouterRequest, error) {
	wire := openRouterRequest{
		Model:               a.model,
		Messages:            make([]openRouterMessage, len(request.Messages)),
		ParallelToolCalls:   true,
		MaxCompletionTokens: request.MaxOutputTokens,
	}
	for i, message := range request.Messages {
		converted, err := toOpenRouterMessage(message)
		if err != nil {
			return openRouterRequest{}, fmt.Errorf("openrouter: message %d: %w", i, err)
		}
		wire.Messages[i] = converted
	}
	if len(request.Tools) > 0 {
		wire.ToolChoice = "auto"
		wire.Tools = make([]openRouterTool, len(request.Tools))
		for i, tool := range request.Tools {
			if strings.TrimSpace(tool.Name) == "" {
				return openRouterRequest{}, fmt.Errorf("openrouter: tool %d has no name", i)
			}
			parameters := tool.Parameters
			if len(parameters) == 0 {
				parameters = json.RawMessage(`{"type":"object","additionalProperties":false}`)
			}
			if !json.Valid(parameters) {
				return openRouterRequest{}, fmt.Errorf("openrouter: tool %q has invalid parameter schema", tool.Name)
			}
			wire.Tools[i] = openRouterTool{
				Type: "function",
				Function: openRouterFunctionTool{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  append(json.RawMessage(nil), parameters...),
				},
			}
		}
	}
	return wire, nil
}

func toOpenRouterMessage(message Message) (openRouterMessage, error) {
	wire := openRouterMessage{
		Role:       message.Role,
		ToolCallID: message.ToolCallID,
	}
	switch message.Role {
	case RoleSystem, RoleUser:
		wire.Name = message.Name
		content := message.Content
		wire.Content = &content
		if message.ToolCallID != "" || len(message.ToolCalls) > 0 {
			return openRouterMessage{}, errors.New("system/user message cannot contain tool call fields")
		}
	case RoleAssistant:
		wire.Name = message.Name
		if message.ToolCallID != "" {
			return openRouterMessage{}, errors.New("assistant message cannot be a tool response")
		}
		if message.Content != "" {
			content := message.Content
			wire.Content = &content
		}
		wire.ToolCalls = make([]openRouterToolCall, len(message.ToolCalls))
		for i, call := range message.ToolCalls {
			if call.ID == "" || call.Name == "" {
				return openRouterMessage{}, fmt.Errorf("tool call %d requires id and name", i)
			}
			wire.ToolCalls[i] = openRouterToolCall{
				ID:   call.ID,
				Type: "function",
				Function: openRouterFunctionCall{
					Name:      call.Name,
					Arguments: string(call.Arguments),
				},
			}
		}
	case RoleTool:
		if message.ToolCallID == "" {
			return openRouterMessage{}, errors.New("tool message requires tool_call_id")
		}
		if len(message.ToolCalls) > 0 {
			return openRouterMessage{}, errors.New("tool message cannot request tools")
		}
		content := message.Content
		wire.Content = &content
	default:
		return openRouterMessage{}, fmt.Errorf("unsupported role %q", message.Role)
	}
	return wire, nil
}

func fromOpenRouterMessage(wire openRouterRawMessage) (Message, error) {
	if wire.Role != RoleAssistant {
		return Message{}, fmt.Errorf("openrouter: completion has unexpected role %q", wire.Role)
	}
	content, err := decodeNullableString(wire.Content)
	if err != nil {
		return Message{}, fmt.Errorf("openrouter: decode assistant content: %w", err)
	}
	message := Message{Role: RoleAssistant, Content: content}
	message.ToolCalls = make([]ToolCall, len(wire.ToolCalls))
	for i, call := range wire.ToolCalls {
		if call.Type != "" && call.Type != "function" {
			return Message{}, fmt.Errorf("openrouter: tool call %d has unsupported type %q", i, call.Type)
		}
		if call.ID == "" || call.Function.Name == "" {
			return Message{}, fmt.Errorf("openrouter: tool call %d requires id and name", i)
		}
		message.ToolCalls[i] = ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		}
	}
	return message, nil
}

func decodeNullableString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", errors.New("only text content is supported")
	}
	return value, nil
}

func newOpenRouterError(response *http.Response, providerError *openRouterAPIError) *OpenRouterError {
	result := &OpenRouterError{StatusCode: response.StatusCode}
	if providerError != nil {
		result.Code = strings.Trim(string(providerError.Code), `"`)
		result.Message = strings.TrimSpace(providerError.Message)
	}
	if result.Message == "" {
		result.Message = http.StatusText(response.StatusCode)
	}
	if retryAfter := strings.TrimSpace(response.Header.Get("Retry-After")); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
			result.RetryAfter = time.Duration(seconds) * time.Second
		}
	}
	return result
}
