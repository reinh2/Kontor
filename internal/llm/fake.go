package llm

import (
	"context"
	"errors"
	"sync"
)

// ErrFakeExhausted is returned when a scripted fake receives more calls than
// it has configured steps.
var ErrFakeExhausted = errors.New("llm fake: scripted responses exhausted")

// FakeStep is one deterministic result from FakeAdapter.
type FakeStep struct {
	Response Response
	Err      error
}

// FakeAdapter is a concurrency-safe, deterministic adapter for tests and demo
// scenarios. Each Complete call consumes exactly one step.
type FakeAdapter struct {
	mu       sync.Mutex
	steps    []FakeStep
	requests []Request
}

// NewFakeAdapter returns a fake backed by a copy of steps.
func NewFakeAdapter(steps ...FakeStep) *FakeAdapter {
	return &FakeAdapter{steps: append([]FakeStep(nil), steps...)}
}

// Complete records the request and consumes the next scripted step.
func (f *FakeAdapter) Complete(ctx context.Context, request Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests = append(f.requests, cloneRequest(request))
	if len(f.steps) == 0 {
		return Response{}, ErrFakeExhausted
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	return cloneResponse(step.Response), step.Err
}

// Requests returns a deep copy of requests observed so far.
func (f *FakeAdapter) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()

	requests := make([]Request, len(f.requests))
	for i := range f.requests {
		requests[i] = cloneRequest(f.requests[i])
	}
	return requests
}

func cloneRequest(request Request) Request {
	cloned := request
	cloned.Messages = make([]Message, len(request.Messages))
	for i := range request.Messages {
		cloned.Messages[i] = cloneMessage(request.Messages[i])
	}
	cloned.Tools = make([]ToolDefinition, len(request.Tools))
	for i, tool := range request.Tools {
		cloned.Tools[i] = tool
		cloned.Tools[i].Parameters = append([]byte(nil), tool.Parameters...)
	}
	return cloned
}

func cloneResponse(response Response) Response {
	cloned := response
	cloned.Message = cloneMessage(response.Message)
	return cloned
}

func cloneMessage(message Message) Message {
	cloned := message
	cloned.ToolCalls = make([]ToolCall, len(message.ToolCalls))
	for i, call := range message.ToolCalls {
		cloned.ToolCalls[i] = call
		cloned.ToolCalls[i].Arguments = append([]byte(nil), call.Arguments...)
	}
	return cloned
}
