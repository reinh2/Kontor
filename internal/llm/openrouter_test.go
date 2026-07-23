package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOpenRouterAdapterChatCompletionsToolContract(t *testing.T) {
	t.Parallel()

	requestSeen := make(chan openRouterRequest, 1)
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/api/v1/chat/completions" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.Header.Get("HTTP-Referer"); got != "https://kontor.example" {
			t.Errorf("HTTP-Referer = %q", got)
		}
		if got := request.Header.Get("X-OpenRouter-Title"); got != "Kontor" {
			t.Errorf("X-OpenRouter-Title = %q", got)
		}

		var body openRouterRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		requestSeen <- body
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{
			"id":"gen-123",
			"model":"openai/gpt-4.1-mini",
			"choices":[{"finish_reason":"tool_calls","message":{
				"role":"assistant","content":null,
				"tool_calls":[
					{"id":"call-a","type":"function","function":{"name":"list_services","arguments":"{}"}},
					{"id":"call-b","type":"function","function":{"name":"list_staff","arguments":"{\"service_id\":\"service-1\"}"}}
				]
			}}],
			"usage":{"prompt_tokens":41,"completion_tokens":17,"total_tokens":58}
		}`))
	})

	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "secret-key", Model: "openai/gpt-4.1-mini",
		Endpoint: "https://openrouter.test/api/v1/chat/completions", HTTPClient: recorderClient(handler),
		HTTPReferer: "https://kontor.example", AppTitle: "Kontor", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Complete(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "Find appointments"},
			{Role: RoleAssistant, ToolCalls: []ToolCall{
				{ID: "previous-a", Name: "first", Arguments: json.RawMessage(`{"one":1}`)},
				{ID: "previous-b", Name: "second", Arguments: json.RawMessage(`{"two":2}`)},
			}},
			{Role: RoleTool, Name: "first", ToolCallID: "previous-a", Content: `{"ok":1}`},
			{Role: RoleTool, Name: "second", ToolCallID: "previous-b", Content: `{"ok":2}`},
		},
		Tools: []ToolDefinition{{
			Name: "list_services", Description: "List services",
			Parameters: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		}},
		MaxOutputTokens: 321,
	})
	if err != nil {
		t.Fatal(err)
	}

	wireRequest := <-requestSeen
	if wireRequest.Model != "openai/gpt-4.1-mini" || wireRequest.MaxCompletionTokens != 321 {
		t.Fatalf("wire request model/tokens = %q/%d", wireRequest.Model, wireRequest.MaxCompletionTokens)
	}
	if !wireRequest.ParallelToolCalls {
		t.Fatal("parallel_tool_calls was not true")
	}
	if wireRequest.ToolChoice != "auto" || len(wireRequest.Tools) != 1 || wireRequest.Tools[0].Type != "function" {
		t.Fatalf("wire tools = %#v choice=%q", wireRequest.Tools, wireRequest.ToolChoice)
	}
	if len(wireRequest.Messages) != 4 || len(wireRequest.Messages[1].ToolCalls) != 2 {
		t.Fatalf("wire messages = %#v", wireRequest.Messages)
	}
	if wireRequest.Messages[1].ToolCalls[0].Function.Arguments != `{"one":1}` ||
		wireRequest.Messages[2].ToolCallID != "previous-a" || wireRequest.Messages[3].ToolCallID != "previous-b" {
		t.Fatalf("wire tool history order = %#v", wireRequest.Messages)
	}

	if response.ID != "gen-123" || response.Usage.Total() != 58 || response.FinishReason != "tool_calls" {
		t.Fatalf("response = %#v", response)
	}
	wantNames := []string{"list_services", "list_staff"}
	gotNames := []string{response.Message.ToolCalls[0].Name, response.Message.ToolCalls[1].Name}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("tool call order = %v, want %v", gotNames, wantNames)
	}
	if got := string(response.Message.ToolCalls[1].Arguments); got != `{"service_id":"service-1"}` {
		t.Fatalf("second arguments = %q", got)
	}
}

func TestOpenAIAdapterUsesDirectChatCompletionsWithoutOpenRouterHeaders(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer direct-openai-key" {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.Header.Get("HTTP-Referer"); got != "" {
			t.Errorf("HTTP-Referer = %q, want empty", got)
		}
		if got := request.Header.Get("X-OpenRouter-Title"); got != "" {
			t.Errorf("X-OpenRouter-Title = %q, want empty", got)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(openRouterSuccessBody(3)))
	})

	adapter, err := NewOpenAIAdapter(OpenAIConfig{
		APIKey: "direct-openai-key", Model: "test-model",
		Endpoint: "https://openai.test/v1/chat/completions", HTTPClient: recorderClient(handler),
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Usage.Total() != 3 {
		t.Fatalf("usage = %#v", response.Usage)
	}
}

func TestOpenRouterAdapterReturnsTypedHTTPError(t *testing.T) {
	t.Parallel()
	handler := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Retry-After", "7")
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte(`{"error":{"code":503,"message":"No provider available"}}`))
	})

	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: recorderClient(handler), MaxAttempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10,
	})
	var providerError *OpenRouterError
	if !errors.As(err, &providerError) {
		t.Fatalf("error = %T %v, want *OpenRouterError", err, err)
	}
	if providerError.StatusCode != http.StatusServiceUnavailable || providerError.Code != "503" ||
		providerError.RetryAfter != 7*time.Second || !providerError.Retryable() {
		t.Fatalf("provider error = %#v", providerError)
	}
}

func TestOpenRouterAdapterRetriesTransientTransportErrorsWithDefaultBound(t *testing.T) {
	t.Parallel()
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		if attempts < 3 {
			return nil, io.ErrUnexpectedEOF
		}
		return openRouterTestResponse(http.StatusOK, nil, openRouterSuccessBody(11)), nil
	})}
	var waits []time.Duration
	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: client,
		RetryBaseDelay: 100 * time.Millisecond, RetryMaxDelay: time.Second,
		RetryJitter: func(time.Duration) time.Duration { return 0 },
		RetryWait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != defaultOpenRouterAttempts {
		t.Fatalf("attempts = %d, want default %d", attempts, defaultOpenRouterAttempts)
	}
	wantWaits := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond}
	if !reflect.DeepEqual(waits, wantWaits) {
		t.Fatalf("waits = %v, want exponential waits %v", waits, wantWaits)
	}
	if response.Usage.Total() != 11 {
		t.Fatalf("usage = %#v", response.Usage)
	}
	if !response.UsageIncomplete {
		t.Fatal("transport retry without usage was not marked incomplete")
	}
}

func TestOpenRouterAdapterRetriesOnlyConfiguredHTTPStatuses(t *testing.T) {
	t.Parallel()
	for _, status := range []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			attempts := 0
			var waits []time.Duration
			client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				attempts++
				if attempts == 1 {
					headers := http.Header{}
					if status == http.StatusTooManyRequests {
						headers.Set("Retry-After", "2")
					}
					return openRouterTestResponse(status, headers, `{"error":{"code":"retryable","message":"retry"}}`), nil
				}
				return openRouterTestResponse(http.StatusOK, nil, openRouterSuccessBody(13)), nil
			})}
			adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
				APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: client,
				MaxAttempts: 2, RetryBaseDelay: 100 * time.Millisecond, RetryMaxDelay: time.Second,
				RetryJitter: func(time.Duration) time.Duration { return 0 },
				RetryWait: func(_ context.Context, delay time.Duration) error {
					waits = append(waits, delay)
					return nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10}); err != nil {
				t.Fatal(err)
			}
			if attempts != 2 || len(waits) != 1 {
				t.Fatalf("attempts=%d waits=%v", attempts, waits)
			}
			if status == http.StatusTooManyRequests && waits[0] != 2*time.Second {
				t.Fatalf("Retry-After wait = %v, want 2s", waits[0])
			}
		})
	}
}

func TestOpenRouterAdapterRetriesEmbeddedProviderErrorInHTTP200Choice(t *testing.T) {
	t.Parallel()
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return openRouterTestResponse(http.StatusOK, nil, `{
				"id":"failed-generation",
				"choices":[{"finish_reason":"error","error":{"code":502,"message":"upstream provider failed"}}],
				"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
			}`), nil
		}
		return openRouterTestResponse(http.StatusOK, nil, openRouterSuccessBody(11)), nil
	})}
	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: client,
		MaxAttempts: 2,
		RetryJitter: func(time.Duration) time.Duration { return 0 },
		RetryWait:   func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || response.Usage.Total() != 16 || response.UsageIncomplete {
		t.Fatalf("attempts=%d response=%#v", attempts, response)
	}
}

func TestOpenRouterAdapterDoesNotRetryNonRetryableClientError(t *testing.T) {
	t.Parallel()
	attempts := 0
	waits := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return openRouterTestResponse(http.StatusBadRequest, nil, `{"error":{"code":400,"message":"invalid request"}}`), nil
	})}
	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: client,
		RetryWait: func(context.Context, time.Duration) error { waits++; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10})
	var providerError *OpenRouterError
	if !errors.As(err, &providerError) || providerError.StatusCode != http.StatusBadRequest {
		t.Fatalf("error = %T %v", err, err)
	}
	if attempts != 1 || waits != 0 {
		t.Fatalf("attempts=%d waits=%d, want 1/0", attempts, waits)
	}
}

func TestOpenRouterAdapterDoesNotRetryPermanentTransportError(t *testing.T) {
	t.Parallel()
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return nil, errors.New("permanent transport configuration error")
	})}
	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: client,
		RetryWait: func(context.Context, time.Duration) error {
			t.Fatal("retry wait called for permanent transport error")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10}); err == nil {
		t.Fatal("Complete succeeded")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestOpenRouterAdapterStopsAtConfiguredAttemptBound(t *testing.T) {
	t.Parallel()
	attempts := 0
	waits := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return nil, io.ErrUnexpectedEOF
	})}
	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: client,
		MaxAttempts: 2,
		RetryJitter: func(time.Duration) time.Duration { return 0 },
		RetryWait:   func(context.Context, time.Duration) error { waits++; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want unexpected EOF", err)
	}
	if attempts != 2 || waits != 1 {
		t.Fatalf("attempts=%d waits=%d, want 2/1", attempts, waits)
	}
}

func TestOpenRouterAdapterAccumulatesReportedUsageAcrossRetryAttempts(t *testing.T) {
	t.Parallel()
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return openRouterTestResponse(http.StatusBadGateway, nil,
				`{"error":{"code":502,"message":"provider disconnected"},"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`), nil
		}
		return openRouterTestResponse(http.StatusOK, nil, openRouterSuccessBody(11)), nil
	})}
	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: client,
		MaxAttempts: 2,
		RetryJitter: func(time.Duration) time.Duration { return 0 },
		RetryWait:   func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10})
	if err != nil {
		t.Fatal(err)
	}
	if response.Usage.Total() != 16 || response.Usage.InputTokens != 10 || response.Usage.OutputTokens != 6 {
		t.Fatalf("accumulated usage = %#v", response.Usage)
	}
	if response.UsageIncomplete {
		t.Fatal("fully reported retry usage was marked incomplete")
	}
	if response.UnknownUsageAttempts != 0 {
		t.Fatalf("unknown attempts = %d, want 0", response.UnknownUsageAttempts)
	}
}

func TestOpenRouterAdapterCountsAttemptsThatReportedNoUsage(t *testing.T) {
	t.Parallel()
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			// A failed attempt with no usage body: the tokens it consumed are
			// unknown, but the successful attempt below reports its own.
			return openRouterTestResponse(http.StatusBadGateway, nil, `{"error":{"code":502,"message":"provider disconnected"}}`), nil
		}
		return openRouterTestResponse(http.StatusOK, nil, openRouterSuccessBody(11)), nil
	})}
	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: client,
		MaxAttempts: 2,
		RetryJitter: func(time.Duration) time.Duration { return 0 },
		RetryWait:   func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10})
	if err != nil {
		t.Fatal(err)
	}
	// The count lets the runner price the gap instead of writing off the whole
	// worst-case reservation for a call that did report its usage.
	if !response.UsageIncomplete || response.UnknownUsageAttempts != 1 {
		t.Fatalf("incomplete=%v unknown attempts=%d, want true/1", response.UsageIncomplete, response.UnknownUsageAttempts)
	}
	if response.Usage.Total() != 11 {
		t.Fatalf("usage = %#v", response.Usage)
	}
}

func TestOpenRouterAdapterRetryWaitHonorsAdapterContext(t *testing.T) {
	t.Parallel()
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return openRouterTestResponse(http.StatusServiceUnavailable, nil, `{"error":{"code":503,"message":"retry"}}`), nil
	})}
	waitCalled := false
	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: client,
		Timeout: time.Second,
		RetryWait: func(ctx context.Context, _ time.Duration) error {
			waitCalled = true
			if _, hasDeadline := ctx.Deadline(); !hasDeadline {
				t.Error("retry context has no adapter deadline")
			}
			return context.DeadlineExceeded
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10})
	if !errors.Is(err, context.DeadlineExceeded) || attempts != 1 || !waitCalled {
		t.Fatalf("error=%v attempts=%d waitCalled=%v", err, attempts, waitCalled)
	}
}

func TestOpenRouterAdapterRejectsNonTextAssistantContent(t *testing.T) {
	t.Parallel()
	handler := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}]}`))
	})
	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: recorderClient(handler)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}, MaxOutputTokens: 10}); err == nil {
		t.Fatal("Complete succeeded for unsupported multimodal response")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func recorderClient(handler http.Handler) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
}

func openRouterTestResponse(status int, headers http.Header, body string) *http.Response {
	recorder := httptest.NewRecorder()
	for name, values := range headers {
		for _, value := range values {
			recorder.Header().Add(name, value)
		}
	}
	recorder.WriteHeader(status)
	_, _ = recorder.WriteString(body)
	return recorder.Result()
}

func openRouterSuccessBody(totalTokens int) string {
	return `{"id":"gen-retry","model":"model","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":7,"completion_tokens":` +
		strconv.Itoa(totalTokens-7) + `,"total_tokens":` + strconv.Itoa(totalTokens) + `}}`
}

func TestSanitizeParametersForProviderStripsUnsupportedKeys(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"maxProperties":0,
		"additionalProperties":false,
		"required":["service_id","date_from"],
		"properties":{
			"service_id":{
				"type":"string",
				"format":"uuid",
				"minLength":1,
				"maxLength":200,
				"pattern":"^[A-Za-z0-9]+$",
				"description":"The service identifier"
			},
			"date_from":{"type":"string","format":"date-time"},
			"notes":{"type":"string","minLength":1,"maxLength":500}
		}
	}`)
	got := sanitizeParametersForProvider(input)
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal sanitized output: %v", err)
	}

	// Top-level unsupported keys must be gone.
	for _, key := range []string{"$schema", "maxProperties"} {
		if _, exists := parsed[key]; exists {
			t.Errorf("expected top-level key %q to be removed", key)
		}
	}
	// Top-level supported keys must survive.
	for _, key := range []string{"type", "additionalProperties", "required", "properties"} {
		if _, exists := parsed[key]; !exists {
			t.Errorf("expected top-level key %q to be preserved", key)
		}
	}

	// Nested property-level keys.
	properties, _ := parsed["properties"].(map[string]any)
	serviceID, _ := properties["service_id"].(map[string]any)
	for _, key := range []string{"minLength", "maxLength", "pattern", "format"} {
		if _, exists := serviceID[key]; exists {
			t.Errorf("expected service_id key %q to be removed", key)
		}
	}
	for _, key := range []string{"type", "description"} {
		if _, exists := serviceID[key]; !exists {
			t.Errorf("expected service_id key %q to be preserved", key)
		}
	}
	notes, _ := properties["notes"].(map[string]any)
	if _, exists := notes["minLength"]; exists {
		t.Error("expected notes minLength to be removed")
	}
	if _, exists := notes["maxLength"]; exists {
		t.Error("expected notes maxLength to be removed")
	}

	// A stripped constraint must survive as prose, otherwise the model generates
	// values the server-side gateway then rejects.
	serviceDescription, _ := serviceID["description"].(string)
	for _, want := range []string{
		"The service identifier",
		"must be a valid uuid",
		"must match the regular expression ^[A-Za-z0-9]+$",
		"at least 1 characters",
		"at most 200 characters",
	} {
		if !strings.Contains(serviceDescription, want) {
			t.Errorf("service_id description %q does not carry %q", serviceDescription, want)
		}
	}
	notesDescription, _ := notes["description"].(string)
	if !strings.Contains(notesDescription, "at most 500 characters") {
		t.Errorf("notes description %q lost its length constraint", notesDescription)
	}
	// Sanitization must stay deterministic so identical tool definitions produce
	// an identical wire payload on every request.
	if second := sanitizeParametersForProvider(input); !bytes.Equal(got, second) {
		t.Errorf("sanitized output is not deterministic:\n%s\n%s", got, second)
	}
}

func TestSanitizeParametersForProviderPreservesArraySchema(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{
		"type":"object",
		"properties":{
			"tags":{"type":"array","items":{"type":"string","minLength":1,"maxLength":50}}
		}
	}`)
	got := sanitizeParametersForProvider(input)
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	properties, _ := parsed["properties"].(map[string]any)
	tags, _ := properties["tags"].(map[string]any)
	items, _ := tags["items"].(map[string]any)
	if _, exists := items["type"]; !exists {
		t.Error("items.type must be preserved")
	}
	if _, exists := items["minLength"]; exists {
		t.Error("items.minLength must be removed")
	}
}

func TestSanitizeParametersForProviderInvalidJSON(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`not valid json`)
	got := sanitizeParametersForProvider(input)
	if string(got) != string(input) {
		t.Fatalf("expected invalid JSON to be returned as-is, got %q", string(got))
	}
}
