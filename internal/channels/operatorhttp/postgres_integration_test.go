package operatorhttp

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
	"github.com/reinhlord/kontor/internal/agenttrace"
	"github.com/reinhlord/kontor/internal/platform/database"
)

const (
	operatorTestTenant      = "00000000-0000-4000-8000-000000000001"
	operatorOtherTenant     = "00000000-0000-4000-8000-000000000002"
	operatorCustomerAlice   = "51000000-0000-4000-8000-000000000001"
	operatorCustomerBob     = "51000000-0000-4000-8000-000000000002"
	operatorOtherCustomer   = "51000000-0000-4000-8000-000000000003"
	operatorConversationA   = "61000000-0000-4000-8000-000000000001"
	operatorConversationB   = "61000000-0000-4000-8000-000000000002"
	operatorOtherConv       = "61000000-0000-4000-8000-000000000003"
	operatorTriggerMessage  = "62000000-0000-4000-8000-000000000001"
	operatorReplyMessage    = "62000000-0000-4000-8000-000000000002"
	operatorRunNewest       = "71000000-0000-4000-8000-000000000001"
	operatorRunTiedLower    = "71000000-0000-4000-8000-000000000002"
	operatorRunTiedHigher   = "71000000-0000-4000-8000-000000000003"
	operatorOtherRun        = "71000000-0000-4000-8000-000000000004"
	operatorIteration       = "72000000-0000-4000-8000-000000000001"
	operatorToolExecution   = "73000000-0000-4000-8000-000000000001"
	operatorService         = "81000000-0000-4000-8000-000000000001"
	operatorOtherService    = "81000000-0000-4000-8000-000000000002"
	operatorStaffBooked     = "82000000-0000-4000-8000-000000000001"
	operatorStaffEmpty      = "82000000-0000-4000-8000-000000000002"
	operatorOtherStaff      = "82000000-0000-4000-8000-000000000003"
	operatorBooking         = "91000000-0000-4000-8000-000000000001"
	operatorOtherBooking    = "91000000-0000-4000-8000-000000000002"
	operatorEscalation      = "92000000-0000-4000-8000-000000000001"
	operatorOtherEscalation = "92000000-0000-4000-8000-000000000002"
	operatorScheduleBlock   = "93000000-0000-4000-8000-000000000001"
	operatorOtherBlock      = "93000000-0000-4000-8000-000000000002"
)

func TestPostgreSQLOperatorReadModel(t *testing.T) {
	pool := operatorIntegrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fixedNow := time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
	seedOperatorReadModel(t, ctx, pool, fixedNow)
	traceStore := agenttrace.NewStore(pool, operatorTestTenant)
	store, err := NewPostgreSQL(pool, traceStore, operatorTestTenant, "Europe/Berlin")
	if err != nil {
		t.Fatalf("construct operator postgres: %v", err)
	}
	store.now = func() time.Time { return fixedNow }

	t.Run("tenant scoped runs use stable keyset pagination", func(t *testing.T) {
		first, err := store.ListRuns(ctx, ListRunsRequest{Limit: 2})
		if err != nil {
			t.Fatalf("list first run page: %v", err)
		}
		if len(first.Items) != 2 {
			t.Fatalf("first page length = %d, want 2: %#v", len(first.Items), first)
		}
		// The second and third runs intentionally share a timestamp. UUID order is
		// the deterministic tiebreaker used by both ORDER BY and the cursor.
		if first.Items[0].ID != operatorRunNewest || first.Items[1].ID != operatorRunTiedHigher {
			t.Fatalf("first page ids = %q, %q", first.Items[0].ID, first.Items[1].ID)
		}
		if first.NextCursor == "" {
			t.Fatal("first page omitted next cursor")
		}

		second, err := store.ListRuns(ctx, ListRunsRequest{Limit: 2, Cursor: first.NextCursor})
		if err != nil {
			t.Fatalf("list second run page: %v", err)
		}
		if len(second.Items) != 1 || second.Items[0].ID != operatorRunTiedLower {
			t.Fatalf("second page = %#v", second)
		}
		if second.NextCursor != "" {
			t.Fatalf("last page next cursor = %q", second.NextCursor)
		}
		for _, page := range []RunPage{first, second} {
			for _, item := range page.Items {
				if item.ID == operatorOtherRun || item.CustomerName == "Other Tenant" {
					t.Fatalf("cross-tenant run leaked into page: %#v", item)
				}
			}
		}
	})

	t.Run("dashboard reports live tenant metrics and token totals", func(t *testing.T) {
		dashboard, err := store.Dashboard(ctx, DashboardRequest{Days: 7})
		if err != nil {
			t.Fatalf("load dashboard: %v", err)
		}
		if dashboard.GeneratedAt != fixedNow || dashboard.PeriodDays != 7 {
			t.Fatalf("dashboard window = %#v", dashboard)
		}
		if dashboard.KPIs.BookingsToday != 1 || dashboard.KPIs.TotalRuns != 3 ||
			dashboard.KPIs.TotalTokens != 365 || dashboard.KPIs.MedianLatencyMS != 200 ||
			dashboard.KPIs.OpenEscalations != 1 || dashboard.KPIs.SuccessRate != 0.5 {
			t.Fatalf("dashboard KPIs = %#v", dashboard.KPIs)
		}

		outcomes := make(map[string]int64, len(dashboard.RunOutcomes))
		for _, outcome := range dashboard.RunOutcomes {
			outcomes[outcome.Status] = outcome.Count
		}
		if len(outcomes) != 3 || outcomes["completed"] != 1 || outcomes["failed"] != 1 || outcomes["running"] != 1 {
			t.Fatalf("run outcomes = %#v", outcomes)
		}
		var seriesTotal int64
		for _, point := range dashboard.BookingSeries {
			seriesTotal += point.Count
			if point.Channel != "telegram" {
				t.Fatalf("unexpected or cross-tenant booking series point: %#v", point)
			}
		}
		if seriesTotal != 1 {
			t.Fatalf("booking series total = %d, want 1: %#v", seriesTotal, dashboard.BookingSeries)
		}
		if len(dashboard.RecentRuns) != 3 {
			t.Fatalf("recent runs = %#v", dashboard.RecentRuns)
		}
	})

	t.Run("run detail includes messages booking and nested trace", func(t *testing.T) {
		detail, err := store.GetRun(ctx, operatorRunNewest)
		if err != nil {
			t.Fatalf("get run detail: %v", err)
		}
		if detail.Run.ID != operatorRunNewest || detail.Run.TriggerMessageID != operatorTriggerMessage ||
			detail.Channel != "telegram" || detail.ConversationStatus != "open" ||
			detail.Customer.ID != operatorCustomerAlice || detail.Customer.DisplayName != "Alice Operator" {
			t.Fatalf("run context = %#v", detail)
		}
		if len(detail.Messages) != 2 || detail.Messages[0].ID != operatorTriggerMessage ||
			detail.Messages[0].Role != "user" || detail.Messages[1].ID != operatorReplyMessage ||
			detail.Messages[1].Role != "assistant" {
			t.Fatalf("run messages = %#v", detail.Messages)
		}
		if len(detail.Bookings) != 1 || detail.Bookings[0].ID != operatorBooking ||
			detail.Bookings[0].ScheduleVersion != 2 {
			t.Fatalf("run bookings = %#v", detail.Bookings)
		}
		if len(detail.Run.Iterations) != 1 || len(detail.Run.Tools) != 1 {
			t.Fatalf("nested trace = %#v", detail.Run)
		}
		tool := detail.Run.Tools[0]
		if tool.ID != operatorToolExecution || tool.ContractVersion != "1.0.0" ||
			tool.StartedAt == nil || tool.FinishedAt == nil || len(tool.Attempts) != 2 {
			t.Fatalf("tool trace = %#v", tool)
		}
		if tool.Attempts[0].AttemptNo != 1 || tool.Attempts[0].Status != "failed" ||
			tool.Attempts[0].ErrorCode != "DEPENDENCY_UNAVAILABLE" || !tool.Attempts[0].Retryable ||
			tool.Attempts[1].AttemptNo != 2 || tool.Attempts[1].Status != "succeeded" {
			t.Fatalf("tool attempts = %#v", tool.Attempts)
		}
	})

	t.Run("calendar uses overlap semantics and retains empty staff", func(t *testing.T) {
		location, err := time.LoadLocation("Europe/Berlin")
		if err != nil {
			t.Fatal(err)
		}
		from := time.Date(2026, time.July, 22, 0, 0, 0, 0, location)
		to := from.AddDate(0, 0, 1)
		calendar, err := store.Calendar(ctx, CalendarRequest{From: from, To: to})
		if err != nil {
			t.Fatalf("load calendar: %v", err)
		}
		if calendar.From != "2026-07-22" || calendar.To != "2026-07-23" || calendar.Timezone != "Europe/Berlin" {
			t.Fatalf("calendar bounds = %#v", calendar)
		}
		if len(calendar.Staff) != 2 || calendar.Staff[0].ID != operatorStaffBooked ||
			calendar.Staff[1].ID != operatorStaffEmpty {
			t.Fatalf("calendar staff = %#v", calendar.Staff)
		}
		if len(calendar.Services) != 1 || calendar.Services[0].ID != operatorService {
			t.Fatalf("calendar services = %#v", calendar.Services)
		}
		if len(calendar.Bookings) != 1 || calendar.Bookings[0].ID != operatorBooking ||
			calendar.Bookings[0].StaffID == operatorStaffEmpty {
			t.Fatalf("calendar bookings = %#v", calendar.Bookings)
		}
		if len(calendar.Blocks) != 1 || calendar.Blocks[0].ID != operatorScheduleBlock {
			t.Fatalf("calendar blocks = %#v", calendar.Blocks)
		}
	})

	t.Run("read methods do not mutate runtime tables", func(t *testing.T) {
		before := operatorWriteSnapshot(t, ctx, pool)
		if _, err := store.Dashboard(ctx, DashboardRequest{Days: 7}); err != nil {
			t.Fatal(err)
		}
		page, err := store.ListRuns(ctx, ListRunsRequest{Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ListRuns(ctx, ListRunsRequest{Limit: 2, Cursor: page.NextCursor}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.GetRun(ctx, operatorRunNewest); err != nil {
			t.Fatal(err)
		}
		location, err := time.LoadLocation("Europe/Berlin")
		if err != nil {
			t.Fatal(err)
		}
		from := time.Date(2026, time.July, 22, 0, 0, 0, 0, location)
		if _, err := store.Calendar(ctx, CalendarRequest{From: from, To: from.AddDate(0, 0, 1)}); err != nil {
			t.Fatal(err)
		}
		after := operatorWriteSnapshot(t, ctx, pool)
		if after != before {
			t.Fatalf("operator GET store mutated runtime tables\nbefore: %s\nafter:  %s", before, after)
		}
	})
}

func seedOperatorReadModel(t *testing.T, ctx context.Context, pool *pgxpool.Pool, now time.Time) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin operator fixture: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO tenants(id,slug,name,timezone)
		VALUES($1,'other-operator-tenant','Other Operator Tenant','Europe/Berlin')`, operatorOtherTenant); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO services
			(tenant_id,id,slug,name,duration_minutes,buffer_before_minutes,buffer_after_minutes,price_minor,currency)
		VALUES
			($1,$2,'operator-haircut','Operator Haircut',60,0,0,5000,'EUR'),
			($3,$4,'other-service','Other Service',60,0,0,9000,'EUR')`,
		operatorTestTenant, operatorService, operatorOtherTenant, operatorOtherService); err != nil {
		t.Fatalf("seed services: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO staff(tenant_id,id,slug,display_name,timezone)
		VALUES
			($1,$2,'alex-operator','Alex Operator','Europe/Berlin'),
			($1,$3,'bea-empty','Bea Empty','Europe/Berlin'),
			($4,$5,'other-staff','Other Staff','Europe/Berlin')`,
		operatorTestTenant, operatorStaffBooked, operatorStaffEmpty, operatorOtherTenant, operatorOtherStaff); err != nil {
		t.Fatalf("seed staff: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO customers(tenant_id,id,display_name,email)
		VALUES
			($1,$2,'Alice Operator','alice@example.com'),
			($1,$3,'Bob Operator','bob@example.com'),
			($4,$5,'Other Tenant','other@example.com')`,
		operatorTestTenant, operatorCustomerAlice, operatorCustomerBob, operatorOtherTenant, operatorOtherCustomer); err != nil {
		t.Fatalf("seed customers: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO conversations(tenant_id,id,customer_id,channel,status,token_budget)
		VALUES
			($1,$2,$3,'telegram','open',50000),
			($1,$4,$5,'demo','escalated',50000),
			($6,$7,$8,'demo','open',50000)`,
		operatorTestTenant, operatorConversationA, operatorCustomerAlice,
		operatorConversationB, operatorCustomerBob,
		operatorOtherTenant, operatorOtherConv, operatorOtherCustomer); err != nil {
		t.Fatalf("seed conversations: %v", err)
	}

	runNewestStarted := now.Add(-time.Hour)
	runTiedStarted := now.Add(-2 * time.Hour)
	if _, err := tx.Exec(ctx, `
		INSERT INTO messages(tenant_id,id,conversation_id,role,content,created_at)
		VALUES
			($1,$2,$3,'user','Please book the overlapping appointment',$4),
			($1,$5,$3,'assistant','The appointment is booked',$6)`,
		operatorTestTenant, operatorTriggerMessage, operatorConversationA, runNewestStarted.Add(-time.Second),
		operatorReplyMessage, runNewestStarted.Add(100*time.Millisecond)); err != nil {
		t.Fatalf("seed messages: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO agent_runs
			(tenant_id,id,conversation_id,trigger_message_id,status,provider,model,
			 prompt_tokens,completion_tokens,duration_ms,started_at,finished_at,error_code,error_message)
		VALUES
			($1,$2,$3,$4,'completed','fake','kontor/demo-v1',100,20,100,$5,$6,NULL,NULL),
			($1,$7,$8,NULL,'failed','openrouter','test/model',200,30,300,$9,$10,'provider_failure','sanitized failure'),
			($1,$11,$8,NULL,'running','openrouter','test/model',10,5,NULL,$9,NULL,NULL,NULL),
			($12,$13,$14,NULL,'completed','fake','other/model',900,99,50,$15,$16,NULL,NULL)`,
		operatorTestTenant, operatorRunNewest, operatorConversationA, operatorTriggerMessage,
		runNewestStarted, runNewestStarted.Add(100*time.Millisecond),
		operatorRunTiedLower, operatorConversationB, runTiedStarted, runTiedStarted.Add(300*time.Millisecond),
		operatorRunTiedHigher,
		operatorOtherTenant, operatorOtherRun, operatorOtherConv, now.Add(-30*time.Minute), now.Add(-30*time.Minute+50*time.Millisecond)); err != nil {
		t.Fatalf("seed runs: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO agent_iterations
			(tenant_id,id,agent_run_id,iteration_no,status,finish_reason,prompt_tokens,
			 completion_tokens,duration_ms,model_response,created_at)
		VALUES($1,$2,$3,1,'succeeded','tool_calls',100,20,80,
		       '{"returned_tool_call_count":1,"reserved_tokens":200,"charged_tokens":120}'::jsonb,$4)`,
		operatorTestTenant, operatorIteration, operatorRunNewest, runNewestStarted); err != nil {
		t.Fatalf("seed iteration: %v", err)
	}
	toolStarted := runNewestStarted.Add(20 * time.Millisecond)
	if _, err := tx.Exec(ctx, `
		INSERT INTO tool_executions
			(tenant_id,id,agent_run_id,agent_iteration_id,tool_call_id,call_index,call_count,
			 tool_name,contract_version,arguments_json,result_json,status,duration_ms,started_at,finished_at)
		VALUES($1,$2,$3,$4,'operator-call',1,1,'find_slots','1.0.0','{}'::jsonb,
		       '{"status":"success"}'::jsonb,'succeeded',60,$5,$6)`,
		operatorTestTenant, operatorToolExecution, operatorRunNewest, operatorIteration,
		toolStarted, toolStarted.Add(60*time.Millisecond)); err != nil {
		t.Fatalf("seed tool execution: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tool_execution_attempts
			(tenant_id,tool_execution_id,attempt_no,status,result_json,error_code,error_message,
			 retryable,provider_status,duration_ms,started_at,finished_at)
		VALUES
			($1,$2,1,'failed','{"status":"error"}'::jsonb,'DEPENDENCY_UNAVAILABLE','timeout',true,504,20,$3,$4),
			($1,$2,2,'succeeded','{"status":"success"}'::jsonb,NULL,NULL,false,200,30,$4,$5)`,
		operatorTestTenant, operatorToolExecution, toolStarted, toolStarted.Add(20*time.Millisecond),
		toolStarted.Add(50*time.Millisecond)); err != nil {
		t.Fatalf("seed tool attempts: %v", err)
	}

	bookingStart := now.Add(time.Hour)
	if _, err := tx.Exec(ctx, `
		INSERT INTO bookings
			(tenant_id,id,customer_id,conversation_id,service_id,staff_id,status,starts_at,ends_at,
			 buffer_before_minutes,buffer_after_minutes,schedule_version,notes,created_at)
		VALUES
			($1,$2,$3,$4,$5,$6,'confirmed',$7,$8,0,0,2,'operator fixture',$9),
			($10,$11,$12,$13,$14,$15,'confirmed',$7,$8,0,0,1,'other fixture',$9)`,
		operatorTestTenant, operatorBooking, operatorCustomerAlice, operatorConversationA,
		operatorService, operatorStaffBooked, bookingStart, bookingStart.Add(time.Hour), now.Add(-24*time.Hour),
		operatorOtherTenant, operatorOtherBooking, operatorOtherCustomer, operatorOtherConv,
		operatorOtherService, operatorOtherStaff); err != nil {
		t.Fatalf("seed bookings: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO escalations(tenant_id,id,conversation_id,customer_id,agent_run_id,reason_code,summary,status,created_at)
		VALUES
			($1,$2,$3,$4,$5,'provider_failure','Needs an operator','open',$6),
			($7,$8,$9,$10,$11,'other','Other tenant escalation','open',$6)`,
		operatorTestTenant, operatorEscalation, operatorConversationB, operatorCustomerBob,
		operatorRunTiedLower, runTiedStarted,
		operatorOtherTenant, operatorOtherEscalation, operatorOtherConv, operatorOtherCustomer, operatorOtherRun); err != nil {
		t.Fatalf("seed escalations: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO schedule_blocks(tenant_id,id,staff_id,starts_at,ends_at,kind,note)
		VALUES
			($1,$2,$3,$4,$5,'manual','Operator overlap'),
			($6,$7,$8,$4,$5,'manual','Other overlap')`,
		operatorTestTenant, operatorScheduleBlock, operatorStaffBooked,
		bookingStart.Add(15*time.Minute), bookingStart.Add(45*time.Minute),
		operatorOtherTenant, operatorOtherBlock, operatorOtherStaff); err != nil {
		t.Fatalf("seed schedule blocks: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit operator fixture: %v", err)
	}
}

func operatorWriteSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var snapshot []byte
	if err := pool.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'tenants', (SELECT count(*) FROM tenants),
			'services', (SELECT count(*) FROM services),
			'staff', (SELECT count(*) FROM staff),
			'staff_services', (SELECT count(*) FROM staff_services),
			'availability_rules', (SELECT count(*) FROM availability_rules),
			'schedule_blocks', (SELECT count(*) FROM schedule_blocks),
			'customers', (SELECT count(*) FROM customers),
			'conversations', (SELECT count(*) FROM conversations),
			'messages', (SELECT count(*) FROM messages),
			'bookings', (SELECT count(*) FROM bookings),
			'booking_events', (SELECT count(*) FROM booking_events),
			'schedule_locks', (SELECT count(*) FROM schedule_locks),
			'idempotency_records', (SELECT count(*) FROM idempotency_records),
			'agent_runs', (SELECT count(*) FROM agent_runs),
			'agent_iterations', (SELECT count(*) FROM agent_iterations),
			'tool_executions', (SELECT count(*) FROM tool_executions),
			'tool_execution_attempts', (SELECT count(*) FROM tool_execution_attempts),
			'action_proposals', (SELECT count(*) FROM action_proposals),
			'escalations', (SELECT count(*) FROM escalations),
			'dead_letter_events', (SELECT count(*) FROM dead_letter_events),
			'conversation_events', (SELECT count(*) FROM conversation_events),
			'telegram_updates', (SELECT count(*) FROM telegram_updates),
			'jobs', (SELECT count(*) FROM jobs),
			'dead_letter_jobs', (SELECT count(*) FROM dead_letter_jobs)
		)`).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot write tables: %v", err)
	}
	return string(snapshot)
}

func operatorIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	schema := "kontor_operator_test_" + hex.EncodeToString(random[:])
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
	if err := database.ApplyMigrations(ctx, pool, migrations.Files, "."); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}
