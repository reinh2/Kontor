package tenants

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/identity"
	"github.com/reinhlord/kontor/internal/platform/database"
)

func TestBootstrapLegacyTenantAdoptsOnlyTargetAndSupportsExactRetry(t *testing.T) {
	pool := stage6BootstrapIntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	const northID = "00000000-0000-4000-8000-000000000101"
	const southID = "00000000-0000-4000-8000-000000000102"
	insertLegacyTenant(t, ctx, pool, northID, "north")
	insertLegacyTenant(t, ctx, pool, southID, "south")
	store, err := NewStore(pool, Config{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	input := LegacyBootstrapInput{
		TenantID: northID, TenantSlug: "north", WidgetOrigin: "https://north.example:0443",
		Owner: OwnerInput{Email: "owner@north.example", DisplayName: "North owner", Password: "correct-horse-battery-staple"},
	}

	result, err := store.BootstrapLegacyTenant(ctx, input)
	if err != nil || !result.Applied {
		t.Fatalf("BootstrapLegacyTenant = %+v, %v; want applied", result, err)
	}
	retry, err := store.BootstrapLegacyTenant(ctx, input)
	if err != nil || retry.Applied {
		t.Fatalf("exact retry = %+v, %v; want no-op", retry, err)
	}

	var northOrigin, southOrigin, passwordHash string
	if err := pool.QueryRow(ctx, `SELECT widget_origin FROM tenant_channels WHERE tenant_id=$1`, northID).Scan(&northOrigin); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT widget_origin FROM tenant_channels WHERE tenant_id=$1`, southID).Scan(&southOrigin); err != nil {
		t.Fatal(err)
	}
	if northOrigin != "https://north.example" || southOrigin != "" {
		t.Fatalf("origins north=%q south=%q", northOrigin, southOrigin)
	}
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM operators WHERE tenant_id=$1`, northID).Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if !identity.VerifyPassword(input.Owner.Password, passwordHash) {
		t.Fatal("stored legacy bootstrap owner password does not verify")
	}
	if countLegacyRows(t, ctx, pool, `SELECT count(*) FROM operators WHERE tenant_id=$1`, northID) != 1 ||
		countLegacyRows(t, ctx, pool, `SELECT count(*) FROM operators WHERE tenant_id=$1`, southID) != 0 {
		t.Fatal("bootstrap changed the wrong tenant's operators")
	}
}

func TestBootstrapLegacyTenantRejectsConfiguredTenantWithoutMutation(t *testing.T) {
	pool := stage6BootstrapIntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	const tenantID = "00000000-0000-4000-8000-000000000103"
	insertLegacyTenant(t, ctx, pool, tenantID, "configured")
	if _, err := pool.Exec(ctx, `UPDATE tenant_channels SET widget_origin='https://configured.example' WHERE tenant_id=$1`, tenantID); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(pool, Config{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	_, err = store.BootstrapLegacyTenant(ctx, LegacyBootstrapInput{
		TenantID: tenantID, TenantSlug: "configured", WidgetOrigin: "https://replacement.example",
		Owner: OwnerInput{Email: "owner@configured.example", DisplayName: "Configured owner", Password: "correct-horse-battery-staple"},
	})
	if !errors.Is(err, ErrLegacyBootstrapIneligible) {
		t.Fatalf("BootstrapLegacyTenant error = %v, want ErrLegacyBootstrapIneligible", err)
	}
	var origin string
	if err := pool.QueryRow(ctx, `SELECT widget_origin FROM tenant_channels WHERE tenant_id=$1`, tenantID).Scan(&origin); err != nil {
		t.Fatal(err)
	}
	if origin != "https://configured.example" || countLegacyRows(t, ctx, pool, `SELECT count(*) FROM operators WHERE tenant_id=$1`, tenantID) != 0 {
		t.Fatalf("configured tenant mutated: origin=%q", origin)
	}
}

func insertLegacyTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, slug string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO tenants(id,slug,name,timezone,currency)
		VALUES($1,$2,$3,'Europe/Berlin','EUR')`, tenantID, slug, slug+" salon"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tenant_channels(tenant_id) VALUES($1)`, tenantID); err != nil {
		t.Fatal(err)
	}
}

func countLegacyRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, arguments ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, query, arguments...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func stage6BootstrapIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	schema := "kontor_tenants_test_" + hex.EncodeToString(random[:])
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+identifier); err != nil {
		admin.Close()
		t.Fatalf("create integration schema: %v", err)
	}
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+identifier+` CASCADE`)
		admin.Close()
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+identifier+` CASCADE`)
		admin.Close()
		t.Fatalf("connect integration schema: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, `DROP SCHEMA `+identifier+` CASCADE`)
		admin.Close()
	})
	// Apply through the shared runner rather than executing the files
	// directly: it holds the migration advisory lock, so packages building
	// their private schemas in parallel cannot race each other inside
	// CREATE EXTENSION, which PostgreSQL does not make atomic even with
	// IF NOT EXISTS.
	if err := database.ApplyMigrations(ctx, pool, migrations.Files, "."); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

func TestBootstrapLegacyTenantAdoptsLegacyTelegramAndExactRetry(t *testing.T) {
	pool := stage6BootstrapIntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	const tenantID = "00000000-0000-4000-8000-000000000104"
	const botToken = "123456:legacy-bot-token"
	const webhookSecret = "legacy-webhook-secret"
	insertLegacyTenant(t, ctx, pool, tenantID, "telegram-north")
	store, err := NewStore(pool, Config{ChannelEncryptionKey: []byte("0123456789abcdef0123456789abcdef")})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	input := LegacyBootstrapInput{
		TenantID: tenantID, TenantSlug: "telegram-north", WidgetOrigin: "https://telegram-north.example",
		Owner:    OwnerInput{Email: "owner@telegram-north.example", DisplayName: "Telegram owner", Password: "correct-horse-battery-staple"},
		Telegram: ChannelConfig{TelegramEnabled: true, TelegramBotToken: botToken, TelegramWebhookSecret: webhookSecret},
	}
	result, err := store.BootstrapLegacyTenant(ctx, input)
	if err != nil || !result.Applied {
		t.Fatalf("BootstrapLegacyTenant = %+v, %v; want applied", result, err)
	}
	credentials, err := store.TelegramCredentials(ctx, tenantID)
	if err != nil {
		t.Fatalf("TelegramCredentials: %v", err)
	}
	if credentials.BotToken != botToken || credentials.WebhookDigest != webhookDigest(webhookSecret) {
		t.Fatalf("telegram credentials = %+v", credentials)
	}
	retry, err := store.BootstrapLegacyTenant(ctx, input)
	if err != nil || retry.Applied {
		t.Fatalf("exact retry = %+v, %v; want no-op", retry, err)
	}

	const partialTenantID = "00000000-0000-4000-8000-000000000105"
	insertLegacyTenant(t, ctx, pool, partialTenantID, "partial-telegram")
	_, err = store.BootstrapLegacyTenant(ctx, LegacyBootstrapInput{
		TenantID: partialTenantID, TenantSlug: "partial-telegram", WidgetOrigin: "https://partial.example",
		Owner:    OwnerInput{Email: "owner@partial.example", DisplayName: "Partial owner", Password: "correct-horse-battery-staple"},
		Telegram: ChannelConfig{TelegramEnabled: true, TelegramBotToken: botToken},
	})
	if !errors.Is(err, ErrLegacyBootstrapIneligible) {
		t.Fatalf("partial legacy Telegram bootstrap error = %v, want ErrLegacyBootstrapIneligible", err)
	}
	if countLegacyRows(t, ctx, pool, `SELECT count(*) FROM operators WHERE tenant_id=$1`, partialTenantID) != 0 {
		t.Fatal("partial legacy Telegram bootstrap created an operator")
	}
}

func TestProvisionNormalizesWhitespaceAroundTenantSlug(t *testing.T) {
	pool := stage6BootstrapIntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	store, err := NewStore(pool, Config{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	created, err := store.Provision(ctx, ProvisionInput{
		Slug: " north ", Name: "North", Timezone: "Europe/Berlin", Currency: "EUR",
		Owner:    OwnerInput{Email: "owner@north.example", DisplayName: "North owner", Password: "correct-horse-battery-staple"},
		Channels: ChannelConfig{WidgetOrigin: "https://north.example"},
		Services: []ServiceInput{{Slug: "cut", Name: "Cut", DurationMinutes: 30, Currency: "EUR"}},
		Staff: []StaffInput{{
			Slug: "ada", DisplayName: "Ada", Timezone: "Europe/Berlin", ServiceSlugs: []string{"cut"},
			Availability: []AvailabilityRuleInput{{RuleType: "working", DayOfWeek: 1, LocalStart: "09:00", LocalEnd: "17:00"}},
		}},
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if created.Slug != "north" {
		t.Fatalf("provisioned slug = %q, want north", created.Slug)
	}
}
