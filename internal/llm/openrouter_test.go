package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestOpenRouterAdapterReturnsTypedHTTPError(t *testing.T) {
	t.Parallel()
	handler := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Retry-After", "7")
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte(`{"error":{"code":503,"message":"No provider available"}}`))
	})

	adapter, err := NewOpenRouterAdapter(OpenRouterConfig{
		APIKey: "key", Model: "model", Endpoint: "https://openrouter.test", HTTPClient: recorderClient(handler),
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
