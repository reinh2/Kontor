package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidCredentials = errors.New("identity: invalid credentials")
	ErrSessionInvalid     = errors.New("identity: invalid or expired session")
	ErrForbidden          = errors.New("identity: operator role is not authorized")
	ErrInvalidOperator    = errors.New("identity: invalid operator input")
	ErrOperatorExists     = errors.New("identity: operator already exists")
)

// Config controls the server-held session lifetime. Session tokens are opaque
// random bearer values; the lifetime is enforced from PostgreSQL's clock.
type Config struct {
	SessionTTL time.Duration
	Now        func() time.Time
}

type Store struct {
	pool       *pgxpool.Pool
	sessionTTL time.Duration
	now        func() time.Time
}

func NewStore(pool *pgxpool.Pool, config Config) (*Store, error) {
	if pool == nil {
		return nil, errors.New("identity: nil PostgreSQL pool")
	}
	if config.SessionTTL == 0 {
		config.SessionTTL = 12 * time.Hour
	}
	if config.SessionTTL < 5*time.Minute || config.SessionTTL > 30*24*time.Hour {
		return nil, errors.New("identity: session TTL must be between 5 minutes and 30 days")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Store{pool: pool, sessionTTL: config.SessionTTL, now: config.Now}, nil
}

// CreateOperator adds an owner or staff account to an existing tenant.
func (s *Store) CreateOperator(ctx context.Context, input CreateOperatorInput) (Operator, error) {
	if s == nil || s.pool == nil {
		return Operator{}, errors.New("identity: nil store")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Operator{}, fmt.Errorf("identity: begin create operator: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	operator, err := CreateOperatorTx(ctx, tx, input)
	if err != nil {
		return Operator{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operator{}, fmt.Errorf("identity: commit create operator: %w", err)
	}
	return operator, nil
}

// CreateOperatorTx is shared with transactional tenant provisioning so a
// tenant is never visible without its first owner account.
func CreateOperatorTx(ctx context.Context, tx pgx.Tx, input CreateOperatorInput) (Operator, error) {
	if tx == nil || strings.TrimSpace(input.TenantID) == "" || !validRole(input.Role) {
		return Operator{}, ErrInvalidOperator
	}
	email, err := NormalizeEmail(input.Email)
	if err != nil {
		return Operator{}, err
	}
	name := strings.TrimSpace(input.DisplayName)
	if name == "" || len(name) > 200 {
		return Operator{}, ErrInvalidOperator
	}
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return Operator{}, err
	}
	var operator Operator
	err = tx.QueryRow(ctx, `
		INSERT INTO operators(tenant_id,email,display_name,password_hash,role)
		VALUES($1,$2,$3,$4,$5)
		RETURNING tenant_id::text,id::text,email,display_name,role,active,created_at,updated_at`,
		input.TenantID, email, name, passwordHash, input.Role,
	).Scan(&operator.TenantID, &operator.ID, &operator.Email, &operator.DisplayName,
		&operator.Role, &operator.Active, &operator.CreatedAt, &operator.UpdatedAt)
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			return Operator{}, ErrOperatorExists
		}
		return Operator{}, fmt.Errorf("identity: insert operator: %w", err)
	}
	return operator, nil
}

// Authenticate verifies an operator inside the tenant selected by its slug and
// creates a new opaque session. Generic failures intentionally do not disclose
// whether the tenant, email, or password was wrong.
func (s *Store) Authenticate(ctx context.Context, tenantSlug, email, password string) (LoginResult, error) {
	if s == nil || s.pool == nil {
		return LoginResult{}, errors.New("identity: nil store")
	}
	email, err := NormalizeEmail(email)
	if err != nil || strings.TrimSpace(tenantSlug) == "" || len(password) > maximumPasswordLen {
		return LoginResult{}, ErrInvalidCredentials
	}
	var principal Principal
	var passwordHash string
	err = s.pool.QueryRow(ctx, `
		SELECT t.id::text,t.slug,t.name,t.timezone,t.currency,
		       o.id::text,o.email,o.display_name,o.role,o.password_hash
		FROM tenants t
		JOIN operators o ON o.tenant_id=t.id
		WHERE t.slug=$1 AND o.email=$2 AND o.active`,
		strings.TrimSpace(tenantSlug), email,
	).Scan(&principal.TenantID, &principal.TenantSlug, &principal.TenantName,
		&principal.Timezone, &principal.Currency, &principal.OperatorID, &principal.Email,
		&principal.DisplayName, &principal.Role, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		// Use a complete work-factor operation even when the account is missing,
		// reducing login-name enumeration through timing.
		_, _ = HashPassword(strings.Repeat("x", minimumPasswordLen))
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("identity: lookup credentials: %w", err)
	}
	if !VerifyPassword(password, passwordHash) {
		return LoginResult{}, ErrInvalidCredentials
	}
	token, err := newSessionToken()
	if err != nil {
		return LoginResult{}, err
	}
	principal, err = s.createSession(ctx, principal, token)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, Principal: principal}, nil
}

func (s *Store) createSession(ctx context.Context, principal Principal, token string) (Principal, error) {
	digest := tokenDigest(token)
	expiresAt := s.now().UTC().Add(s.sessionTTL)
	var sessionID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO operator_sessions(tenant_id,operator_id,token_digest,expires_at)
		VALUES($1,$2,$3,$4)
		RETURNING id::text`,
		principal.TenantID, principal.OperatorID, digest[:], expiresAt,
	).Scan(&sessionID)
	if err != nil {
		return Principal{}, fmt.Errorf("identity: create session: %w", err)
	}
	principal.SessionID = sessionID
	principal.ExpiresAt = expiresAt
	return principal, nil
}

// ValidateSession returns a principal only for a live session belonging to an
// active operator. The tenant is derived from the server-stored session, never
// from request headers, paths, or browser storage.
func (s *Store) ValidateSession(ctx context.Context, token string) (Principal, error) {
	if s == nil || s.pool == nil || token == "" {
		return Principal{}, ErrSessionInvalid
	}
	digest := tokenDigest(token)
	var principal Principal
	err := s.pool.QueryRow(ctx, `
		SELECT s.id::text,t.id::text,t.slug,t.name,t.timezone,t.currency,
		       o.id::text,o.email,o.display_name,o.role,s.expires_at
		FROM operator_sessions s
		JOIN operators o ON o.tenant_id=s.tenant_id AND o.id=s.operator_id
		JOIN tenants t ON t.id=s.tenant_id
		WHERE s.token_digest=$1 AND s.revoked_at IS NULL AND s.expires_at > clock_timestamp()
		  AND o.active`, digest[:],
	).Scan(&principal.SessionID, &principal.TenantID, &principal.TenantSlug,
		&principal.TenantName, &principal.Timezone, &principal.Currency,
		&principal.OperatorID, &principal.Email, &principal.DisplayName,
		&principal.Role, &principal.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrSessionInvalid
	}
	if err != nil {
		return Principal{}, fmt.Errorf("identity: validate session: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE operator_sessions SET last_used_at=clock_timestamp() WHERE id=$1`, principal.SessionID); err != nil {
		return Principal{}, fmt.Errorf("identity: update session activity: %w", err)
	}
	return principal, nil
}

// RevokeSession is idempotent and only accepts the bearer token being revoked;
// callers cannot supply a different session ID to terminate another operator.
func (s *Store) RevokeSession(ctx context.Context, token string) error {
	if s == nil || s.pool == nil || token == "" {
		return ErrSessionInvalid
	}
	digest := tokenDigest(token)
	tag, err := s.pool.Exec(ctx, `
		UPDATE operator_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp())
		WHERE token_digest=$1 AND expires_at > clock_timestamp()`, digest[:])
	if err != nil {
		return fmt.Errorf("identity: revoke session: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrSessionInvalid
	}
	return nil
}

// NormalizeEmail produces the database's canonical tenant-local login name.
func NormalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if len(email) < 3 || len(email) > 254 || strings.ContainsAny(email, " \t\r\n") {
		return "", ErrInvalidOperator
	}
	at := strings.LastIndexByte(email, '@')
	if at < 1 || at == len(email)-1 || !strings.Contains(email[at+1:], ".") {
		return "", ErrInvalidOperator
	}
	return email, nil
}

func newSessionToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("identity: generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func tokenDigest(token string) [sha256.Size]byte { return sha256.Sum256([]byte(token)) }
