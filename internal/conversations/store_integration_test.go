package conversations_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/platform/config"
	"github.com/reinhlord/kontor/internal/platform/database"
)

func TestStorePersistsOnlyCapabilityDigestAndScopesVerification(t *testing.T) {
	pool := conversationIntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store := conversations.NewStore(pool)

	first, err := store.CreateDemo(ctx, config.DefaultTenantID, conversations.Profile{
		DisplayName: "Capability One", Email: "one@example.com",
	}, 50000)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateDemo(ctx, config.DefaultTenantID, conversations.Profile{
		DisplayName: "Capability Two", Email: "two@example.com",
	}, 50000)
	if err != nil {
		t.Fatal(err)
	}
	if first.CapabilityToken == "" || second.CapabilityToken == "" || first.CapabilityToken == second.CapabilityToken {
		t.Fatal("creation did not return distinct opaque capabilities")
	}

	var persisted string
	if err := pool.QueryRow(ctx, `
		SELECT channel_ref FROM conversations WHERE tenant_id=$1 AND id=$2`,
		config.DefaultTenantID, first.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256([]byte(first.CapabilityToken))
	if persisted != hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("persisted capability is not the expected SHA-256 digest: %q", persisted)
	}
	if persisted == first.CapabilityToken {
		t.Fatal("database persisted the raw capability token")
	}

	reloaded, err := store.Get(ctx, config.DefaultTenantID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CapabilityToken != "" {
		t.Fatal("Get returned a capability token after its one-time creation response")
	}
	if err := store.VerifyCapability(ctx, config.DefaultTenantID, first.ID, first.CapabilityToken); err != nil {
		t.Fatalf("correct capability was rejected: %v", err)
	}
	if err := store.VerifyCapability(ctx, config.DefaultTenantID, first.ID, "wrong-token"); !errors.Is(err, conversations.ErrUnauthorized) {
		t.Fatalf("wrong capability error=%v, want ErrUnauthorized", err)
	}
	if err := store.VerifyCapability(ctx, config.DefaultTenantID, second.ID, first.CapabilityToken); !errors.Is(err, conversations.ErrUnauthorized) {
		t.Fatalf("cross-conversation capability error=%v, want ErrUnauthorized", err)
	}
	if err := store.VerifyCapability(ctx, config.DefaultTenantID, "00000000-0000-4000-8000-999999999999", first.CapabilityToken); !errors.Is(err, conversations.ErrNotFound) {
		t.Fatalf("missing conversation error=%v, want ErrNotFound", err)
	}
}

func TestStoreAllowsMissingContactAndCapturesLiteralContactFromMessage(t *testing.T) {
	pool := conversationIntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store := conversations.NewStore(pool)

	conversation, err := store.CreateDemo(ctx, config.DefaultTenantID, conversations.Profile{DisplayName: "Needs Contact"}, 50000)
	if err != nil {
		t.Fatalf("create without contact: %v", err)
	}
	profile, err := store.CaptureContactFromMessage(ctx, config.DefaultTenantID, conversation.CustomerID, "Please use me@example.com, thanks")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Email != "me@example.com" || profile.Phone != "" {
		t.Fatalf("captured profile=%#v", profile)
	}
	profile, err = store.CaptureContactFromMessage(ctx, config.DefaultTenantID, conversation.CustomerID, "My other address is ignored@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Email != "me@example.com" {
		t.Fatalf("literal contact overwrote existing profile: %#v", profile)
	}
}

func conversationIntegrationPool(t *testing.T) *pgxpool.Pool {
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
	schema := "kontor_conversation_test_" + hex.EncodeToString(random[:])
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
