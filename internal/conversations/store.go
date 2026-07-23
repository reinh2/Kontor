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
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/platform/ids"
)

var (
	emailInMessage = regexp.MustCompile(`(?i)\b[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+\b`)
	phoneInMessage = regexp.MustCompile(`\+[1-9][0-9]{7,14}\b`)
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

// EnsureChannelConversation returns the open conversation bound to one
// external channel identity (for Telegram, the chat id), creating the
// customer and conversation on first contact. Concurrent first messages for
// the same identity resolve to a single conversation through the unique
// (tenant, channel, channel_ref) index.
func (s *Store) EnsureChannelConversation(
	ctx context.Context,
	tenantID, channel, channelRef string,
	profile Profile,
	tokenBudget int,
) (Conversation, error) {
	if tenantID == "" || channel == "" || channelRef == "" || tokenBudget < 1 {
		return Conversation{}, errors.New("invalid channel conversation input")
	}
	if strings.TrimSpace(profile.DisplayName) == "" {
		return Conversation{}, errors.New("display name is required")
	}

	existing, err := s.channelConversation(ctx, tenantID, channel, channelRef)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Conversation{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Conversation{}, fmt.Errorf("begin channel conversation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	customerID := ids.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO customers(tenant_id,id,display_name,email,phone)
		VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''))`,
		tenantID, customerID, strings.TrimSpace(profile.DisplayName),
		strings.TrimSpace(profile.Email), strings.TrimSpace(profile.Phone),
	); err != nil {
		return Conversation{}, fmt.Errorf("insert channel customer: %w", err)
	}
	conversationID := ids.New()
	tag, err := tx.Exec(ctx, `
		INSERT INTO conversations(tenant_id,id,customer_id,channel,channel_ref,status,token_budget)
		VALUES($1,$2,$3,$4,$5,'open',$6)
		ON CONFLICT (tenant_id,channel,channel_ref) WHERE channel_ref IS NOT NULL DO NOTHING`,
		tenantID, conversationID, customerID, channel, channelRef, tokenBudget)
	if err != nil {
		return Conversation{}, fmt.Errorf("insert channel conversation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// A concurrent first message won the race; discard our customer row
		// with the rollback and adopt the winner.
		_ = tx.Rollback(ctx)
		return s.channelConversation(ctx, tenantID, channel, channelRef)
	}
	if err := tx.Commit(ctx); err != nil {
		return Conversation{}, fmt.Errorf("commit channel conversation: %w", err)
	}
	return s.channelConversation(ctx, tenantID, channel, channelRef)
}

// CaptureContactFromMessage persists only a contact string literally supplied
// in this authenticated customer's saved message. It never lets model output
// write customer identity, and it fills missing data rather than replacing an
// existing profile value.
func (s *Store) CaptureContactFromMessage(
	ctx context.Context, tenantID, customerID, message string,
) (Profile, error) {
	email := emailInMessage.FindString(message)
	phone := phoneInMessage.FindString(message)
	var profile Profile
	err := s.pool.QueryRow(ctx, `
		UPDATE customers
		SET email=COALESCE(NULLIF(email,''),NULLIF($3,'')),
		    phone=COALESCE(NULLIF(phone,''),NULLIF($4,''))
		WHERE tenant_id=$1 AND id=$2
		RETURNING display_name,COALESCE(email,''),COALESCE(phone,'')`,
		tenantID, customerID, email, phone).
		Scan(&profile.DisplayName, &profile.Email, &profile.Phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("capture customer contact: %w", err)
	}
	return profile, nil
}

func (s *Store) channelConversation(ctx context.Context, tenantID, channel, channelRef string) (Conversation, error) {
	var item Conversation
	err := s.pool.QueryRow(ctx, `
		SELECT tenant_id,id,customer_id,channel,status,token_budget,tokens_used,tokens_reserved,
		       consecutive_clarification_failures
		FROM conversations
		WHERE tenant_id=$1 AND channel=$2 AND channel_ref=$3`, tenantID, channel, channelRef,
	).Scan(&item.TenantID, &item.ID, &item.CustomerID, &item.Channel, &item.Status,
		&item.TokenBudget, &item.TokensUsed, &item.TokensReserved,
		&item.ConsecutiveClarificationFailures)
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	if err != nil {
		return Conversation{}, fmt.Errorf("get channel conversation: %w", err)
	}
	return item, nil
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

// RecalibrateInflatedUsage recalculates tokens_used for open conversations
// based on actual provider-reported usage from agent_iterations. This corrects
// inflated accounting caused by the pre-fix 1:1 byte-to-token estimator.
//
// For successful iterations, uses prompt_tokens + completion_tokens (real).
// For failed iterations (provider returned 0 usage), applies a flat penalty
// per iteration as a conservative stand-in. The penalty is deliberately low
// because the failed call did not consume real budget on the provider side.
//
// This is idempotent: running it when usage is already correct changes nothing
// meaningful (the recalculated sum equals or slightly differs from current).
// It only touches conversations with status='open' and tokens_reserved=0.
func (s *Store) RecalibrateInflatedUsage(ctx context.Context, tenantID string, failedIterationPenalty int) (int, error) {
	if failedIterationPenalty <= 0 {
		failedIterationPenalty = 500
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE conversations c
		SET tokens_used = recalc.real_usage, updated_at = now()
		FROM (
			SELECT r.conversation_id,
			       COALESCE(SUM(
			           CASE WHEN i.prompt_tokens + i.completion_tokens > 0
			                THEN i.prompt_tokens + i.completion_tokens
			                ELSE $2
			           END
			       ), 0)::integer AS real_usage
			FROM agent_runs r
			JOIN agent_iterations i ON i.tenant_id = r.tenant_id AND i.agent_run_id = r.id
			WHERE r.tenant_id = $1
			GROUP BY r.conversation_id
		) recalc
		WHERE c.tenant_id = $1
		  AND c.id = recalc.conversation_id
		  AND c.status = 'open'
		  AND c.tokens_reserved = 0
		  AND c.tokens_used > recalc.real_usage`,
		tenantID, failedIterationPenalty)
	if err != nil {
		return 0, fmt.Errorf("recalibrate inflated token usage: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
