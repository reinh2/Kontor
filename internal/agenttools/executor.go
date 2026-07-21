// Package agenttools adapts the persisted agent context to the policy-enforcing
// tools gateway and records retry attempts beneath one parent tool call.
package agenttools

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/agent"
	"github.com/reinhlord/kontor/internal/tools"
)

type Executor struct {
	pool        *pgxpool.Pool
	gateway     *tools.Gateway
	tenantID    string
	maxAttempts int
	timeout     time.Duration
	baseBackoff time.Duration
}

func NewExecutor(pool *pgxpool.Pool, gateway *tools.Gateway, tenantID string, maxAttempts int, timeout time.Duration) *Executor {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Executor{
		pool: pool, gateway: gateway, tenantID: tenantID,
		maxAttempts: maxAttempts, timeout: timeout, baseBackoff: 100 * time.Millisecond,
	}
}

func (e *Executor) Execute(ctx context.Context, request agent.ToolRequest) (agent.ToolExecution, error) {
	trusted, err := e.resolveTrustedContext(ctx, request)
	if err != nil {
		return agent.ToolExecution{}, err
	}

	var execution agent.ToolExecution
	for attemptNo := 1; attemptNo <= e.maxAttempts; attemptNo++ {
		started := time.Now()
		attemptCtx, cancel := context.WithTimeout(ctx, e.timeout)
		result := e.gateway.Execute(attemptCtx, trusted, tools.Call{
			ID: request.Call.ID, Name: request.Call.Name, Arguments: request.Call.Arguments,
		})
		attemptErr := attemptCtx.Err()
		cancel()

		payload, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return execution, fmt.Errorf("encode tool result: %w", marshalErr)
		}
		status := agent.ToolStatusSuccess
		if result.Status == tools.StatusError || attemptErr != nil {
			status = agent.ToolStatusError
		}
		execution.Content = payload
		execution.IsError = result.Status == tools.StatusError || attemptErr != nil
		switch result.Status {
		case tools.StatusSuccess:
			execution.Status = agent.ToolStatusSucceeded
		case tools.StatusConfirmationRequired:
			execution.Status = agent.ToolStatusConfirmationRequired
		case tools.StatusError:
			execution.Status = agent.ToolStatusFailed
			if result.Error != nil && (result.Error.Code == tools.CodePolicyDenied ||
				result.Error.Code == tools.CodeNotFoundOrNotOwned || result.Error.Code == tools.CodeToolNotAllowed) {
				execution.Status = agent.ToolStatusRefused
			}
		}
		execution.Attempts = append(execution.Attempts, agent.ToolAttempt{
			StartedAt: started, FinishedAt: time.Now(), Status: status, Detail: append(json.RawMessage(nil), payload...),
		})

		if attemptErr != nil {
			if !errors.Is(attemptErr, context.DeadlineExceeded) || attemptNo == e.maxAttempts {
				return execution, attemptErr
			}
		} else if result.Status != tools.StatusError || result.Error == nil || !result.Error.Retryable {
			return execution, nil
		} else if attemptNo == e.maxAttempts {
			return execution, nil
		}

		if err := waitBackoff(ctx, e.baseBackoff, attemptNo); err != nil {
			return execution, err
		}
	}
	return execution, nil
}

func (e *Executor) resolveTrustedContext(ctx context.Context, request agent.ToolRequest) (tools.TrustedContext, error) {
	var trusted tools.TrustedContext
	err := e.pool.QueryRow(ctx, `
		SELECT r.tenant_id::text,c.customer_id::text,r.conversation_id::text,
		       COALESCE(r.trigger_message_id::text,'')
		FROM agent_runs r
		JOIN conversations c ON c.tenant_id=r.tenant_id AND c.id=r.conversation_id
		WHERE r.tenant_id=$1 AND r.id=$2 AND r.conversation_id=$3`,
		e.tenantID, request.RunID, request.ConversationID).
		Scan(&trusted.TenantID, &trusted.CustomerID, &trusted.ConversationID, &trusted.InboundMessageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return tools.TrustedContext{}, errors.New("trusted agent run context not found")
	}
	if err != nil {
		return tools.TrustedContext{}, fmt.Errorf("resolve trusted tool context: %w", err)
	}
	trusted.Capabilities = map[tools.Capability]bool{
		tools.CapabilityScheduleRead:         true,
		tools.CapabilityBookingCreateSelf:    true,
		tools.CapabilityBookingWriteSelf:     true,
		tools.CapabilityCRMContactSelf:       true,
		tools.CapabilityCRMDealAfterBook:     true,
		tools.CapabilityConversationEscalate: true,
	}
	return trusted, nil
}

func waitBackoff(ctx context.Context, base time.Duration, attempt int) error {
	max := base << (attempt - 1)
	if max > 2*time.Second {
		max = 2 * time.Second
	}
	var random [8]byte
	_, _ = rand.Read(random[:])
	delay := max/2 + time.Duration(binary.LittleEndian.Uint64(random[:])%uint64(max/2+1))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var _ agent.ToolExecutor = (*Executor)(nil)
