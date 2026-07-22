package tenants

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/reinhlord/kontor/internal/identity"
)

// ErrLegacyBootstrapIneligible means the target tenant has already been
// configured, is partially configured, or does not exist. The bootstrap path
// must never infer how to repair any of those states.
var ErrLegacyBootstrapIneligible = errors.New("tenants: legacy bootstrap target is ineligible")

type legacyChannelState struct {
	widgetOrigin string
	enabled      bool
	ciphertext   []byte
	nonce        []byte
	secretDigest []byte
}

// BootstrapLegacyTenant atomically adopts a pristine Stage 1-5 tenant. An
// explicitly supplied legacy Telegram channel is encrypted and adopted in the
// same transaction as the first owner; partial or previously configured
// channels fail closed. A repeat succeeds without writes only when it exactly
// matches the prior bootstrap input.
func (s *Store) BootstrapLegacyTenant(ctx context.Context, input LegacyBootstrapInput) (LegacyBootstrapResult, error) {
	if s == nil || s.pool == nil || strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.TenantSlug) == "" {
		return LegacyBootstrapResult{}, ErrLegacyBootstrapIneligible
	}
	widgetOrigin, err := CanonicalWidgetOrigin(input.WidgetOrigin)
	if err != nil {
		return LegacyBootstrapResult{}, ErrLegacyBootstrapIneligible
	}
	email, err := identity.NormalizeEmail(input.Owner.Email)
	if err != nil {
		return LegacyBootstrapResult{}, ErrLegacyBootstrapIneligible
	}
	displayName := strings.TrimSpace(input.Owner.DisplayName)
	if displayName == "" || len(displayName) > 200 {
		return LegacyBootstrapResult{}, ErrLegacyBootstrapIneligible
	}
	ciphertext, nonce, digest, err := s.prepareChannelConfig(input.Telegram)
	if err != nil {
		return LegacyBootstrapResult{}, ErrLegacyBootstrapIneligible
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LegacyBootstrapResult{}, fmt.Errorf("tenants: begin legacy bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var channels legacyChannelState
	err = tx.QueryRow(ctx, `
		SELECT c.widget_origin,c.telegram_enabled,c.telegram_bot_token_ciphertext,
		       c.telegram_bot_token_nonce,c.telegram_webhook_secret_digest
		FROM tenants t
		JOIN tenant_channels c ON c.tenant_id=t.id
		WHERE t.id=$1 AND t.slug=$2
		FOR UPDATE OF t,c`, input.TenantID, strings.TrimSpace(input.TenantSlug),
	).Scan(&channels.widgetOrigin, &channels.enabled, &channels.ciphertext, &channels.nonce, &channels.secretDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return LegacyBootstrapResult{}, ErrLegacyBootstrapIneligible
	}
	if err != nil {
		return LegacyBootstrapResult{}, fmt.Errorf("tenants: lock legacy bootstrap target: %w", err)
	}

	var operatorCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM operators WHERE tenant_id=$1`, input.TenantID).Scan(&operatorCount); err != nil {
		return LegacyBootstrapResult{}, fmt.Errorf("tenants: count legacy operators: %w", err)
	}
	if operatorCount == 0 && legacyChannelsPristine(channels) {
		if _, err := identity.CreateOperatorTx(ctx, tx, identity.CreateOperatorInput{
			TenantID: input.TenantID, Email: email, DisplayName: displayName,
			Password: input.Owner.Password, Role: identity.RoleOwner,
		}); err != nil {
			return LegacyBootstrapResult{}, fmt.Errorf("tenants: create legacy bootstrap owner: %w", err)
		}
		tag, err := tx.Exec(ctx, `
			UPDATE tenant_channels
			SET widget_origin=$2,telegram_bot_token_ciphertext=$3,telegram_bot_token_nonce=$4,
			    telegram_webhook_secret_digest=$5,telegram_enabled=$6,updated_at=now()
			WHERE tenant_id=$1`, input.TenantID, widgetOrigin, ciphertext, nonce,
			nullableDigest(digest), input.Telegram.TelegramEnabled)
		if err != nil {
			return LegacyBootstrapResult{}, fmt.Errorf("tenants: set legacy channels: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return LegacyBootstrapResult{}, ErrLegacyBootstrapIneligible
		}
		if err := tx.Commit(ctx); err != nil {
			return LegacyBootstrapResult{}, fmt.Errorf("tenants: commit legacy bootstrap: %w", err)
		}
		return LegacyBootstrapResult{Applied: true}, nil
	}

	if operatorCount == 1 && s.legacyChannelsMatchInput(channels, widgetOrigin, input.Telegram, digest) {
		var storedEmail, storedName, storedRole, passwordHash string
		var active bool
		err := tx.QueryRow(ctx, `
			SELECT email,display_name,role,active,password_hash
			FROM operators WHERE tenant_id=$1`, input.TenantID,
		).Scan(&storedEmail, &storedName, &storedRole, &active, &passwordHash)
		if err != nil {
			return LegacyBootstrapResult{}, fmt.Errorf("tenants: read legacy bootstrap owner: %w", err)
		}
		if storedEmail == email && storedName == displayName && storedRole == identity.RoleOwner && active && identity.VerifyPassword(input.Owner.Password, passwordHash) {
			if err := tx.Commit(ctx); err != nil {
				return LegacyBootstrapResult{}, fmt.Errorf("tenants: commit legacy bootstrap replay: %w", err)
			}
			return LegacyBootstrapResult{}, nil
		}
	}
	return LegacyBootstrapResult{}, ErrLegacyBootstrapIneligible
}

func legacyChannelsPristine(channels legacyChannelState) bool {
	return channels.widgetOrigin == "" && !channels.enabled && len(channels.ciphertext) == 0 && len(channels.nonce) == 0 && len(channels.secretDigest) == 0
}

func (s *Store) legacyChannelsMatchInput(channels legacyChannelState, origin string, input ChannelConfig, digest [32]byte) bool {
	if channels.widgetOrigin != origin || channels.enabled != input.TelegramEnabled {
		return false
	}
	if !input.TelegramEnabled {
		return len(channels.ciphertext) == 0 && len(channels.nonce) == 0 && len(channels.secretDigest) == 0
	}
	if len(channels.ciphertext) == 0 || len(channels.nonce) == 0 || !bytes.Equal(channels.secretDigest, digest[:]) {
		return false
	}
	botToken, err := s.cipher.open(channels.ciphertext, channels.nonce)
	return err == nil && botToken == input.TelegramBotToken
}
