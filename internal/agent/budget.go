package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/reinhlord/kontor/internal/llm"
)

var (
	// ErrTokenBudgetExceeded means a model call was refused before reaching the
	// provider because its conservative reservation would cross the hard cap.
	ErrTokenBudgetExceeded = errors.New("agent: conversation token budget exceeded")
	// ErrUsageExceedsReservation indicates that a provider reported more tokens
	// than the conservative reservation. The full reservation remains charged
	// and no accounting credit is returned.
	ErrUsageExceedsReservation = errors.New("agent: provider usage exceeds token reservation")
)

// TokenBudget atomically reserves capacity for a conversation before a model
// call. Implementations can persist this state in PostgreSQL; Stage 1 includes
// a concurrency-safe in-memory implementation.
type TokenBudget interface {
	Reserve(ctx context.Context, conversationID string, tokens int) (TokenReservation, error)
}

// TokenReservation settles a conservative reservation with provider-reported
// usage. Settlement must be called at most once.
type TokenReservation interface {
	ReservedTokens() int
	Settle(ctx context.Context, actualTokens int) error
}

// TokenEstimator computes the reservation needed for one provider request.
type TokenEstimator interface {
	Estimate(request llm.Request) (int, error)
}

// ConservativeTokenEstimator reserves at least one token per serialized byte,
// plus chat-template overhead and the requested maximum completion. Byte count
// deliberately overestimates normal BPE tokenization and avoids relying on a
// provider-specific tokenizer.
type ConservativeTokenEstimator struct {
	BaseOverhead       int
	PerMessageOverhead int
	PerToolOverhead    int
	// ProviderAttempts reserves the worst-case cost of retries performed inside
	// one Adapter.Complete call. A value below 1 means one attempt.
	ProviderAttempts int
}

// Estimate implements TokenEstimator.
func (e ConservativeTokenEstimator) Estimate(request llm.Request) (int, error) {
	if request.MaxOutputTokens <= 0 {
		return 0, errors.New("agent: max output tokens must be positive")
	}
	base := e.BaseOverhead
	if base <= 0 {
		base = 256
	}
	perMessage := e.PerMessageOverhead
	if perMessage <= 0 {
		perMessage = 64
	}
	perTool := e.PerToolOverhead
	if perTool <= 0 {
		perTool = 64
	}

	bytes := 0
	for _, message := range request.Messages {
		bytes += len(message.Role) + len(message.Content) + len(message.Name) + len(message.ToolCallID)
		for _, call := range message.ToolCalls {
			bytes += len(call.ID) + len(call.Name) + len(call.Arguments)
		}
	}
	for _, tool := range request.Tools {
		bytes += len(tool.Name) + len(tool.Description) + len(tool.Parameters)
	}
	perAttempt := request.MaxOutputTokens + bytes + base +
		(len(request.Messages) * perMessage) + (len(request.Tools) * perTool)
	attempts := e.ProviderAttempts
	if attempts < 1 {
		attempts = 1
	}
	if perAttempt > math.MaxInt/attempts {
		return 0, errors.New("agent: token reservation estimate overflow")
	}
	return perAttempt * attempts, nil
}

// MemoryTokenBudget enforces one fixed hard cap per conversation. Accounted
// tokens include both settled usage and reservations for in-flight calls.
type MemoryTokenBudget struct {
	mu        sync.Mutex
	limit     int
	accounted map[string]int
}

// NewMemoryTokenBudget constructs a per-conversation budget.
func NewMemoryTokenBudget(limit int) (*MemoryTokenBudget, error) {
	if limit <= 0 {
		return nil, errors.New("agent: token budget limit must be positive")
	}
	return &MemoryTokenBudget{
		limit:     limit,
		accounted: make(map[string]int),
	}, nil
}

// Reserve atomically charges tokens until the reservation is settled.
func (b *MemoryTokenBudget) Reserve(ctx context.Context, conversationID string, tokens int) (TokenReservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if conversationID == "" {
		return nil, errors.New("agent: conversation ID is required for token accounting")
	}
	if tokens <= 0 {
		return nil, errors.New("agent: token reservation must be positive")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if tokens > b.limit-b.accounted[conversationID] {
		return nil, fmt.Errorf("%w: limit=%d accounted=%d requested=%d", ErrTokenBudgetExceeded, b.limit, b.accounted[conversationID], tokens)
	}
	b.accounted[conversationID] += tokens
	return &memoryTokenReservation{
		budget:         b,
		conversationID: conversationID,
		reserved:       tokens,
	}, nil
}

// Accounted returns settled usage plus in-flight reservations. It is intended
// for metrics and tests.
func (b *MemoryTokenBudget) Accounted(conversationID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.accounted[conversationID]
}

// Limit returns the fixed cap applied to every conversation.
func (b *MemoryTokenBudget) Limit() int {
	return b.limit
}

type memoryTokenReservation struct {
	mu             sync.Mutex
	budget         *MemoryTokenBudget
	conversationID string
	reserved       int
	settled        bool
}

func (r *memoryTokenReservation) ReservedTokens() int {
	return r.reserved
}

func (r *memoryTokenReservation) Settle(ctx context.Context, actualTokens int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if actualTokens < 0 {
		return errors.New("agent: actual token usage cannot be negative")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.settled {
		return errors.New("agent: token reservation already settled")
	}
	r.settled = true
	if actualTokens > r.reserved {
		return fmt.Errorf("%w: reserved=%d actual=%d", ErrUsageExceedsReservation, r.reserved, actualTokens)
	}

	r.budget.mu.Lock()
	r.budget.accounted[r.conversationID] -= r.reserved - actualTokens
	r.budget.mu.Unlock()
	return nil
}
