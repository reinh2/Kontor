package agenttrace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/agent"
	"github.com/reinhlord/kontor/internal/llm"
	"github.com/reinhlord/kontor/internal/platform/database"
)

const traceTestTenant = "00000000-0000-4000-8000-000000000001"

func TestPostgreSQLTracePersistsFailedModelIteration(t *testing.T) {
	pool := traceIntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const (
		customerID     = "11111111-1111-4111-8111-111111111111"
		conversationID = "22222222-2222-4222-8222-222222222222"
		messageID      = "33333333-3333-4333-8333-333333333333"
		runID          = "44444444-4444-4444-8444-444444444444"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO customers(tenant_id,id,display_name,email)
		VALUES($1,$2,'Trace Customer','trace@example.com');
		INSERT INTO conversations(tenant_id,id,customer_id,channel,token_budget)
		VALUES($1,$3,$2,'demo',50000);
		INSERT INTO messages(tenant_id,id,conversation_id,role,content)
		VALUES($1,$4,$3,'user','hello')`, traceTestTenant, customerID, conversationID, messageID); err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool, traceTestTenant)
	if err := store.StartRun(ctx, runID, conversationID, messageID, "openrouter", "test/model"); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-20 * time.Millisecond)
	if err := store.RecordModelCall(ctx, agent.ModelCallTrace{
		RunID: runID, ConversationID: conversationID, Iteration: 1,
		StartedAt: started, FinishedAt: time.Now(), Model: "test/model",
		Usage:          llm.Usage{InputTokens: 7, OutputTokens: 2, TotalTokens: 9},
		ReservedTokens: 100, ChargedTokens: 100, Status: agent.ModelCallFailed,
		ErrorMessage: "sanitized provider failure",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishRun(ctx, runID, "failed", "provider_failure", "sanitized provider failure", started); err != nil {
		t.Fatal(err)
	}
	trace, err := store.GetRun(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Status != "failed" || trace.ErrorCode != "provider_failure" || len(trace.Iterations) != 1 {
		t.Fatalf("trace = %#v", trace)
	}
	iteration := trace.Iterations[0]
	if iteration.Status != "failed" || iteration.ErrorMessage != "sanitized provider failure" ||
		iteration.ReservedTokens != 100 || iteration.ChargedTokens != 100 {
		t.Fatalf("iteration = %#v", iteration)
	}
}

func TestPostgreSQLTraceRejectsMissingParents(t *testing.T) {
	pool := traceIntegrationPool(t)
	store := NewStore(pool, traceTestTenant)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	now := time.Now()

	checks := []struct {
		name string
		err  error
	}{
		{name: "finish run", err: store.FinishRun(ctx, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "failed", "missing", "missing", now)},
		{name: "tool parent", err: store.RecordToolExecutionStarted(ctx, agent.ToolExecutionStartedTrace{
			RunID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Iteration: 1, CallID: "missing-call",
			ToolName: "list_services", ContractVersion: "1.0.0", Arguments: json.RawMessage(`{}`),
			CallIndex: 1, CallCount: 1, StartedAt: now,
		})},
		{name: "attempt", err: store.RecordToolAttempt(ctx, agent.ToolAttemptTrace{
			RunID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", CallID: "missing-call", AttemptNo: 1,
			Status: agent.ToolStatusFailed, Detail: json.RawMessage(`{}`), StartedAt: now, FinishedAt: now,
		})},
		{name: "completion", err: store.RecordToolExecutionCompleted(ctx, agent.ToolExecutionCompletedTrace{
			RunID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", CallID: "missing-call",
			Status: agent.ToolStatusFailed, Result: json.RawMessage(`{}`), StartedAt: now, FinishedAt: now,
		})},
	}
	for _, check := range checks {
		if check.err == nil || !strings.Contains(check.err.Error(), "not found") {
			t.Fatalf("%s error = %v, want not found", check.name, check.err)
		}
	}
}

func traceIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	schema := "kontor_trace_test_" + hex.EncodeToString(random[:])
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+identifier); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, `DROP SCHEMA `+identifier+` CASCADE`)
		admin.Close()
	})
	if err := database.ApplyMigrations(ctx, pool, migrations.Files, "."); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}
