// Package agentbudget connects the agent's reservation interface to the
// database-enforced per-conversation token cap.
package agentbudget

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/reinhlord/kontor/internal/agent"
	"github.com/reinhlord/kontor/internal/conversations"
)

type PostgreSQL struct {
	store    *conversations.Store
	tenantID string
}

func NewPostgreSQL(store *conversations.Store, tenantID string) *PostgreSQL {
	return &PostgreSQL{store: store, tenantID: tenantID}
}

func (b *PostgreSQL) Reserve(ctx context.Context, conversationID string, tokens int) (agent.TokenReservation, error) {
	if err := b.store.ReserveTokens(ctx, b.tenantID, conversationID, tokens); err != nil {
		if errors.Is(err, conversations.ErrBudgetExceeded) {
			return nil, fmt.Errorf("%w: requested=%d", agent.ErrTokenBudgetExceeded, tokens)
		}
		return nil, err
	}
	return &reservation{
		store: b.store, tenantID: b.tenantID, conversationID: conversationID, reserved: tokens,
	}, nil
}

type reservation struct {
	mu             sync.Mutex
	store          *conversations.Store
	tenantID       string
	conversationID string
	reserved       int
	settled        bool
}

func (r *reservation) ReservedTokens() int { return r.reserved }

func (r *reservation) Settle(ctx context.Context, actualTokens int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.settled {
		return errors.New("token reservation already settled")
	}
	r.settled = true
	if actualTokens < 0 || actualTokens > r.reserved {
		// Preserve the hard cap: charge the complete reservation on an
		// impossible provider report instead of crediting unknown usage.
		if err := r.store.SettleTokens(ctx, r.tenantID, r.conversationID, r.reserved, r.reserved); err != nil {
			return err
		}
		return fmt.Errorf("%w: reserved=%d actual=%d", agent.ErrUsageExceedsReservation, r.reserved, actualTokens)
	}
	return r.store.SettleTokens(ctx, r.tenantID, r.conversationID, r.reserved, actualTokens)
}

var _ agent.TokenBudget = (*PostgreSQL)(nil)
