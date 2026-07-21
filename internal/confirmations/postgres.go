// Package confirmations persists two-phase mutation authorization. The LLM
// can propose an action, but only a later customer message can authorize it.
package confirmations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/platform/ids"
	"github.com/reinhlord/kontor/internal/tools"
)

type PostgreSQL struct {
	pool *pgxpool.Pool
}

func NewPostgreSQL(pool *pgxpool.Pool) *PostgreSQL { return &PostgreSQL{pool: pool} }

func (s *PostgreSQL) Propose(
	ctx context.Context,
	binding tools.ConfirmationBinding,
	proposal tools.ConfirmationProposal,
	now time.Time,
) (tools.ConfirmationProposal, error) {
	if binding.ProposedFromMessageID == "" || len(binding.ArgumentsJSON) == 0 {
		return tools.ConfirmationProposal{}, errors.New("confirmation proposal lacks frozen action context")
	}
	if proposal.ID == "" {
		proposal.ID = ids.New()
	}
	payload, err := json.Marshal(proposal)
	if err != nil {
		return tools.ConfirmationProposal{}, fmt.Errorf("encode confirmation summary: %w", err)
	}
	hash := hex.EncodeToString(binding.ArgumentsHash[:])

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return tools.ConfirmationProposal{}, fmt.Errorf("begin confirmation proposal: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	// Serialize proposals per conversation so free-text consent can never face
	// two live actions.
	if err := tx.QueryRow(ctx, `
		SELECT id FROM conversations
		WHERE tenant_id=$1 AND id=$2 AND customer_id=$3
		FOR UPDATE`, binding.TenantID, binding.ConversationID, binding.OwnerCustomerID).Scan(new(string)); err != nil {
		return tools.ConfirmationProposal{}, fmt.Errorf("lock confirmation conversation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE action_proposals SET status='expired'
		WHERE tenant_id=$1 AND conversation_id=$2 AND customer_id=$3
		  AND status IN ('pending','confirmed') AND expires_at<=$4`,
		binding.TenantID, binding.ConversationID, binding.OwnerCustomerID, now); err != nil {
		return tools.ConfirmationProposal{}, fmt.Errorf("expire confirmation proposals: %w", err)
	}

	var existingID string
	var existingPayload string
	err = tx.QueryRow(ctx, `
		SELECT id::text,summary
		FROM action_proposals
		WHERE tenant_id=$1 AND customer_id=$2 AND conversation_id=$3
		  AND tool_name=$4 AND arguments_hash=$5
		  AND status IN ('pending','confirmed') AND expires_at>$6
		ORDER BY created_at DESC LIMIT 1 FOR UPDATE`,
		binding.TenantID, binding.OwnerCustomerID, binding.ConversationID,
		binding.Tool, hash, now).Scan(&existingID, &existingPayload)
	if err == nil {
		var existing tools.ConfirmationProposal
		if json.Unmarshal([]byte(existingPayload), &existing) == nil {
			if err := tx.Commit(ctx); err != nil {
				return tools.ConfirmationProposal{}, fmt.Errorf("commit confirmation replay: %w", err)
			}
			return existing, nil
		}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return tools.ConfirmationProposal{}, fmt.Errorf("find confirmation proposal: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE action_proposals SET status='rejected'
		WHERE tenant_id=$1 AND conversation_id=$2 AND customer_id=$3
		  AND status IN ('pending','confirmed')`,
		binding.TenantID, binding.ConversationID, binding.OwnerCustomerID); err != nil {
		return tools.ConfirmationProposal{}, fmt.Errorf("replace confirmation proposal: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO action_proposals
			(tenant_id,id,conversation_id,customer_id,tool_name,arguments_json,
			 arguments_hash,summary,status,expires_at,proposed_message_id)
		VALUES($1,$2,$3,$4,$5,$6::jsonb,$7,$8,'pending',$9,$10)`,
		binding.TenantID, proposal.ID, binding.ConversationID, binding.OwnerCustomerID,
		binding.Tool, string(binding.ArgumentsJSON), hash, string(payload), proposal.ExpiresAt,
		binding.ProposedFromMessageID)
	if err != nil {
		return tools.ConfirmationProposal{}, fmt.Errorf("insert confirmation proposal: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return tools.ConfirmationProposal{}, fmt.Errorf("commit confirmation proposal: %w", err)
	}
	return proposal, nil
}

func (s *PostgreSQL) Latest(
	ctx context.Context,
	tenantID, ownerCustomerID, conversationID string,
	now time.Time,
) (tools.ConfirmationState, bool, error) {
	var state tools.ConfirmationState
	var proposalPayload string
	var arguments []byte
	var hashText, proposedMessageID string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text,tool_name,arguments_json,arguments_hash,summary,status,
		       expires_at,proposed_message_id::text
		FROM action_proposals
		WHERE tenant_id=$1 AND customer_id=$2 AND conversation_id=$3
		  AND status IN ('pending','confirmed') AND expires_at>$4
		ORDER BY created_at DESC LIMIT 1`, tenantID, ownerCustomerID, conversationID, now).
		Scan(&state.Proposal.ID, &state.Binding.Tool, &arguments, &hashText,
			&proposalPayload, &state.Status, &state.Proposal.ExpiresAt, &proposedMessageID)
	if errors.Is(err, pgx.ErrNoRows) {
		return tools.ConfirmationState{}, false, nil
	}
	if err != nil {
		return tools.ConfirmationState{}, false, fmt.Errorf("get latest confirmation: %w", err)
	}
	if err := json.Unmarshal([]byte(proposalPayload), &state.Proposal); err != nil {
		return tools.ConfirmationState{}, false, fmt.Errorf("decode confirmation summary: %w", err)
	}
	hashBytes, err := hex.DecodeString(hashText)
	if err != nil || len(hashBytes) != sha256.Size {
		return tools.ConfirmationState{}, false, errors.New("stored confirmation hash is invalid")
	}
	copy(state.Binding.ArgumentsHash[:], hashBytes)
	state.Binding.TenantID = tenantID
	state.Binding.OwnerCustomerID = ownerCustomerID
	state.Binding.ConversationID = conversationID
	state.Binding.ProposedFromMessageID = proposedMessageID
	state.Binding.ArgumentsJSON = append(json.RawMessage(nil), arguments...)
	if state.Status == "confirmed" {
		state.Status = "authorized"
	}
	return state, true, nil
}

func (s *PostgreSQL) Authorize(ctx context.Context, id string, confirmedBy tools.TrustedContext, now time.Time) error {
	if confirmedBy.InboundMessageID == "" {
		return tools.ErrConfirmationInvalid
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE action_proposals
		SET status='confirmed',confirmed_message_id=$5,confirmed_at=$6
		WHERE tenant_id=$1 AND id=$2 AND customer_id=$3 AND conversation_id=$4
		  AND status='pending' AND expires_at>$6
		  AND proposed_message_id<>$5`,
		confirmedBy.TenantID, id, confirmedBy.CustomerID, confirmedBy.ConversationID,
		confirmedBy.InboundMessageID, now)
	if err != nil {
		return fmt.Errorf("authorize confirmation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return s.classifyFailure(ctx, id, confirmedBy.TenantID, confirmedBy.CustomerID, confirmedBy.ConversationID, now)
	}
	return nil
}

func (s *PostgreSQL) VerifyAuthorized(ctx context.Context, id string, binding tools.ConfirmationBinding, now time.Time) error {
	var toolName, hashText, status string
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT tool_name,arguments_hash,status,expires_at
		FROM action_proposals
		WHERE tenant_id=$1 AND id=$2 AND customer_id=$3 AND conversation_id=$4`,
		binding.TenantID, id, binding.OwnerCustomerID, binding.ConversationID).
		Scan(&toolName, &hashText, &status, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return tools.ErrConfirmationInvalid
	}
	if err != nil {
		return fmt.Errorf("verify confirmation: %w", err)
	}
	if !now.Before(expiresAt) {
		return tools.ErrConfirmationExpired
	}
	if toolName != binding.Tool || hashText != hex.EncodeToString(binding.ArgumentsHash[:]) {
		return tools.ErrConfirmationStale
	}
	if status != "confirmed" && status != "consumed" {
		return tools.ErrConfirmationInvalid
	}
	return nil
}

func (s *PostgreSQL) MarkConsumed(ctx context.Context, id string, binding tools.ConfirmationBinding, now time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE action_proposals
		SET status='consumed',consumed_at=COALESCE(consumed_at,$6)
		WHERE tenant_id=$1 AND id=$2 AND customer_id=$3 AND conversation_id=$4
		  AND tool_name=$5 AND arguments_hash=$7 AND status IN ('confirmed','consumed')
		  AND expires_at>$6`,
		binding.TenantID, id, binding.OwnerCustomerID, binding.ConversationID,
		binding.Tool, now, hex.EncodeToString(binding.ArgumentsHash[:]))
	if err != nil {
		return fmt.Errorf("consume confirmation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return tools.ErrConfirmationInvalid
	}
	return nil
}

func (s *PostgreSQL) classifyFailure(
	ctx context.Context,
	id, tenantID, customerID, conversationID string,
	now time.Time,
) error {
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT expires_at FROM action_proposals
		WHERE tenant_id=$1 AND id=$2 AND customer_id=$3 AND conversation_id=$4`,
		tenantID, id, customerID, conversationID).Scan(&expiresAt)
	if err != nil {
		return tools.ErrConfirmationInvalid
	}
	if !now.Before(expiresAt) {
		return tools.ErrConfirmationExpired
	}
	return tools.ErrConfirmationInvalid
}

var _ tools.ConfirmationStore = (*PostgreSQL)(nil)
