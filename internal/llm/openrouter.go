package llm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultOpenRouterEndpoint = "https://openrouter.ai/api/v1/chat/completions"
	defaultOpenAIEndpoint     = "https://api.openai.com/v1/chat/completions"
	defaultOpenRouterTimeout  = 20 * time.Second
	defaultOpenRouterAttempts = 3
	defaultRetryBaseDelay     = 200 * time.Millisecond
	defaultRetryMaxDelay      = 4 * time.Second
	maxOpenRouterAttempts     = 10
	maxOpenRouterResponseSize = 4 << 20
)

// RetryJitterFunc returns jitter in [0,max]. It is injectable so retry timing
// can be tested without randomness.
type RetryJitterFunc func(max time.Duration) time.Duration

// RetryWaitFunc waits for a retry or returns when ctx is done.
type RetryWaitFunc func(ctx context.Context, delay time.Duration) error

// OpenRouterConfig is supplied by the application configuration layer. The
// adapter deliberately does not read environment variables itself.
type OpenRouterConfig struct {
	APIKey         string
	Model          string
	Endpoint       string
	HTTPReferer    string
	AppTitle       string
	Timeout        time.Duration
	HTTPClient     *http.Client
	MaxAttempts    int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	RetryJitter    RetryJitterFunc
	RetryWait      RetryWaitFunc
	Now            func() time.Time
}

// OpenAIConfig configures a direct OpenAI Chat Completions connection. It
// intentionally does not expose OpenRouter attribution headers.
type OpenAIConfig struct {
	APIKey         string
	Model          string
	Endpoint       string
	Timeout        time.Duration
	HTTPClient     *http.Client
	MaxAttempts    int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	RetryJitter    RetryJitterFunc
	RetryWait      RetryWaitFunc
	Now            func() time.Time
}

// OpenRouterAdapter implements the normalized OpenRouter Chat Completions API.
type OpenRouterAdapter struct {
	apiKey         string
	model          string
	endpoint       string
	httpReferer    string
	appTitle       string
	timeout        time.Duration
	client         *http.Client
	maxAttempts    int
	retryBaseDelay time.Duration
	retryMaxDelay  time.Duration
	retryJitter    RetryJitterFunc
	retryWait      RetryWaitFunc
	now            func() time.Time
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
	maxAttempts := config.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = defaultOpenRouterAttempts
	}
	if maxAttempts < 1 || maxAttempts > maxOpenRouterAttempts {
		return nil, fmt.Errorf("openrouter: max attempts must be between 1 and %d", maxOpenRouterAttempts)
	}
	retryBaseDelay := config.RetryBaseDelay
	if retryBaseDelay == 0 {
		retryBaseDelay = defaultRetryBaseDelay
	}
	if retryBaseDelay < 0 {
		return nil, errors.New("openrouter: retry base delay cannot be negative")
	}
	retryMaxDelay := config.RetryMaxDelay
	if retryMaxDelay == 0 {
		retryMaxDelay = defaultRetryMaxDelay
	}
	if retryMaxDelay < retryBaseDelay {
		return nil, errors.New("openrouter: retry max delay must be at least the base delay")
	}
	retryJitter := config.RetryJitter
	if retryJitter == nil {
		retryJitter = cryptoRetryJitter
	}
	retryWait := config.RetryWait
	if retryWait == nil {
		retryWait = waitForRetry
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}

	return &OpenRouterAdapter{
		apiKey:         strings.TrimSpace(config.APIKey),
		model:          strings.TrimSpace(config.Model),
		endpoint:       endpoint,
		httpReferer:    config.HTTPReferer,
		appTitle:       config.AppTitle,
		timeout:        timeout,
		client:         client,
		maxAttempts:    maxAttempts,
		retryBaseDelay: retryBaseDelay,
		retryMaxDelay:  retryMaxDelay,
		retryJitter:    retryJitter,
		retryWait:      retryWait,
		now:            now,
	}, nil
}

// NewOpenAIAdapter constructs a direct OpenAI adapter. OpenAI and OpenRouter
// use the same Chat Completions wire contract used by this adapter, while only
// OpenRouter receives its attribution headers.
func NewOpenAIAdapter(config OpenAIConfig) (*OpenRouterAdapter, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = defaultOpenAIEndpoint
	}
	return NewOpenRouterAdapter(OpenRouterConfig{
		APIKey:         config.APIKey,
		Model:          config.Model,
		Endpoint:       endpoint,
		Timeout:        config.Timeout,
		HTTPClient:     config.HTTPClient,
		MaxAttempts:    config.MaxAttempts,
		RetryBaseDelay: config.RetryBaseDelay,
		RetryMaxDelay:  config.RetryMaxDelay,
		RetryJitter:    config.RetryJitter,
		RetryWait:      config.RetryWait,
		Now:            config.Now,
	})
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
		e.StatusCode == http.StatusInternalServerError ||
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

	var totalUsage Usage
	usageIncomplete := false
	for attempt := 1; attempt <= a.maxAttempts; attempt++ {
		response, attemptErr := a.completeAttempt(requestContext, body)
		if attemptErr != nil && response.Usage.Total() <= 0 {
			usageIncomplete = true
		}
		addProviderUsage(&totalUsage, response.Usage)
		response.Usage = totalUsage
		response.UsageIncomplete = usageIncomplete
		if attemptErr == nil {
			return response, nil
		}
		if attempt == a.maxAttempts || !shouldRetryOpenRouter(requestContext, attemptErr) {
			return response, attemptErr
		}

		delay := a.retryDelay(attempt, attemptErr)
		if err := a.retryWait(requestContext, delay); err != nil {
			return response, fmt.Errorf("openrouter: retry wait: %w", err)
		}
	}
	return Response{Usage: totalUsage, UsageIncomplete: usageIncomplete}, errors.New("openrouter: retry loop ended unexpectedly")
}

func (a *OpenRouterAdapter) completeAttempt(ctx context.Context, body []byte) (Response, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
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
			return Response{}, newOpenRouterError(httpResponse, nil, a.now())
		}
		return partialOpenRouterResponse(wireResponse), newOpenRouterError(httpResponse, wireResponse.Error, a.now())
	}
	if decodeErr != nil {
		return Response{}, fmt.Errorf("openrouter: decode response: %w", decodeErr)
	}
	if wireResponse.Error != nil {
		return partialOpenRouterResponse(wireResponse), newOpenRouterError(httpResponse, wireResponse.Error, a.now())
	}
	if len(wireResponse.Choices) == 0 {
		return Response{}, errors.New("openrouter: response contains no choices")
	}

	choice := wireResponse.Choices[0]
	if choice.Error != nil {
		return partialOpenRouterResponse(wireResponse), newOpenRouterError(httpResponse, choice.Error, a.now())
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

func (a *OpenRouterAdapter) retryDelay(failedAttempt int, err error) time.Duration {
	backoff := a.retryBaseDelay
	for step := 1; step < failedAttempt && backoff < a.retryMaxDelay; step++ {
		if backoff > a.retryMaxDelay/2 {
			backoff = a.retryMaxDelay
			break
		}
		backoff *= 2
	}
	if backoff > a.retryMaxDelay {
		backoff = a.retryMaxDelay
	}

	half := backoff / 2
	jitterWindow := backoff - half
	jitter := a.retryJitter(jitterWindow)
	if jitter < 0 {
		jitter = 0
	}
	if jitter > jitterWindow {
		jitter = jitterWindow
	}
	delay := half + jitter

	var providerError *OpenRouterError
	if errors.As(err, &providerError) && providerError.RetryAfter > delay {
		delay = providerError.RetryAfter
	}
	return delay
}

func shouldRetryOpenRouter(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	var providerError *OpenRouterError
	if errors.As(err, &providerError) {
		switch providerError.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	return isTransientTransportError(err)
}

func isTransientTransportError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary())
}

func addProviderUsage(total *Usage, usage Usage) {
	total.InputTokens += usage.InputTokens
	total.OutputTokens += usage.OutputTokens
	total.TotalTokens += usage.Total()
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func cryptoRetryJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return max / 2
	}
	return time.Duration(binary.LittleEndian.Uint64(random[:]) % uint64(max+1))
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
	switch request.ToolChoice {
	case "", ToolChoiceAuto:
	case ToolChoiceRequired:
		if len(request.Tools) == 0 {
			return openRouterRequest{}, errors.New("openrouter: required tool choice needs at least one tool")
		}
	default:
		return openRouterRequest{}, fmt.Errorf("openrouter: unsupported tool choice %q", request.ToolChoice)
	}
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
		wire.ToolChoice = string(request.ToolChoice)
		if wire.ToolChoice == "" {
			wire.ToolChoice = string(ToolChoiceAuto)
		}
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

func newOpenRouterError(response *http.Response, providerError *openRouterAPIError, now time.Time) *OpenRouterError {
	result := &OpenRouterError{StatusCode: response.StatusCode}
	if providerError != nil {
		result.Code = strings.Trim(string(providerError.Code), `"`)
		result.Message = strings.TrimSpace(providerError.Message)
		// OpenRouter can report a non-streaming upstream-provider failure inside
		// an otherwise successful HTTP 200 response. In that shape the embedded
		// numeric error code, not the transport status, controls retry policy.
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			if embeddedStatus, ok := embeddedOpenRouterStatus(providerError.Code); ok {
				result.StatusCode = embeddedStatus
			}
		}
	}
	if result.Message == "" {
		result.Message = http.StatusText(response.StatusCode)
	}
	if retryAfter := strings.TrimSpace(response.Header.Get("Retry-After")); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
			result.RetryAfter = time.Duration(seconds) * time.Second
		} else if retryAt, err := http.ParseTime(retryAfter); err == nil && retryAt.After(now) {
			result.RetryAfter = retryAt.Sub(now)
		}
	}
	return result
}

func embeddedOpenRouterStatus(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var numeric int
	if err := json.Unmarshal(raw, &numeric); err == nil && numeric >= 400 && numeric <= 599 {
		return numeric, true
	}
	var textCode string
	if err := json.Unmarshal(raw, &textCode); err != nil {
		return 0, false
	}
	numeric, err := strconv.Atoi(strings.TrimSpace(textCode))
	return numeric, err == nil && numeric >= 400 && numeric <= 599
}
