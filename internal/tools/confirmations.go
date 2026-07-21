package tools

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var (
	ErrConfirmationInvalid = errors.New("confirmation is invalid")
	ErrConfirmationExpired = errors.New("confirmation has expired")
	ErrConfirmationStale   = errors.New("confirmation does not match this action")
)

type ConfirmationFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type ConfirmationProposal struct {
	ID        string             `json:"id"`
	Action    string             `json:"action"`
	Title     string             `json:"title"`
	Facts     []ConfirmationFact `json:"facts"`
	ExpiresAt time.Time          `json:"expires_at"`
}

type ConfirmationBinding struct {
	TenantID              string
	OwnerCustomerID       string
	ConversationID        string
	ProposedFromMessageID string
	Tool                  string
	ArgumentsHash         [sha256.Size]byte
	// ArgumentsJSON is the exact canonical action frozen when the proposal is
	// shown. It excludes confirmation_id and is safe to persist/re-inject.
	ArgumentsJSON json.RawMessage
}

type ConfirmationState struct {
	Proposal ConfirmationProposal
	Binding  ConfirmationBinding
	Status   string
}

// ConfirmationStore is the durable boundary for two-phase authorization.
// Authorize must only be called after a channel has observed explicit consent.
type ConfirmationStore interface {
	Propose(ctx context.Context, binding ConfirmationBinding, proposal ConfirmationProposal, now time.Time) (ConfirmationProposal, error)
	Latest(ctx context.Context, tenantID, ownerCustomerID, conversationID string, now time.Time) (ConfirmationState, bool, error)
	Authorize(ctx context.Context, confirmationID string, confirmedBy TrustedContext, now time.Time) error
	VerifyAuthorized(ctx context.Context, confirmationID string, binding ConfirmationBinding, now time.Time) error
	MarkConsumed(ctx context.Context, confirmationID string, binding ConfirmationBinding, now time.Time) error
}

// MemoryConfirmationStore is race-safe and useful for tests/demo. Production
// can implement ConfirmationStore using PostgreSQL with equivalent atomic state
// transitions.
type MemoryConfirmationStore struct {
	mu      sync.Mutex
	records map[string]ConfirmationState
}

func NewMemoryConfirmationStore() *MemoryConfirmationStore {
	return &MemoryConfirmationStore{records: make(map[string]ConfirmationState)}
}

func (s *MemoryConfirmationStore) Propose(_ context.Context, binding ConfirmationBinding, proposal ConfirmationProposal, now time.Time) (ConfirmationProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, record := range s.records {
		if (record.Status == "pending" || record.Status == "authorized") &&
			record.Proposal.ExpiresAt.After(now) && equalBinding(record.Binding, binding) {
			return cloneConfirmationState(record).Proposal, nil
		}
	}
	if proposal.ID == "" {
		proposal.ID = newUUID()
	}
	record := cloneConfirmationState(ConfirmationState{
		Proposal: proposal,
		Binding:  binding,
		Status:   "pending",
	})
	s.records[proposal.ID] = record
	return cloneConfirmationState(record).Proposal, nil
}

func (s *MemoryConfirmationStore) Latest(_ context.Context, tenantID, ownerCustomerID, conversationID string, now time.Time) (ConfirmationState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var latest ConfirmationState
	found := false
	for _, record := range s.records {
		if record.Binding.TenantID != tenantID || record.Binding.OwnerCustomerID != ownerCustomerID ||
			record.Binding.ConversationID != conversationID || !now.Before(record.Proposal.ExpiresAt) ||
			(record.Status != "pending" && record.Status != "authorized") {
			continue
		}
		if !found || record.Proposal.ExpiresAt.After(latest.Proposal.ExpiresAt) {
			latest = cloneConfirmationState(record)
			found = true
		}
	}
	return latest, found, nil
}

func (s *MemoryConfirmationStore) Authorize(_ context.Context, id string, confirmedBy TrustedContext, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.records[id]
	if !ok || record.Binding.TenantID != confirmedBy.TenantID ||
		record.Binding.OwnerCustomerID != confirmedBy.CustomerID ||
		record.Binding.ConversationID != confirmedBy.ConversationID {
		return ErrConfirmationInvalid
	}
	if !now.Before(record.Proposal.ExpiresAt) {
		return ErrConfirmationExpired
	}
	if record.Status != "pending" || confirmedBy.InboundMessageID == "" ||
		confirmedBy.InboundMessageID == record.Binding.ProposedFromMessageID {
		return ErrConfirmationInvalid
	}
	record.Status = "authorized"
	s.records[id] = record
	return nil
}

func (s *MemoryConfirmationStore) VerifyAuthorized(_ context.Context, id string, binding ConfirmationBinding, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.records[id]
	if !ok {
		return ErrConfirmationInvalid
	}
	if !now.Before(record.Proposal.ExpiresAt) {
		return ErrConfirmationExpired
	}
	if !equalBinding(record.Binding, binding) {
		return ErrConfirmationStale
	}
	// A consumed grant may replay only the identical canonical action. The
	// backend idempotency key is part of that binding, so this cannot create a
	// second effect and closes the commit/response crash window.
	if record.Status != "authorized" && record.Status != "consumed" {
		return ErrConfirmationInvalid
	}
	return nil
}

func (s *MemoryConfirmationStore) MarkConsumed(_ context.Context, id string, binding ConfirmationBinding, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.records[id]
	if !ok {
		return ErrConfirmationInvalid
	}
	if !now.Before(record.Proposal.ExpiresAt) {
		return ErrConfirmationExpired
	}
	if !equalBinding(record.Binding, binding) {
		return ErrConfirmationStale
	}
	// Idempotent for concurrent executions which both verified before the
	// backend's idempotent create completed.
	if record.Status == "consumed" {
		return nil
	}
	if record.Status != "authorized" {
		return ErrConfirmationInvalid
	}
	record.Status = "consumed"
	s.records[id] = record
	return nil
}

func equalBinding(a, b ConfirmationBinding) bool {
	return a.TenantID == b.TenantID && a.OwnerCustomerID == b.OwnerCustomerID &&
		a.ConversationID == b.ConversationID && a.Tool == b.Tool &&
		a.ArgumentsHash == b.ArgumentsHash
}

func cloneConfirmationState(state ConfirmationState) ConfirmationState {
	state.Proposal.Facts = append([]ConfirmationFact(nil), state.Proposal.Facts...)
	state.Binding.ArgumentsJSON = append(json.RawMessage(nil), state.Binding.ArgumentsJSON...)
	return state
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	var out [36]byte
	hex.Encode(out[0:8], b[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], b[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], b[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], b[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], b[10:16])
	return string(out[:])
}
