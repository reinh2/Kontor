// Package conversations owns customers, conversation history, explicit
// confirmation recognition, and the database-backed conversation token cap.
package conversations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/platform/ids"
)

var (
	ErrNotFound       = errors.New("conversation not found")
	ErrBudgetExceeded = errors.New("conversation token budget exhausted")
	ErrNotConsent     = errors.New("message is not unambiguous consent")
	ErrUnauthorized   = errors.New("invalid conversation capability")
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Conversation struct {
	TenantID       string
	ID             string
	CustomerID     string
	Channel        string
	Status         string
	TokenBudget    int
	TokensUsed     int
	TokensReserved int
	// ConsecutiveClarificationFailures is the server-owned count of structured
	// clarification outcomes; the third consecutive one forces a hand-off.
	ConsecutiveClarificationFailures int
	// CapabilityToken is returned only by CreateDemo. The database stores its
	// SHA-256 digest and Get deliberately never hydrates this field.
	CapabilityToken string `json:"-"`
}

type Message struct {
	TenantID       string
	ID             string
	ConversationID string
	Role           string
	Content        string
	TokenCount     int
	CreatedAt      time.Time
}

type Profile struct {
	DisplayName string
	Email       string
	Phone       string
}

func (s *Store) CreateDemo(ctx context.Context, tenantID string, profile Profile, tokenBudget int) (Conversation, error) {
	if tenantID == "" || strings.TrimSpace(profile.DisplayName) == "" || tokenBudget < 1 {
		return Conversation{}, errors.New("invalid conversation input")
	}
	if profile.Email == "" && profile.Phone == "" {
		return Conversation{}, errors.New("email or phone is required")
	}
	capabilityToken, capabilityHash, err := newCapabilityToken()
	if err != nil {
		return Conversation{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Conversation{}, fmt.Errorf("begin conversation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	customerID := ids.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO customers(tenant_id,id,display_name,email,phone)
		VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''))`,
		tenantID, customerID, strings.TrimSpace(profile.DisplayName), strings.TrimSpace(profile.Email), strings.TrimSpace(profile.Phone),
	); err != nil {
		return Conversation{}, fmt.Errorf("insert customer: %w", err)
	}

	conversation := Conversation{
		TenantID: tenantID, ID: ids.New(), CustomerID: customerID,
		Channel: "demo", Status: "open", TokenBudget: tokenBudget,
		CapabilityToken: capabilityToken,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO conversations(tenant_id,id,customer_id,channel,channel_ref,status,token_budget)
		VALUES($1,$2,$3,'demo',$4,'open',$5)`,
		tenantID, conversation.ID, customerID, capabilityHash, tokenBudget,
	); err != nil {
		return Conversation{}, fmt.Errorf("insert conversation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Conversation{}, fmt.Errorf("commit conversation: %w", err)
	}
	return conversation, nil
}

// VerifyCapability authenticates a caller for one conversation. It hashes the
// presented opaque token and compares fixed-size digests in constant time. A
// token created for another conversation therefore cannot be used merely by
// substituting that conversation's UUID.
func (s *Store) VerifyCapability(ctx context.Context, tenantID, conversationID, rawToken string) error {
	presented := sha256.Sum256([]byte(rawToken))
	storedDigest := make([]byte, sha256.Size)

	var encodedDigest string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(channel_ref,'')
		FROM conversations
		WHERE tenant_id=$1 AND id=$2`, tenantID, conversationID).Scan(&encodedDigest)
	found := true
	if errors.Is(err, pgx.ErrNoRows) {
		found = false
	} else if err != nil {
		return fmt.Errorf("load conversation capability: %w", err)
	}
	if found {
		decoded, decodeErr := hex.DecodeString(encodedDigest)
		if decodeErr == nil && len(decoded) == sha256.Size {
			copy(storedDigest, decoded)
		}
	}

	matched := subtle.ConstantTimeCompare(presented[:], storedDigest) == 1
	if !found {
		return ErrNotFound
	}
	if rawToken == "" || !matched {
		return ErrUnauthorized
	}
	return nil
}

func newCapabilityToken() (raw, digest string, err error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", "", fmt.Errorf("generate conversation capability: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(secret[:])
	hashed := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(hashed[:]), nil
}

func (s *Store) Get(ctx context.Context, tenantID, conversationID string) (Conversation, error) {
	var item Conversation
	err := s.pool.QueryRow(ctx, `
		SELECT tenant_id,id,customer_id,channel,status,token_budget,tokens_used,tokens_reserved,
		       consecutive_clarification_failures
		FROM conversations WHERE tenant_id=$1 AND id=$2`, tenantID, conversationID,
	).Scan(&item.TenantID, &item.ID, &item.CustomerID, &item.Channel, &item.Status,
		&item.TokenBudget, &item.TokensUsed, &item.TokensReserved,
		&item.ConsecutiveClarificationFailures)
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	if err != nil {
		return Conversation{}, fmt.Errorf("get conversation: %w", err)
	}
	return item, nil
}

func (s *Store) AppendMessage(ctx context.Context, tenantID, conversationID, role, content, externalRef string) (Message, error) {
	return s.appendMessage(ctx, tenantID, conversationID, role, content, externalRef, nil)
}

// AppendMessageAt persists a server-observed receive time. The application
// captures this from PostgreSQL before waiting for the per-conversation turn
// lock so pipelined consent cannot appear to arrive after a proposal summary.
func (s *Store) AppendMessageAt(ctx context.Context, tenantID, conversationID, role, content, externalRef string, receivedAt time.Time) (Message, error) {
	if receivedAt.IsZero() {
		return Message{}, errors.New("message receive time is required")
	}
	return s.appendMessage(ctx, tenantID, conversationID, role, content, externalRef, &receivedAt)
}

func (s *Store) appendMessage(ctx context.Context, tenantID, conversationID, role, content, externalRef string, receivedAt *time.Time) (Message, error) {
	if role != "user" && role != "assistant" && role != "system" && role != "tool" {
		return Message{}, errors.New("invalid message role")
	}
	item := Message{
		TenantID: tenantID, ID: ids.New(), ConversationID: conversationID,
		Role: role, Content: content,
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO messages(tenant_id,id,conversation_id,role,content,external_ref,created_at)
		VALUES($1,$2,$3,$4,$5,NULLIF($6,''),COALESCE($7,clock_timestamp()))
		RETURNING created_at`,
		tenantID, item.ID, conversationID, role, content, externalRef, receivedAt,
	).Scan(&item.CreatedAt)
	if err != nil {
		return Message{}, fmt.Errorf("append message: %w", err)
	}
	return item, nil
}

func (s *Store) History(ctx context.Context, tenantID, conversationID string, limit int) ([]Message, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT tenant_id,id,conversation_id,role,content,token_count,created_at
		FROM (
			SELECT tenant_id,id,conversation_id,role,content,token_count,created_at
			FROM messages WHERE tenant_id=$1 AND conversation_id=$2
			ORDER BY created_at DESC,id DESC LIMIT $3
		) recent ORDER BY created_at,id`, tenantID, conversationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var result []Message
	for rows.Next() {
		var item Message
		if err := rows.Scan(&item.TenantID, &item.ID, &item.ConversationID, &item.Role,
			&item.Content, &item.TokenCount, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ReserveTokens atomically reserves a conservative upper bound before a model
// call. It is impossible for concurrent turns to reserve past the row's cap.
func (s *Store) ReserveTokens(ctx context.Context, tenantID, conversationID string, amount int) error {
	if amount < 1 {
		return errors.New("reservation must be positive")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversations
		SET tokens_reserved=tokens_reserved+$3, updated_at=now()
		WHERE tenant_id=$1 AND id=$2
		  AND tokens_used+tokens_reserved+$3 <= token_budget`, tenantID, conversationID, amount)
	if err != nil {
		return fmt.Errorf("reserve token budget: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBudgetExceeded
	}
	return nil
}

func (s *Store) SettleTokens(ctx context.Context, tenantID, conversationID string, reserved, actual int) error {
	if reserved < 1 || actual < 0 || actual > reserved {
		return errors.New("invalid token settlement")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversations
		SET tokens_reserved=tokens_reserved-$3,
		    tokens_used=tokens_used+$4,
		    updated_at=now()
		WHERE tenant_id=$1 AND id=$2
		  AND tokens_reserved >= $3
		  AND tokens_used+$4 <= token_budget`, tenantID, conversationID, reserved, actual)
	if err != nil {
		return fmt.Errorf("settle token budget: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBudgetExceeded
	}
	return nil
}

func (s *Store) ReleaseTokens(ctx context.Context, tenantID, conversationID string, reserved int) error {
	if reserved < 1 {
		return nil
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversations
		SET tokens_reserved=tokens_reserved-$3, updated_at=now()
		WHERE tenant_id=$1 AND id=$2 AND tokens_reserved >= $3`, tenantID, conversationID, reserved)
	if err != nil {
		return fmt.Errorf("release token budget: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IsExplicitConsent intentionally accepts only an unambiguous whole message.
// Prompt-like suffixes or changed instructions are not authorization.
func IsExplicitConsent(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = strings.TrimSuffix(normalized, ".")
	switch normalized {
	case "yes", "yes, confirm", "confirm", "confirm it", "book it", "yes, book it", "go ahead":
		return true
	default:
		return false
	}
}

// IsHumanRequest recognizes deliberately narrow, whole-message hand-off
// requests. Broader language remains available to the model's
// escalate_to_human tool, while common direct requests never depend on model
// compliance.
func IsHumanRequest(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = strings.Trim(normalized, " \t\r\n.!?")
	normalized = strings.Join(strings.Fields(normalized), " ")
	switch normalized {
	case "human", "human please", "a human", "a human please",
		"person", "person please", "a person", "a person please",
		"operator", "operator please", "live agent", "live agent please",
		"talk to a human", "speak to a human", "talk to a person", "speak to a person",
		"talk to an operator", "speak to an operator", "connect me to a human",
		"connect me to a person", "connect me to an operator", "i want a human",
		"i need a human", "i want a person", "i need a person":
		return true
	default:
		return false
	}
}
