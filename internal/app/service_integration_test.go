package app_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/agent"
	"github.com/reinhlord/kontor/internal/agentbudget"
	"github.com/reinhlord/kontor/internal/agenttools"
	"github.com/reinhlord/kontor/internal/agenttrace"
	appcore "github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/bootstrap"
	"github.com/reinhlord/kontor/internal/confirmations"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/demo"
	"github.com/reinhlord/kontor/internal/llm"
	"github.com/reinhlord/kontor/internal/platform/config"
	"github.com/reinhlord/kontor/internal/scheduling"
	"github.com/reinhlord/kontor/internal/tools"
)

func TestStage1ConversationCreatesBookingOnlyAfterConfirmation(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cfg := stage1TestConfig()
	if err := demo.EnsureFixedTenant(ctx, pool, demo.Tenant{
		ID: cfg.Tenant.ID, Slug: cfg.Tenant.Slug, Name: cfg.Tenant.Name,
		Timezone: cfg.Tenant.Timezone, Currency: cfg.Tenant.Currency,
	}); err != nil {
		t.Fatal(err)
	}
	if err := demo.SeedCatalog(ctx, pool, cfg.Tenant.ID, cfg.Tenant.Currency); err != nil {
		t.Fatal(err)
	}
	components, err := bootstrap.Build(ctx, cfg, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	conversation, err := components.Application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Demo Customer", Email: "demo@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Hold the same cross-process advisory lock used by the application and
	// prove the turn cannot persist or act until the prior turn is complete.
	lockConnection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lockMaterial := sha256.Sum256([]byte(cfg.Tenant.ID + "\x00" + conversation.ID))
	lockKey := int64(binary.BigEndian.Uint64(lockMaterial[:8]))
	if _, err := lockConnection.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		lockConnection.Release()
		t.Fatal(err)
	}
	type asyncTurn struct {
		result appcore.TurnResult
		err    error
	}
	turnDone := make(chan asyncTurn, 1)
	go func() {
		result, turnErr := components.Application.SendMessage(
			ctx, conversation.ID, "I'd like a haircut Thursday evening", "integration-message-0001",
		)
		turnDone <- asyncTurn{result: result, err: turnErr}
	}()
	var early *asyncTurn
	select {
	case completed := <-turnDone:
		early = &completed
	case <-time.After(30 * time.Millisecond):
	}
	messageCountWhileBlocked := countRows(t, pool, `SELECT count(*) FROM messages WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID)
	if _, err := lockConnection.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockKey); err != nil {
		lockConnection.Release()
		t.Fatal(err)
	}
	lockConnection.Release()
	if early != nil {
		t.Fatalf("conversation turn bypassed serialization lock: %#v", *early)
	}
	if messageCountWhileBlocked != 0 {
		t.Fatalf("blocked turn persisted %d messages before acquiring its conversation lock", messageCountWhileBlocked)
	}
	completed := <-turnDone
	proposalTurn, err := completed.result, completed.err
	if err != nil {
		t.Fatal(err)
	}
	proposalTrace, err := components.Trace.GetRun(ctx, proposalTurn.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if proposalTurn.PendingConfirmation == nil {
		var errorCode, errorMessage string
		var slotTokenLength int
		_ = pool.QueryRow(ctx, `
			SELECT COALESCE(result_json->'error'->>'code',''),
			       COALESCE(result_json->'error'->>'message',''),
			       COALESCE(length(arguments_json->>'slot_token'),0)
			FROM tool_executions
			WHERE tenant_id=$1 AND agent_run_id=$2 AND tool_name='create_booking'
			ORDER BY created_at DESC LIMIT 1`, cfg.Tenant.ID, proposalTurn.RunID).
			Scan(&errorCode, &errorMessage, &slotTokenLength)
		t.Fatalf("first turn did not return a confirmation proposal: message=%q error=%s/%s slot_token_length=%d", proposalTurn.Message, errorCode, errorMessage, slotTokenLength)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM bookings WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 0 {
		t.Fatalf("booking was written before explicit confirmation: count=%d", got)
	}

	if proposalTrace.Status != "completed" {
		t.Fatalf("proposal run status=%q, want completed", proposalTrace.Status)
	}
	assertMultiCallAndNestedAttempts(t, proposalTrace.Tools)

	confirmationTurn, err := components.Application.SendMessage(
		ctx, conversation.ID, "Yes, confirm", "integration-message-0002",
	)
	if err != nil {
		t.Fatal(err)
	}
	if confirmationTurn.PendingConfirmation != nil {
		t.Fatalf("consumed proposal remained pending: %#v", confirmationTurn.PendingConfirmation)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM bookings WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("confirmed flow wrote %d bookings, want exactly one", got)
	}

	confirmedTrace, err := components.Trace.GetRun(ctx, confirmationTurn.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var successfulCreate bool
	for _, tool := range confirmedTrace.Tools {
		if tool.Name == "create_booking" && tool.Status == "succeeded" && len(tool.Attempts) == 1 && tool.Attempts[0].AttemptNo == 1 {
			successfulCreate = true
		}
	}
	if !successfulCreate {
		t.Fatalf("confirmation trace has no successful create_booking with attempt 1: %#v", confirmedTrace.Tools)
	}

	var proposalStatus string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM action_proposals
		WHERE tenant_id=$1 AND id=$2`, cfg.Tenant.ID, proposalTurn.PendingConfirmation.ID).
		Scan(&proposalStatus); err != nil {
		t.Fatal(err)
	}
	if proposalStatus != "consumed" {
		t.Fatalf("proposal status=%q, want consumed", proposalStatus)
	}
	var used, reserved, budget int
	if err := pool.QueryRow(ctx, `
		SELECT tokens_used,tokens_reserved,token_budget FROM conversations
		WHERE tenant_id=$1 AND id=$2`, cfg.Tenant.ID, conversation.ID).
		Scan(&used, &reserved, &budget); err != nil {
		t.Fatal(err)
	}
	if used <= 0 || reserved != 0 || used > budget {
		t.Fatalf("invalid persisted token accounting: used=%d reserved=%d budget=%d", used, reserved, budget)
	}

	// Both turns must have committed durable stream events for SSE replay.
	events, err := components.Conversations.EventsAfter(ctx, cfg.Tenant.ID, conversation.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].ID >= events[1].ID {
		t.Fatalf("durable turn events=%#v, want two with ascending ids", events)
	}
	if !strings.Contains(string(events[0].Payload), `"pending_confirmation"`) {
		t.Fatalf("proposal event lacks the confirmation payload: %s", events[0].Payload)
	}
	if !strings.Contains(string(events[1].Payload), "booked") {
		t.Fatalf("confirmed event lacks the booked outcome: %s", events[1].Payload)
	}
	replayed, err := components.Conversations.EventsAfter(ctx, cfg.Tenant.ID, conversation.ID, events[0].ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != 1 || replayed[0].ID != events[1].ID {
		t.Fatalf("Last-Event-ID replay returned %#v, want only the second event", replayed)
	}
}

func TestStage1SameConversationLockContentionIsBoundedBeforeSave(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()
	if err := demo.EnsureFixedTenant(ctx, pool, demo.Tenant{
		ID: cfg.Tenant.ID, Slug: cfg.Tenant.Slug, Name: cfg.Tenant.Name,
		Timezone: cfg.Tenant.Timezone, Currency: cfg.Tenant.Currency,
	}); err != nil {
		t.Fatal(err)
	}
	if err := demo.SeedCatalog(ctx, pool, cfg.Tenant.ID, cfg.Tenant.Currency); err != nil {
		t.Fatal(err)
	}
	components, err := bootstrap.Build(ctx, cfg, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := components.Application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Contended Customer", Email: "contended@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	lockConnection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lockConnection.Release()
	lockMaterial := sha256.Sum256([]byte(cfg.Tenant.ID + "\x00" + conversation.ID))
	lockKey := int64(binary.BigEndian.Uint64(lockMaterial[:8]))
	if _, err := lockConnection.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = lockConnection.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey) }()

	startedAt := time.Now()
	_, err = components.Application.SendMessage(ctx, conversation.ID, "Please book a haircut", "contended-0001")
	if !errors.Is(err, appcore.ErrTurnOverloaded) {
		t.Fatalf("contended turn error=%v, want ErrTurnOverloaded", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("contended advisory lock waited %s, want bounded queue time", elapsed)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM messages WHERE tenant_id=$1 AND conversation_id=$2`,
		cfg.Tenant.ID, conversation.ID); got != 0 {
		t.Fatalf("contended turn persisted %d messages before admission failure", got)
	}
}

func TestStage1AuthorizedConfirmationCanBeRetriedAfterModelIgnoresIt(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()
	if err := demo.SeedCatalog(ctx, pool, cfg.Tenant.ID, cfg.Tenant.Currency); err != nil {
		t.Fatal(err)
	}

	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	backend := scheduling.NewToolBackend(scheduling.NewPGXRepository(pool, cfg.Tenant.ID))
	gateway, err := tools.NewGateway(tools.Config{
		Backend: backend, Confirmations: confirmationStore,
		SlotSigningKey: []byte(cfg.SlotTokenSecret),
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := agenttools.NewExecutor(pool, gateway, cfg.Tenant.ID, 3, 2*time.Second)
	demoModel, err := llm.NewDemoAdapter(llm.DemoConfig{
		CustomerName: "Retry Customer", CustomerEmail: "retry@example.com",
		Timezone: cfg.Tenant.Timezone,
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &ignoreAuthorizedOnceAdapter{delegate: demoModel}
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations: 8, TurnTimeout: 10 * time.Second,
		MaxOutputTokensPerCall: 800, ConversationTokenLimit: 50_000,
	}, agent.Dependencies{
		Model: model, ToolExecutor: executor, Trace: traceStore,
		Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
	}, stage1ToolDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	application, err := appcore.New(appcore.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: "fake", Model: "fake/ignore-once", TokenBudget: 50_000,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Retry Customer", Email: "retry@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	proposal, err := application.SendMessage(ctx, conversation.ID, "I'd like a haircut Thursday evening", "retry-proposal-0001")
	if err != nil {
		t.Fatal(err)
	}
	if proposal.PendingConfirmation == nil {
		t.Fatalf("proposal did not return confirmation: %#v", proposal)
	}

	ignored, err := application.SendMessage(ctx, conversation.ID, "Yes, confirm", "retry-ignored-0002")
	if err != nil {
		t.Fatal(err)
	}
	if !model.ignored || ignored.PendingConfirmation == nil || ignored.PendingConfirmation.ID != proposal.PendingConfirmation.ID {
		t.Fatalf("authorized proposal was not exposed for retry: ignored=%v result=%#v", model.ignored, ignored)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM bookings WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 0 {
		t.Fatalf("ignored authorized action wrote %d bookings", got)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM action_proposals WHERE tenant_id=$1 AND id=$2`, cfg.Tenant.ID, proposal.PendingConfirmation.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "confirmed" {
		t.Fatalf("ignored proposal status=%q, want confirmed", status)
	}

	retried, err := application.SendMessage(ctx, conversation.ID, "Yes, confirm", "retry-success-0003")
	if err != nil {
		t.Fatal(err)
	}
	if retried.Outcome != "completed" || retried.PendingConfirmation != nil {
		t.Fatalf("retried confirmation result=%#v", retried)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM bookings WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("retried confirmation wrote %d bookings, want exactly one", got)
	}
}

type ignoreAuthorizedOnceAdapter struct {
	delegate llm.Adapter
	ignored  bool
}

func (a *ignoreAuthorizedOnceAdapter) Complete(ctx context.Context, request llm.Request) (llm.Response, error) {
	if !a.ignored && len(request.Messages) > 0 {
		last := request.Messages[len(request.Messages)-1]
		if last.Role == llm.RoleSystem && strings.HasPrefix(last.Content, llm.AuthorizedActionPrefix) {
			a.ignored = true
			return llm.Response{
				Model: "fake/ignore-once", FinishReason: "tool_calls",
				Usage: llm.Usage{TotalTokens: 10},
				Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
					ID: "ignore-once-respond", Name: tools.ToolRespondToCustomer,
					Arguments: []byte(`{"disposition":"complete","message":"Please confirm once more."}`),
				}}},
			}, nil
		}
	}
	return a.delegate.Complete(ctx, request)
}

func TestStage1PipelinedConsentCannotAuthorizeUnseenProposal(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()
	if err := demo.SeedCatalog(ctx, pool, cfg.Tenant.ID, cfg.Tenant.Currency); err != nil {
		t.Fatal(err)
	}
	components, err := bootstrap.Build(ctx, cfg, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := components.Application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Pipelined Consent", Email: "pipeline@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	lockConnection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lockMaterial := sha256.Sum256([]byte(cfg.Tenant.ID + "\x00" + conversation.ID))
	lockKey := int64(binary.BigEndian.Uint64(lockMaterial[:8]))
	if _, err := lockConnection.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		lockConnection.Release()
		t.Fatal(err)
	}
	type asyncTurn struct {
		result appcore.TurnResult
		err    error
	}
	proposalDone := make(chan asyncTurn, 1)
	consentDone := make(chan asyncTurn, 1)
	go func() {
		result, turnErr := components.Application.SendMessage(
			ctx, conversation.ID, "I'd like a haircut Thursday evening", "pipeline-proposal-0001",
		)
		proposalDone <- asyncTurn{result: result, err: turnErr}
	}()
	time.Sleep(100 * time.Millisecond)
	go func() {
		result, turnErr := components.Application.SendMessage(
			ctx, conversation.ID, "Yes, confirm", "pipeline-consent-0002",
		)
		consentDone <- asyncTurn{result: result, err: turnErr}
	}()
	time.Sleep(100 * time.Millisecond)
	if _, err := lockConnection.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockKey); err != nil {
		lockConnection.Release()
		t.Fatal(err)
	}
	lockConnection.Release()

	proposal := <-proposalDone
	if proposal.err != nil || proposal.result.PendingConfirmation == nil {
		t.Fatalf("proposal turn=%#v err=%v", proposal.result, proposal.err)
	}
	pipelined := <-consentDone
	if pipelined.err != nil {
		t.Fatal(pipelined.err)
	}
	if pipelined.result.PendingConfirmation == nil {
		t.Fatalf("pipelined consent did not receive a safe replacement summary: %#v", pipelined.result)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM bookings WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 0 {
		t.Fatalf("pipelined unseen consent created %d bookings", got)
	}
	var proposalStatus string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM action_proposals
		WHERE tenant_id=$1 AND id=$2`, cfg.Tenant.ID, proposal.result.PendingConfirmation.ID).Scan(&proposalStatus); err != nil {
		t.Fatal(err)
	}
	if proposalStatus == "confirmed" || proposalStatus == "consumed" {
		t.Fatalf("pipelined consent authorized unseen proposal: status=%q", proposalStatus)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM action_proposals
		WHERE tenant_id=$1 AND conversation_id=$2 AND status='pending'`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("safe replacement pending proposals=%d, want 1", got)
	}

	confirmed, err := components.Application.SendMessage(
		ctx, conversation.ID, "Yes, confirm", "pipeline-visible-consent-0003",
	)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Outcome != "completed" {
		t.Fatalf("visible consent outcome=%q", confirmed.Outcome)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM bookings WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("visible consent created %d bookings, want 1", got)
	}
}

func TestStage1DirectHumanRequestHaltsFurtherAgentActions(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()
	components, err := bootstrap.Build(ctx, cfg, pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := components.Application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Human Request", Email: "human@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := components.Application.SendMessage(ctx, conversation.ID, "Human please!", "human-request-0001")
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != "escalated" || first.RunID == "" {
		t.Fatalf("direct hand-off=%#v", first)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM escalations
		WHERE tenant_id=$1 AND conversation_id=$2 AND reason_code='customer_request'`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("customer-request escalations=%d, want 1", got)
	}
	second, err := components.Application.SendMessage(ctx, conversation.ID, "Book anything tomorrow", "human-request-0002")
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome != "escalated" || second.RunID != "" {
		t.Fatalf("post-handoff turn ran automation: %#v", second)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM agent_runs WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("post-handoff agent runs=%d, want 1", got)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM bookings WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 0 {
		t.Fatalf("post-handoff bookings=%d", got)
	}
}

func TestStage1ThirdClarificationSignalsUnderstandingEscalation(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()
	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	backend := scheduling.NewToolBackend(scheduling.NewPGXRepository(pool, cfg.Tenant.ID))
	gateway, err := tools.NewGateway(tools.Config{
		Backend: backend, Confirmations: confirmationStore,
		SlotSigningKey: []byte(cfg.SlotTokenSecret),
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := agenttools.NewExecutor(pool, gateway, cfg.Tenant.ID, 3, 2*time.Second)
	clarificationStep := func(callID, message string) llm.FakeStep {
		return llm.FakeStep{Response: llm.Response{
			Model: "fake/clarify", FinishReason: "tool_calls", Usage: llm.Usage{TotalTokens: 10},
			Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: callID, Name: tools.ToolRespondToCustomer,
				Arguments: []byte(`{"disposition":"clarification_needed","message":"` + message + `"}`),
			}}},
		}}
	}
	model := llm.NewFakeAdapter(
		clarificationStep("clarify-1", "Could you clarify?"),
		clarificationStep("clarify-2", "Which service did you mean?"),
		clarificationStep("clarify-3", "I still do not understand the request."),
		llm.FakeStep{Response: llm.Response{Model: "fake/clarify", Usage: llm.Usage{TotalTokens: 10}, Message: llm.Message{Role: llm.RoleAssistant, Content: "This must not run."}}},
	)
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations: 4, TurnTimeout: 5 * time.Second,
		MaxOutputTokensPerCall: 200, ConversationTokenLimit: 50_000,
	}, agent.Dependencies{
		Model: model, ToolExecutor: executor, Trace: traceStore,
		Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
	}, stage1ToolDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	application, err := appcore.New(appcore.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: "fake", Model: "fake/clarify", TokenBudget: 50_000,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Clarification Test", Email: "clarify@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, message := range []string{"unclear one", "unclear two", "unclear three"} {
		turn, err := application.SendMessage(ctx, conversation.ID, message, fmt.Sprintf("clarification-%04d", index+1))
		if err != nil {
			t.Fatal(err)
		}
		if index < 2 && turn.Outcome != "completed" {
			t.Fatalf("clarification %d outcome=%q", index+1, turn.Outcome)
		}
		if index == 2 && turn.Outcome != "escalated" {
			t.Fatalf("third clarification outcome=%q", turn.Outcome)
		}
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM escalations
		WHERE tenant_id=$1 AND conversation_id=$2 AND reason_code='understanding_failed'`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("understanding escalations=%d, want 1", got)
	}
	blocked, err := application.SendMessage(ctx, conversation.ID, "a fourth message", "clarification-0004")
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Outcome != "escalated" || len(model.Requests()) != 3 {
		t.Fatalf("post-understanding handoff=%#v provider_calls=%d", blocked, len(model.Requests()))
	}
}

func TestStage1UnknownToolCallPromptsWithoutEscalation(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()

	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{
			Model: "fake/unknown-tool", FinishReason: "tool_calls", Usage: llm.Usage{TotalTokens: 10},
			Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: "unknown-call", Name: "list_available_slots", Arguments: []byte(`{}`),
			}}},
		}},
		terminalModelStep("ask-service", "Which service would you like?", tools.ResponseClarificationNeeded),
	)
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations: 4, TurnTimeout: 5 * time.Second,
		MaxOutputTokensPerCall: 200, ConversationTokenLimit: 50_000,
	}, agent.Dependencies{
		Model: model, Trace: traceStore,
		Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
	}, stage1ToolDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	application, err := appcore.New(appcore.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: "fake", Model: "fake/unknown-tool", TokenBudget: 50_000,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Unknown Tool Test", Email: "unknown-tool@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := application.SendMessage(ctx, conversation.ID, "25 July colour at 09:00", "unknown-tool-message-0001")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Outcome != "completed" || turn.Message != "Which service would you like?" {
		t.Fatalf("unknown tool turn=%#v", turn)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM escalations
		WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 0 {
		t.Fatalf("unknown tool escalations=%d, want 0", got)
	}
	trace, err := traceStore.GetRun(ctx, turn.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Status != "completed" || len(trace.Tools) != 2 || trace.Tools[0].Status != "failed" ||
		trace.Tools[1].Name != tools.ToolRespondToCustomer {
		t.Fatalf("unknown tool trace=%#v", trace)
	}
}

func TestStage1ExplicitEscalationToolPersistsHandOff(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()

	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	backend := scheduling.NewToolBackend(scheduling.NewPGXRepository(pool, cfg.Tenant.ID))
	gateway, err := tools.NewGateway(tools.Config{
		Backend: backend, Confirmations: confirmationStore,
		SlotSigningKey: []byte(cfg.SlotTokenSecret),
	})
	if err != nil {
		t.Fatal(err)
	}
	executor := agenttools.NewExecutor(pool, gateway, cfg.Tenant.ID, 3, 2*time.Second)
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{
			Model: "fake/escalation", FinishReason: "tool_calls", Usage: llm.Usage{TotalTokens: 10},
			Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: "explicit-escalation-call", Name: tools.ToolEscalate,
				Arguments: []byte(`{"reason":{"code":"customer_request","summary":"The customer asked to speak with a person."}}`),
			}}},
		}},
		llm.FakeStep{Response: llm.Response{
			Model: "fake/escalation", FinishReason: "stop", Usage: llm.Usage{TotalTokens: 10},
			Message: llm.Message{Role: llm.RoleAssistant, Content: "A person will follow up."},
		}},
	)
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations: 4, TurnTimeout: 5 * time.Second,
		MaxOutputTokensPerCall: 200, ConversationTokenLimit: 50_000,
	}, agent.Dependencies{
		Model: model, ToolExecutor: executor, Trace: traceStore,
		Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
	}, stage1ToolDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	application, err := appcore.New(appcore.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: "fake", Model: "fake/escalation", TokenBudget: 50_000,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Escalation Test", Email: "escalation@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := application.SendMessage(ctx, conversation.ID, "Please escalate this policy case", "escalation-message-0001")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Outcome != "escalated" {
		t.Fatalf("explicit escalation outcome=%q, want escalated", turn.Outcome)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM escalations
		WHERE tenant_id=$1 AND conversation_id=$2 AND agent_run_id=$3
		  AND source_tool_call_id='explicit-escalation-call' AND reason_code='customer_request'`,
		cfg.Tenant.ID, conversation.ID, turn.RunID); got != 1 {
		t.Fatalf("explicit tool escalations=%d, want 1", got)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM dead_letter_events
		WHERE tenant_id=$1 AND conversation_id=$2`, cfg.Tenant.ID, conversation.ID); got != 0 {
		t.Fatalf("successful hand-off wrote %d dead letters", got)
	}
	replay := gateway.Execute(ctx, tools.TrustedContext{
		TenantID: cfg.Tenant.ID, CustomerID: conversation.CustomerID,
		ConversationID: conversation.ID, AgentRunID: turn.RunID,
		InboundMessageID: "replay-message",
		Capabilities:     map[tools.Capability]bool{tools.CapabilityConversationEscalate: true},
	}, tools.Call{
		ID: "explicit-escalation-call", Name: tools.ToolEscalate,
		Arguments: []byte(`{"reason":{"code":"customer_request","summary":"The customer asked to speak with a person."}}`),
	})
	if replay.Status != tools.StatusSuccess || !replay.Meta.IdempotencyReplayed {
		t.Fatalf("explicit escalation replay=%#v", replay)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM escalations
		WHERE tenant_id=$1 AND conversation_id=$2 AND source_tool_call_id='explicit-escalation-call'`,
		cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("escalation replay wrote %d rows, want 1", got)
	}
	trace, err := traceStore.GetRun(ctx, turn.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Status != "escalated" || len(trace.Tools) != 1 || trace.Tools[0].Status != "succeeded" ||
		len(trace.Tools[0].Attempts) != 1 || trace.Tools[0].Attempts[0].AttemptNo != 1 {
		t.Fatalf("explicit escalation trace=%#v", trace)
	}
}

func TestStage1ProviderFailureIsSaveFirstAndDeadLettered(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()

	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	model := &saveFirstFailureAdapter{pool: pool, tenantID: cfg.Tenant.ID}
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations: 4, TurnTimeout: 5 * time.Second,
		MaxOutputTokensPerCall: 200, ConversationTokenLimit: 50_000,
	}, agent.Dependencies{
		Model: model, Trace: traceStore,
		Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
	}, stage1ToolDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	application, err := appcore.New(appcore.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: "fake", Model: "fake/failure", TokenBudget: 50_000,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Failure Test", Email: "failure@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	model.conversationID = conversation.ID
	turn, err := application.SendMessage(ctx, conversation.ID, "Please book something", "failure-message-0001")
	if err != nil {
		t.Fatal(err)
	}
	if !model.sawInbound {
		t.Fatal("provider was called before the inbound message was persisted")
	}
	if turn.Outcome != "failed" || turn.Message == "" {
		t.Fatalf("provider failure did not return a controlled fallback: %#v", turn)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM messages
		WHERE tenant_id=$1 AND conversation_id=$2 AND role IN ('user','assistant')`, cfg.Tenant.ID, conversation.ID); got != 2 {
		t.Fatalf("persisted failure-path messages=%d, want inbound plus fallback", got)
	}
	if got := countRows(t, pool, `
		SELECT count(*)
		FROM dead_letter_events d
		JOIN messages m ON m.tenant_id=d.tenant_id AND m.id=d.trigger_message_id
		WHERE d.tenant_id=$1 AND d.conversation_id=$2
		  AND d.reason_code='provider_failure' AND d.status='pending' AND m.role='user'`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("save-first dead-letter events=%d, want 1", got)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM escalations
		WHERE tenant_id=$1 AND conversation_id=$2 AND reason_code='provider_failure'`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("provider-failure escalations=%d, want 1", got)
	}
	trace, err := traceStore.GetRun(ctx, turn.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Status != "failed" || trace.ErrorCode != "provider_failure" ||
		len(trace.Iterations) != 1 || trace.Iterations[0].Status != "failed" ||
		trace.Iterations[0].ErrorMessage == "" {
		t.Fatalf("provider failure trace=%#v", trace)
	}
}

func TestStage1PostCommitFailureAcknowledgesBookingAndEscalates(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()

	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{
			Model: "fake/post-commit", FinishReason: "tool_calls", Usage: llm.Usage{TotalTokens: 10},
			Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: "committed-create", Name: "create_booking", Arguments: []byte(`{}`),
			}}},
		}},
		llm.FakeStep{Response: llm.Response{Model: "fake/post-commit"}, Err: errors.New("provider failed after commit")},
	)
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations: 4, TurnTimeout: 5 * time.Second,
		MaxOutputTokensPerCall: 200, ConversationTokenLimit: 50_000,
	}, agent.Dependencies{
		Model: model, ToolExecutor: committedBookingExecutor{}, Trace: traceStore,
		Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
	}, stage1ToolDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	application, err := appcore.New(appcore.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: "fake", Model: "fake/post-commit", TokenBudget: 50_000,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Post Commit", Email: "post-commit@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	turn, err := application.SendMessage(ctx, conversation.ID, "Complete the authorized booking", "post-commit-0001")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Outcome != "escalated" || !strings.Contains(turn.Message, "appointment is booked") {
		t.Fatalf("post-commit response did not acknowledge durable booking: %#v", turn)
	}
	if strings.Contains(turn.Message, "couldn’t complete that safely") {
		t.Fatalf("post-commit response used generic failure copy: %q", turn.Message)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM escalations
		WHERE tenant_id=$1 AND conversation_id=$2 AND reason_code='post_commit_failure'`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("post-commit escalations=%d, want 1", got)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM dead_letter_events
		WHERE tenant_id=$1 AND conversation_id=$2 AND reason_code='post_commit_failure' AND status='pending'`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("post-commit dead letters=%d, want 1", got)
	}
	var conversationStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM conversations WHERE tenant_id=$1 AND id=$2`, cfg.Tenant.ID, conversation.ID).Scan(&conversationStatus); err != nil {
		t.Fatal(err)
	}
	if conversationStatus != "escalated" {
		t.Fatalf("conversation status=%q, want escalated", conversationStatus)
	}
	trace, err := traceStore.GetRun(ctx, turn.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Status != "escalated" {
		t.Fatalf("post-commit trace=%#v", trace)
	}
	var errorCode string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(error_code,'') FROM agent_runs WHERE tenant_id=$1 AND id=$2`, cfg.Tenant.ID, turn.RunID).Scan(&errorCode); err != nil {
		t.Fatal(err)
	}
	if errorCode != "post_commit_failure" {
		t.Fatalf("post-commit run error_code=%q", errorCode)
	}
}

func TestStage1CommittedBookingOverridesMisleadingModelCopy(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()
	model := llm.NewFakeAdapter(
		llm.FakeStep{Response: llm.Response{
			Model: "fake/committed-copy", FinishReason: "tool_calls", Usage: llm.Usage{TotalTokens: 10},
			Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: "committed-copy-create", Name: "create_booking", Arguments: []byte(`{}`),
			}}},
		}},
		llm.FakeStep{Response: llm.Response{
			Model: "fake/committed-copy", FinishReason: "tool_calls", Usage: llm.Usage{TotalTokens: 10},
			Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
				ID: "committed-copy-respond", Name: tools.ToolRespondToCustomer,
				Arguments: []byte(`{"disposition":"complete","message":"The booking was not made."}`),
			}}},
		}},
	)
	application := newCommittedBookingTestApplication(t, pool, cfg, model)
	conversation, err := application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Committed Copy", Email: "committed-copy@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	turn, err := application.SendMessage(ctx, conversation.ID, "Book the authorized appointment", "committed-copy-0001")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Outcome != "completed" || turn.Message != "Your appointment is booked. The booking is confirmed." {
		t.Fatalf("committed booking did not use server-owned copy: %#v", turn)
	}
	if strings.Contains(strings.ToLower(turn.Message), "not made") {
		t.Fatalf("model contradiction escaped into committed response: %q", turn.Message)
	}
}

func TestStage1CommittedBookingThenSiblingRefusalAcknowledgesBookingAndHandoff(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()
	model := llm.NewFakeAdapter(llm.FakeStep{Response: llm.Response{
		Model: "fake/committed-handoff", FinishReason: "tool_calls", Usage: llm.Usage{TotalTokens: 10},
		Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "committed-handoff-create", Name: "create_booking", Arguments: []byte(`{}`)},
			{ID: "committed-handoff-refused", Name: "delete_another_customers_booking", Arguments: []byte(`{}`)},
		}},
	}})
	application := newCommittedBookingTestApplication(t, pool, cfg, model)
	conversation, err := application.CreateConversation(ctx, conversations.Profile{
		DisplayName: "Committed Handoff", Email: "committed-handoff@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	turn, err := application.SendMessage(ctx, conversation.ID, "Book then do something unsafe", "committed-handoff-0001")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Outcome != "escalated" || !strings.Contains(turn.Message, "appointment is booked") || !strings.Contains(turn.Message, "handed") {
		t.Fatalf("committed booking handoff copy=%#v", turn)
	}
	if got := countRows(t, pool, `
		SELECT count(*) FROM escalations
		WHERE tenant_id=$1 AND conversation_id=$2 AND reason_code='tool_refused'`, cfg.Tenant.ID, conversation.ID); got != 1 {
		t.Fatalf("committed sibling-refusal escalations=%d, want 1", got)
	}
}

func newCommittedBookingTestApplication(
	t *testing.T,
	pool *pgxpool.Pool,
	cfg config.Config,
	model llm.Adapter,
) *appcore.Service {
	t.Helper()
	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations: 4, TurnTimeout: 5 * time.Second,
		MaxOutputTokensPerCall: 200, ConversationTokenLimit: 50_000,
	}, agent.Dependencies{
		Model: model, ToolExecutor: committedBookingExecutor{}, Trace: traceStore,
		Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
	}, stage1ToolDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	application, err := appcore.New(appcore.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: "fake", Model: "fake/committed", TokenBudget: 50_000,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		t.Fatal(err)
	}
	return application
}

func TestStage1BookingWithoutContactPromptsAndCapturesLaterContact(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()
	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	model := llm.NewFakeAdapter(
		terminalModelStep("need-contact", "Please share an email or phone number to continue.", tools.ResponseClarificationNeeded),
		terminalModelStep("contact-saved", "Thanks, I have your contact details. Which service would you like?", tools.ResponseClarificationNeeded),
	)
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations: 4, TurnTimeout: 5 * time.Second, MaxOutputTokensPerCall: 200, ConversationTokenLimit: 50_000,
	}, agent.Dependencies{
		Model: model, Trace: traceStore, Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
	}, stage1ToolDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	application, err := appcore.New(appcore.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: "fake", Model: "fake/contact", TokenBudget: 50_000,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := application.CreateConversation(ctx, conversations.Profile{DisplayName: "No Contact"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := application.SendMessage(ctx, conversation.ID, "I want to book a haircut", "contact-request-0001")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Outcome != "completed" || !strings.Contains(turn.Message, "email or phone") {
		t.Fatalf("missing-contact turn=%#v", turn)
	}
	requests := model.Requests()
	if len(requests) != 1 || !requestContains(requests[0], "Authenticated customer contact on file: no.") ||
		!requestContains(requests[0], "do not call create_booking") {
		t.Fatalf("missing-contact prompt was not sent: %#v", requests)
	}
	if _, err := application.SendMessage(ctx, conversation.ID, "My email is no.contact@example.com", "contact-request-0002"); err != nil {
		t.Fatal(err)
	}
	requests = model.Requests()
	if len(requests) != 2 || !requestContains(requests[1], "Authenticated customer contact on file: yes.") {
		t.Fatalf("captured contact state was not sent on follow-up: %#v", requests)
	}
	var email string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(email,'') FROM customers WHERE tenant_id=$1 AND id=$2`, cfg.Tenant.ID, conversation.CustomerID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "no.contact@example.com" {
		t.Fatalf("customer email=%q", email)
	}
}

func TestStage1ClarificationTurnInvalidatesOldPendingConfirmationSnapshot(t *testing.T) {
	pool := stage1IntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := stage1TestConfig()
	conversationStore := conversations.NewStore(pool)
	traceStore := agenttrace.NewStore(pool, cfg.Tenant.ID)
	confirmationStore := confirmations.NewPostgreSQL(pool)
	model := llm.NewFakeAdapter(
		terminalModelStep("clarify-new-intent", "Which service would you like instead?", tools.ResponseClarificationNeeded),
	)
	runner, err := agent.NewRunner(agent.Config{
		MaxIterations: 4, TurnTimeout: 5 * time.Second, MaxOutputTokensPerCall: 200, ConversationTokenLimit: 50_000,
	}, agent.Dependencies{
		Model: model, Trace: traceStore, Budget: agentbudget.NewPostgreSQL(conversationStore, cfg.Tenant.ID),
	}, stage1ToolDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	application, err := appcore.New(appcore.Config{
		TenantID: cfg.Tenant.ID, TenantName: cfg.Tenant.Name, TenantTimezone: cfg.Tenant.Timezone,
		Provider: "fake", Model: "fake/stale-card", TokenBudget: 50_000,
	}, pool, conversationStore, runner, traceStore, confirmationStore)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := application.CreateConversation(ctx, conversations.Profile{DisplayName: "Changed Mind", Email: "changed@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	origin, err := conversationStore.AppendMessageAt(ctx, cfg.Tenant.ID, conversation.ID, "user", "Book the original time", "proposal-origin-0001", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	arguments := []byte(`{"slot_token":"slt_v1_aaaaaaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbb","idempotency_key":"stale-card-request-0001"}`)
	hash := sha256.Sum256(arguments)
	proposal, err := confirmationStore.Propose(ctx, tools.ConfirmationBinding{
		TenantID: cfg.Tenant.ID, OwnerCustomerID: conversation.CustomerID, ConversationID: conversation.ID,
		ProposedFromMessageID: origin.ID, Tool: tools.ToolCreateBooking, ArgumentsHash: hash, ArgumentsJSON: arguments,
	}, tools.ConfirmationProposal{
		Action: tools.ToolCreateBooking, Title: "Book original time", ExpiresAt: time.Now().Add(time.Hour),
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	turn, err := application.SendMessage(ctx, conversation.ID, "Actually, I need another day", "proposal-change-0002")
	if err != nil {
		t.Fatal(err)
	}
	if turn.PendingConfirmation != nil || turn.PendingConfirmationActive {
		t.Fatalf("clarification retained stale confirmation: %#v", turn)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM action_proposals WHERE tenant_id=$1 AND id=$2`, cfg.Tenant.ID, proposal.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Fatalf("stale proposal status=%q", status)
	}
	events, err := conversationStore.EventsAfter(ctx, cfg.Tenant.ID, conversation.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !strings.Contains(string(events[0].Payload), `"pending_confirmation_active":false`) {
		t.Fatalf("clarification event does not clear pending UI state: %#v", events)
	}
}

func requestContains(request llm.Request, value string) bool {
	for _, message := range request.Messages {
		if strings.Contains(message.Content, value) {
			return true
		}
	}
	return false
}

func terminalModelStep(id, message string, disposition tools.CustomerResponseDisposition) llm.FakeStep {
	arguments, _ := json.Marshal(tools.RespondToCustomerArguments{Disposition: disposition, Message: message})
	return llm.FakeStep{Response: llm.Response{
		Model: "fake", FinishReason: "tool_calls", Usage: llm.Usage{TotalTokens: 10},
		Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
			ID: id, Name: tools.ToolRespondToCustomer, Arguments: arguments,
		}}},
	}}
}

type committedBookingExecutor struct{}

func (committedBookingExecutor) Execute(_ context.Context, request agent.ToolRequest) (agent.ToolExecution, error) {
	if request.Call.Name != "create_booking" {
		return agent.ToolExecution{}, fmt.Errorf("unexpected tool %q", request.Call.Name)
	}
	return agent.ToolExecution{
		Content:             []byte(`{"status":"success","booking":{"id":"booking-is-durable"}}`),
		Status:              agent.ToolStatusSucceeded,
		SideEffectCommitted: true,
	}, nil
}

type saveFirstFailureAdapter struct {
	pool           *pgxpool.Pool
	tenantID       string
	conversationID string
	sawInbound     bool
}

func (a *saveFirstFailureAdapter) Complete(ctx context.Context, _ llm.Request) (llm.Response, error) {
	var inboundCount int
	if err := a.pool.QueryRow(ctx, `
		SELECT count(*) FROM messages
		WHERE tenant_id=$1 AND conversation_id=$2 AND role='user'`, a.tenantID, a.conversationID).
		Scan(&inboundCount); err != nil {
		return llm.Response{}, err
	}
	a.sawInbound = inboundCount == 1
	return llm.Response{Model: "fake/failure"}, errors.New("simulated provider outage")
}

func stage1ToolDefinitions() []llm.ToolDefinition {
	definitions := tools.Definitions()
	modelDefinitions := make([]llm.ToolDefinition, len(definitions))
	for i, definition := range definitions {
		modelDefinitions[i] = llm.ToolDefinition{
			Name: definition.Name, Version: definition.Version,
			Description: definition.Description, Parameters: definition.Parameters,
		}
	}
	return modelDefinitions
}

func assertMultiCallAndNestedAttempts(t *testing.T, traced []agenttrace.ToolTrace) {
	t.Helper()
	if len(traced) < 4 {
		t.Fatalf("proposal trace contains %d tool calls, want at least 4: %#v", len(traced), traced)
	}
	firstIteration := make(map[int]agenttrace.ToolTrace)
	confirmationRequired := false
	for _, tool := range traced {
		if tool.Name == tools.ToolRespondToCustomer {
			// The runner-local terminal control call never reaches the executor,
			// so it records no nested attempts by design.
			if len(tool.Attempts) != 0 {
				t.Fatalf("terminal control call recorded attempts: %#v", tool.Attempts)
			}
			continue
		}
		if len(tool.Attempts) != 1 || tool.Attempts[0].AttemptNo != 1 {
			t.Fatalf("tool %s does not expose one nested attempt numbered 1: %#v", tool.Name, tool.Attempts)
		}
		if tool.Iteration == 1 {
			firstIteration[tool.CallIndex] = tool
		}
		if tool.Name == "create_booking" && tool.Status == "confirmation_required" {
			confirmationRequired = true
		}
	}
	if len(firstIteration) != 2 || firstIteration[1].Name != "list_services" ||
		firstIteration[2].Name != "list_staff" || firstIteration[1].CallCount != 2 ||
		firstIteration[2].CallCount != 2 {
		t.Fatalf("first iteration did not persist both ordered calls: %#v", firstIteration)
	}
	if !confirmationRequired {
		t.Fatalf("proposal trace has no confirmation_required create_booking: %#v", traced)
	}
}

func stage1IntegrationPool(t *testing.T) *pgxpool.Pool {
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
	schema := "kontor_app_test_" + hex.EncodeToString(random[:])
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

	names, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		migration, err := migrations.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return pool
}

func stage1TestConfig() config.Config {
	return config.Config{
		Tenant: config.Tenant{
			ID: config.DefaultTenantID, Slug: "salon-nord", Name: "Salon Nord",
			Timezone: "Europe/Berlin", Currency: "EUR",
		},
		Agent: config.Agent{
			MaxIterations: 8, TurnTimeout: 10 * time.Second, ToolTimeout: 2 * time.Second,
			ToolMaxAttempts: 3, ConversationTokenBudget: 50_000, MaxOutputTokens: 800,
		},
		LLM:             config.LLM{Provider: "fake"},
		SlotTokenSecret: "integration-test-slot-secret-32-bytes-minimum",
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, arguments ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, arguments...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
