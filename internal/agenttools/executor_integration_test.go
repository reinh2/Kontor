package agenttools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/agent"
	"github.com/reinhlord/kontor/internal/platform/database"
)

const executorTestTenant = "00000000-0000-4000-8000-000000000001"

func TestResolveTrustedContextLoadsPersistedCustomerProfile(t *testing.T) {
	pool := executorIntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const (
		customerID     = "22222222-2222-4222-8222-222222222222"
		conversationID = "33333333-3333-4333-8333-333333333333"
		messageID      = "44444444-4444-4444-8444-444444444444"
		runID          = "55555555-5555-4555-8555-555555555555"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO customers(tenant_id,id,display_name,email,phone)
		VALUES($1,$2,'Persisted Customer','persisted@example.com','+4915111111111');
		INSERT INTO conversations(tenant_id,id,customer_id,channel,token_budget)
		VALUES($1,$3,$2,'demo',50000);
		INSERT INTO messages(tenant_id,id,conversation_id,role,content)
		VALUES($1,$4,$3,'user','book an appointment');
		INSERT INTO agent_runs(tenant_id,id,conversation_id,trigger_message_id,status,provider,model)
		VALUES($1,$5,$3,$4,'running','fake','fake/model')`,
		executorTestTenant, customerID, conversationID, messageID, runID); err != nil {
		t.Fatal(err)
	}

	executor := NewExecutor(pool, nil, executorTestTenant, 1, time.Second)
	trusted, err := executor.resolveTrustedContext(ctx, agent.ToolRequest{
		RunID: runID, ConversationID: conversationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if trusted.TenantID != executorTestTenant || trusted.CustomerID != customerID ||
		trusted.CustomerDisplayName != "Persisted Customer" ||
		trusted.CustomerEmail != "persisted@example.com" ||
		trusted.CustomerPhone != "+4915111111111" ||
		trusted.ConversationID != conversationID || trusted.InboundMessageID != messageID ||
		trusted.AgentRunID != runID {
		t.Fatalf("resolved trusted context = %#v", trusted)
	}
}

func executorIntegrationPool(t *testing.T) *pgxpool.Pool {
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
	schema := "kontor_agenttools_test_" + hex.EncodeToString(random[:])
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
