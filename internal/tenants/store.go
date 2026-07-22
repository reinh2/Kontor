package tenants

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/identity"
	"github.com/reinhlord/kontor/internal/platform/ids"
)

var (
	ErrNotFound        = errors.New("tenants: tenant not found")
	ErrConflict        = errors.New("tenants: tenant already exists")
	ErrInvalidInput    = errors.New("tenants: invalid provisioning input")
	ErrChannelDisabled = errors.New("tenants: requested channel is disabled")
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type Config struct {
	// ChannelEncryptionKey is a 32-byte application secret used exclusively for
	// tenant Telegram bot tokens. It is optional only while no Telegram config
	// is provisioned.
	ChannelEncryptionKey []byte
}

type Store struct {
	pool   *pgxpool.Pool
	cipher *secretCipher
}

func NewStore(pool *pgxpool.Pool, config Config) (*Store, error) {
	if pool == nil {
		return nil, errors.New("tenants: nil PostgreSQL pool")
	}
	store := &Store{pool: pool}
	if len(config.ChannelEncryptionKey) != 0 {
		cipher, err := newSecretCipher(config.ChannelEncryptionKey)
		if err != nil {
			return nil, err
		}
		store.cipher = cipher
	}
	return store, nil
}

// Provision creates a complete tenant configuration atomically. A caller
// cannot observe an ownerless tenant or one whose staff references a service
// from another business.
func (s *Store) Provision(ctx context.Context, input ProvisionInput) (Tenant, error) {
	if s == nil || s.pool == nil {
		return Tenant{}, errors.New("tenants: nil store")
	}
	input.Slug = strings.TrimSpace(input.Slug)
	widgetOrigin, err := CanonicalWidgetOrigin(input.Channels.WidgetOrigin)
	if err != nil {
		return Tenant{}, ErrInvalidInput
	}
	input.Channels.WidgetOrigin = widgetOrigin
	if err := validateProvision(input); err != nil {
		return Tenant{}, err
	}
	botCiphertext, botNonce, digest, err := s.prepareChannelConfig(input.Channels)
	if err != nil {
		return Tenant{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Tenant{}, fmt.Errorf("tenants: begin provisioning: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tenant := Tenant{
		ID: ids.New(), Slug: input.Slug, Name: strings.TrimSpace(input.Name),
		Timezone: input.Timezone, Currency: input.Currency, WidgetOrigin: input.Channels.WidgetOrigin,
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO tenants(id,slug,name,timezone,currency)
		VALUES($1,$2,$3,$4,$5)
		RETURNING created_at,updated_at`, tenant.ID, tenant.Slug, tenant.Name, tenant.Timezone, tenant.Currency,
	).Scan(&tenant.CreatedAt, &tenant.UpdatedAt)
	if err != nil {
		return Tenant{}, mapProvisionError(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tenant_channels(
			tenant_id,widget_origin,telegram_bot_token_ciphertext,telegram_bot_token_nonce,
			telegram_webhook_secret_digest,telegram_enabled
		) VALUES($1,$2,$3,$4,$5,$6)`,
		tenant.ID, input.Channels.WidgetOrigin, botCiphertext, botNonce, nullableDigest(digest), input.Channels.TelegramEnabled,
	); err != nil {
		return Tenant{}, fmt.Errorf("tenants: insert channels: %w", err)
	}
	if _, err := identity.CreateOperatorTx(ctx, tx, identity.CreateOperatorInput{
		TenantID: tenant.ID, Email: input.Owner.Email, DisplayName: input.Owner.DisplayName,
		Password: input.Owner.Password, Role: identity.RoleOwner,
	}); err != nil {
		return Tenant{}, fmt.Errorf("tenants: create owner: %w", err)
	}
	if err := insertCatalog(ctx, tx, tenant.ID, input.Services, input.Staff); err != nil {
		return Tenant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Tenant{}, fmt.Errorf("tenants: commit provisioning: %w", err)
	}
	return tenant, nil
}

func (s *Store) prepareChannelConfig(config ChannelConfig) ([]byte, []byte, [sha256.Size]byte, error) {
	if !config.TelegramEnabled {
		if config.TelegramBotToken != "" || config.TelegramWebhookSecret != "" {
			return nil, nil, [sha256.Size]byte{}, ErrInvalidInput
		}
		return nil, nil, [sha256.Size]byte{}, nil
	}
	if len(config.TelegramBotToken) == 0 || len(config.TelegramBotToken) > 512 ||
		len(config.TelegramWebhookSecret) < 16 || len(config.TelegramWebhookSecret) > 256 {
		return nil, nil, [sha256.Size]byte{}, ErrInvalidInput
	}
	ciphertext, nonce, err := s.cipher.seal(config.TelegramBotToken)
	if err != nil {
		return nil, nil, [sha256.Size]byte{}, err
	}
	return ciphertext, nonce, webhookDigest(config.TelegramWebhookSecret), nil
}

func nullableDigest(digest [sha256.Size]byte) any {
	if digest == [sha256.Size]byte{} {
		return nil
	}
	return digest[:]
}

func insertCatalog(ctx context.Context, tx pgx.Tx, tenantID string, services []ServiceInput, staff []StaffInput) error {
	serviceIDs := make(map[string]string, len(services))
	for _, service := range services {
		id := ids.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO services(
				tenant_id,id,slug,name,description,duration_minutes,buffer_before_minutes,
				buffer_after_minutes,price_minor,currency
			) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			tenantID, id, service.Slug, service.Name, service.Description, service.DurationMinutes,
			service.BufferBeforeMinutes, service.BufferAfterMinutes, service.PriceMinor, service.Currency,
		); err != nil {
			return fmt.Errorf("tenants: insert service %q: %w", service.Slug, err)
		}
		serviceIDs[service.Slug] = id
	}
	for _, member := range staff {
		staffID := ids.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO staff(tenant_id,id,slug,display_name,timezone)
			VALUES($1,$2,$3,$4,$5)`, tenantID, staffID, member.Slug, member.DisplayName, member.Timezone); err != nil {
			return fmt.Errorf("tenants: insert staff %q: %w", member.Slug, err)
		}
		for _, serviceSlug := range member.ServiceSlugs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO staff_services(tenant_id,staff_id,service_id)
				VALUES($1,$2,$3)`, tenantID, staffID, serviceIDs[serviceSlug]); err != nil {
				return fmt.Errorf("tenants: assign staff service: %w", err)
			}
		}
		for _, rule := range member.Availability {
			if _, err := tx.Exec(ctx, `
				INSERT INTO availability_rules(tenant_id,id,staff_id,rule_type,day_of_week,local_start,local_end)
				VALUES($1,$2,$3,$4,$5,$6::time,$7::time)`,
				tenantID, ids.New(), staffID, rule.RuleType, rule.DayOfWeek, rule.LocalStart, rule.LocalEnd); err != nil {
				return fmt.Errorf("tenants: insert availability rule: %w", err)
			}
		}
	}
	return nil
}

func validateProvision(input ProvisionInput) error {
	input.Slug = strings.TrimSpace(input.Slug)
	if !slugPattern.MatchString(input.Slug) || strings.TrimSpace(input.Name) == "" || len(input.Name) > 200 {
		return ErrInvalidInput
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return ErrInvalidInput
	}
	if len(input.Currency) != 3 || input.Currency != strings.ToUpper(input.Currency) {
		return ErrInvalidInput
	}
	if _, err := identity.NormalizeEmail(input.Owner.Email); err != nil || strings.TrimSpace(input.Owner.DisplayName) == "" || len(input.Owner.DisplayName) > 200 {
		return ErrInvalidInput
	}
	if len(input.Services) == 0 || len(input.Staff) == 0 || !validWidgetOrigin(input.Channels.WidgetOrigin) {
		return ErrInvalidInput
	}
	serviceSlugs := make(map[string]struct{}, len(input.Services))
	for _, service := range input.Services {
		if !slugPattern.MatchString(service.Slug) || strings.TrimSpace(service.Name) == "" || len(service.Name) > 200 ||
			len(service.Description) > 2000 || service.DurationMinutes < 5 || service.DurationMinutes > 1440 ||
			service.BufferBeforeMinutes < 0 || service.BufferBeforeMinutes > 240 ||
			service.BufferAfterMinutes < 0 || service.BufferAfterMinutes > 240 || service.PriceMinor < 0 ||
			len(service.Currency) != 3 || service.Currency != strings.ToUpper(service.Currency) {
			return ErrInvalidInput
		}
		if _, exists := serviceSlugs[service.Slug]; exists {
			return ErrInvalidInput
		}
		serviceSlugs[service.Slug] = struct{}{}
	}
	staffSlugs := make(map[string]struct{}, len(input.Staff))
	for _, member := range input.Staff {
		if !slugPattern.MatchString(member.Slug) || strings.TrimSpace(member.DisplayName) == "" || len(member.DisplayName) > 200 {
			return ErrInvalidInput
		}
		if _, err := time.LoadLocation(member.Timezone); err != nil {
			return ErrInvalidInput
		}
		if _, exists := staffSlugs[member.Slug]; exists || len(member.ServiceSlugs) == 0 || len(member.Availability) == 0 {
			return ErrInvalidInput
		}
		staffSlugs[member.Slug] = struct{}{}
		seenServices := map[string]struct{}{}
		for _, serviceSlug := range member.ServiceSlugs {
			if _, found := serviceSlugs[serviceSlug]; !found {
				return ErrInvalidInput
			}
			if _, duplicate := seenServices[serviceSlug]; duplicate {
				return ErrInvalidInput
			}
			seenServices[serviceSlug] = struct{}{}
		}
		for _, rule := range member.Availability {
			if (rule.RuleType != "working" && rule.RuleType != "break") || rule.DayOfWeek < 0 || rule.DayOfWeek > 6 || !validLocalTimeRange(rule.LocalStart, rule.LocalEnd) {
				return ErrInvalidInput
			}
		}
	}
	return nil
}

// CanonicalWidgetOrigin returns the exact scheme-and-host browser origin used
// for CORS. A tenant config cannot include credentials, a non-root path, a
// query, or a fragment because browsers never send those values in Origin.
func CanonicalWidgetOrigin(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil {
		return "", ErrInvalidInput
	}
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if parsed.Opaque != "" || parsed.User != nil ||
		(scheme != "http" && scheme != "https") || hostname == "" ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", ErrInvalidInput
	}
	if port != "" {
		portNumber, err := strconv.ParseUint(port, 10, 16)
		if err != nil || portNumber == 0 {
			return "", ErrInvalidInput
		}
		port = strconv.FormatUint(portNumber, 10)
	}
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	parsed.Scheme = scheme
	parsed.Host = canonicalOriginHost(hostname, port)
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String(), nil
}

func canonicalOriginHost(hostname, port string) string {
	if port != "" {
		return net.JoinHostPort(hostname, port)
	}
	if strings.Contains(hostname, ":") {
		return "[" + hostname + "]"
	}
	return hostname
}

func validWidgetOrigin(value string) bool {
	_, err := CanonicalWidgetOrigin(value)
	return err == nil
}

func validLocalTimeRange(start, end string) bool {
	from, fromErr := time.Parse("15:04", start)
	to, toErr := time.Parse("15:04", end)
	return fromErr == nil && toErr == nil && from.Before(to)
}

func (s *Store) TenantBySlug(ctx context.Context, slug string) (Tenant, error) {
	if s == nil || s.pool == nil || !slugPattern.MatchString(slug) {
		return Tenant{}, ErrNotFound
	}
	var tenant Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT t.id::text,t.slug,t.name,t.timezone,t.currency,
		       COALESCE(c.widget_origin,''),t.created_at,t.updated_at
		FROM tenants t
		LEFT JOIN tenant_channels c ON c.tenant_id=t.id
		WHERE t.slug=$1`, slug).Scan(
		&tenant.ID, &tenant.Slug, &tenant.Name, &tenant.Timezone, &tenant.Currency,
		&tenant.WidgetOrigin, &tenant.CreatedAt, &tenant.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("tenants: get tenant by slug: %w", err)
	}
	return tenant, nil
}

// TenantForHost resolves a public request from the first DNS label before the
// supplied platform suffix (for example salon-nord.kontor.example). This makes
// the host—not a browser-controlled query value—the public tenant boundary.
func (s *Store) TenantForHost(ctx context.Context, host, platformSuffix string) (Tenant, error) {
	slug, ok := slugFromHost(host, platformSuffix)
	if !ok {
		return Tenant{}, ErrNotFound
	}
	return s.TenantBySlug(ctx, slug)
}

func slugFromHost(host, platformSuffix string) (string, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	if value, _, err := net.SplitHostPort(host); err == nil {
		host = value
	}
	platformSuffix = strings.ToLower(strings.Trim(strings.TrimSpace(platformSuffix), "."))
	if platformSuffix == "" || !strings.HasSuffix(host, "."+platformSuffix) {
		return "", false
	}
	slug := strings.TrimSuffix(host, "."+platformSuffix)
	if strings.Contains(slug, ".") || !slugPattern.MatchString(slug) {
		return "", false
	}
	return slug, true
}

// TelegramCredentials retrieves only the server-needed bot token and webhook
// digest for a resolved tenant. It is never serialized to HTTP clients.
func (s *Store) TelegramCredentials(ctx context.Context, tenantID string) (TelegramCredentials, error) {
	if s == nil || s.pool == nil || tenantID == "" {
		return TelegramCredentials{}, ErrNotFound
	}
	var ciphertext, nonce, digest []byte
	var enabled bool
	err := s.pool.QueryRow(ctx, `
		SELECT telegram_bot_token_ciphertext,telegram_bot_token_nonce,
		       telegram_webhook_secret_digest,telegram_enabled
		FROM tenant_channels WHERE tenant_id=$1`, tenantID,
	).Scan(&ciphertext, &nonce, &digest, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return TelegramCredentials{}, ErrNotFound
	}
	if err != nil {
		return TelegramCredentials{}, fmt.Errorf("tenants: load Telegram configuration: %w", err)
	}
	if !enabled {
		return TelegramCredentials{}, ErrChannelDisabled
	}
	botToken, err := s.cipher.open(ciphertext, nonce)
	if err != nil {
		return TelegramCredentials{}, err
	}
	if len(digest) != sha256.Size {
		return TelegramCredentials{}, errors.New("tenants: stored Telegram secret digest is malformed")
	}
	var result TelegramCredentials
	result.BotToken = botToken
	copy(result.WebhookDigest[:], digest)
	return result, nil
}

func mapProvisionError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return fmt.Errorf("tenants: insert tenant: %w", err)
}

// StableServiceSlugs returns a copy useful for deterministic request tests and
// avoids callers depending on their input slice's backing array.
func StableServiceSlugs(values []ServiceInput) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Slug)
	}
	sort.Strings(result)
	return result
}
