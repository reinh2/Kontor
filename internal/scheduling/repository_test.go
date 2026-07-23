package scheduling

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/platform/database"
)

func TestCreateBookingRequestHashIsCanonicalAndOwnerBound(t *testing.T) {
	t.Parallel()
	instant := time.Date(2026, time.July, 20, 9, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	base := CreateBookingRequest{
		CustomerID: "customer-a", ConversationID: "conversation", ServiceID: "service",
		StaffID: "staff", StartsAt: instant, Notes: "quiet", IdempotencyKey: "not-hashed-here-123",
	}
	first, err := hashCreateBooking(base)
	if err != nil {
		t.Fatal(err)
	}
	utcEquivalent := base
	utcEquivalent.StartsAt = instant.UTC()
	second, err := hashCreateBooking(utcEquivalent)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equivalent instants must hash identically: %q != %q", first, second)
	}
	otherOwner := base
	otherOwner.CustomerID = "customer-b"
	third, err := hashCreateBooking(otherOwner)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("customer identity must be part of the request hash")
	}
}

func TestValidateCreateBooking(t *testing.T) {
	t.Parallel()
	valid := CreateBookingRequest{
		CustomerID: "customer", ServiceID: "service", StaffID: "staff",
		StartsAt: time.Now().Add(time.Hour), IdempotencyKey: "abcdefghijklmnop",
	}
	if err := validateCreateBooking(valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	invalid := valid
	invalid.IdempotencyKey = "short"
	if err := validateCreateBooking(invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestValidateCreateBookingRejectsNonFutureStart(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.July, 22, 9, 0, 0, 0, time.UTC)
	base := CreateBookingRequest{
		CustomerID: "customer", ServiceID: "service", StaffID: "staff",
		IdempotencyKey: "abcdefghijklmnop",
	}
	for _, startsAt := range []time.Time{now.Add(-time.Hour), now} {
		request := base
		request.StartsAt = startsAt
		if err := validateCreateBookingAt(request, now); !errors.Is(err, ErrSlotUnavailable) {
			t.Fatalf("start %s: expected ErrSlotUnavailable, got %v", startsAt, err)
		}
	}
	request := base
	request.StartsAt = now.Add(time.Nanosecond)
	if err := validateCreateBookingAt(request, now); err != nil {
		t.Fatalf("future start rejected: %v", err)
	}
}

func TestTouchedLocalDatesIncludesBufferedCrossMidnightRange(t *testing.T) {
	t.Parallel()
	loc := berlin(t)
	start := time.Date(2026, time.October, 24, 23, 50, 0, 0, loc)
	end := time.Date(2026, time.October, 26, 0, 10, 0, 0, loc)
	got := touchedLocalDates(start, end, loc)
	want := []string{"2026-10-24", "2026-10-25", "2026-10-26"}
	if len(got) != len(want) {
		t.Fatalf("touched dates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("touched dates = %v, want %v", got, want)
		}
	}
}

func TestPGXRepositoryBookingIdempotencyAndTraceShape(t *testing.T) {
	pool, fixture := integrationFixture(t)
	repository := NewPGXRepository(pool, "")
	ctx := context.Background()

	services, err := repository.ListServices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0].ID != fixture.serviceID {
		t.Fatalf("unexpected services: %#v", services)
	}
	staff, err := repository.ListStaff(ctx, fixture.serviceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(staff) != 1 || staff[0].ID != fixture.staffID {
		t.Fatalf("unexpected staff: %#v", staff)
	}

	slots, err := repository.FindSlots(ctx, FindSlotsRequest{
		ServiceID: fixture.serviceID, StaffID: fixture.staffID,
		From: fixture.day.Add(8 * time.Hour), To: fixture.day.Add(13 * time.Hour), Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 5 || slots[0].Start.Format("15:04") != "09:00" {
		t.Fatalf("unexpected initial slots: %#v", slots)
	}

	request := CreateBookingRequest{
		CustomerID: fixture.customerA, ConversationID: fixture.conversationID,
		ServiceID: fixture.serviceID, StaffID: fixture.staffID, StartsAt: slots[0].Start,
		Notes: "window seat", IdempotencyKey: "booking-key-00000001",
	}
	created, err := repository.CreateBooking(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if created.Replayed || created.Booking.ID == "" {
		t.Fatalf("unexpected creation result: %#v", created)
	}
	var occupiedStart, occupiedEnd time.Time
	if err := pool.QueryRow(ctx, `
		SELECT lower(occupied_range), upper(occupied_range)
		FROM bookings WHERE tenant_id = $1 AND id = $2`, DefaultTenantID, created.Booking.ID).
		Scan(&occupiedStart, &occupiedEnd); err != nil {
		t.Fatal(err)
	}
	if !occupiedStart.Equal(created.Booking.StartsAt.Add(-10*time.Minute)) ||
		!occupiedEnd.Equal(created.Booking.EndsAt.Add(10*time.Minute)) {
		t.Fatalf("trigger-maintained occupied range = [%s,%s), want [%s,%s)",
			occupiedStart, occupiedEnd, created.Booking.StartsAt.Add(-10*time.Minute), created.Booking.EndsAt.Add(10*time.Minute))
	}
	replayed, err := repository.CreateBooking(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Booking.ID != created.Booking.ID {
		t.Fatalf("unexpected replay result: %#v", replayed)
	}

	conflict := request
	conflict.StartsAt = slots[1].Start
	if _, err := repository.CreateBooking(ctx, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	// The same model-generated key is safely reusable by another authenticated
	// owner because repository scope includes customer_id.
	otherOwner := request
	otherOwner.CustomerID = fixture.customerB
	otherOwner.ConversationID = ""
	otherOwner.StartsAt = slots[4].Start
	other, err := repository.CreateBooking(ctx, otherOwner)
	if err != nil {
		t.Fatalf("owner-scoped key should not collide: %v", err)
	}
	if other.Booking.ID == created.Booking.ID {
		t.Fatal("different owner unexpectedly replayed the first booking")
	}

	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM booking_events WHERE tenant_id = $1`, DefaultTenantID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("got %d booking events, want 2", eventCount)
	}

	assertNestedToolAttempts(t, pool, fixture.conversationID)
	assertConversationTokenHardCap(t, pool, fixture.conversationID)
}

func TestPGXRepositoryConcurrentBookingAllowsOneWinner(t *testing.T) {
	pool, fixture := integrationFixture(t)
	repository := NewPGXRepository(pool, DefaultTenantID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := fixture.day.Add(10 * time.Hour)
	requests := []CreateBookingRequest{
		{CustomerID: fixture.customerA, ConversationID: fixture.conversationID, ServiceID: fixture.serviceID, StaffID: fixture.staffID, StartsAt: start, IdempotencyKey: "concurrent-key-000001"},
		{CustomerID: fixture.customerB, ServiceID: fixture.serviceID, StaffID: fixture.staffID, StartsAt: start, IdempotencyKey: "concurrent-key-000002"},
	}
	startGate := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, request := range requests {
		request := request
		go func() {
			ready.Done()
			<-startGate
			_, err := repository.CreateBooking(ctx, request)
			results <- err
		}()
	}
	ready.Wait()
	close(startGate)

	var successes, unavailable int
	for range requests {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrSlotUnavailable):
			unavailable++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if successes != 1 || unavailable != 1 {
		t.Fatalf("got successes=%d unavailable=%d, want 1/1", successes, unavailable)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM bookings
		WHERE tenant_id = $1 AND staff_id = $2 AND starts_at = $3`,
		DefaultTenantID, fixture.staffID, start).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("database contains %d concurrent bookings, want 1", count)
	}
	var lockCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schedule_locks WHERE tenant_id = $1 AND staff_id = $2`, DefaultTenantID, fixture.staffID).Scan(&lockCount); err != nil {
		t.Fatal(err)
	}
	if lockCount != 1 {
		t.Fatalf("got %d schedule lock rows, want 1", lockCount)
	}
}

type testFixture struct {
	serviceID      string
	staffID        string
	customerA      string
	customerB      string
	conversationID string
	day            time.Time
}

func integrationFixture(t *testing.T) (*pgxpool.Pool, testFixture) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	schema := "kontor_test_" + hex.EncodeToString(random[:])
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+identifier); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+identifier+` CASCADE`)
		admin.Close()
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+identifier+` CASCADE`)
		admin.Close()
		t.Fatalf("connect test schema: %v", err)
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

	fixture := testFixture{
		serviceID: "11111111-1111-4111-8111-111111111111", staffID: "22222222-2222-4222-8222-222222222222",
		customerA: "33333333-3333-4333-8333-333333333333", customerB: "44444444-4444-4444-8444-444444444444",
		conversationID: "55555555-5555-4555-8555-555555555555",
	}
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	// Keep booking integration tests future-safe regardless of when they run;
	// the fixture rule below is Monday-only.
	base := time.Now().In(location).AddDate(0, 0, 2)
	daysUntilMonday := (int(time.Monday) - int(base.Weekday()) + 7) % 7
	base = base.AddDate(0, 0, daysUntilMonday)
	fixture.day = time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, location)
	_, err = pool.Exec(ctx, `
		INSERT INTO services
		    (tenant_id, id, slug, name, duration_minutes, buffer_before_minutes, buffer_after_minutes, price_minor, currency)
		VALUES ($1, $2, 'haircut', 'Haircut', 30, 10, 10, 4500, 'EUR');
		INSERT INTO staff (tenant_id, id, slug, display_name, timezone)
		VALUES ($1, $3, 'alex', 'Alex', 'Europe/Berlin');
		INSERT INTO staff_services (tenant_id, staff_id, service_id) VALUES ($1, $3, $2);
		INSERT INTO availability_rules (tenant_id, staff_id, rule_type, day_of_week, local_start, local_end)
		VALUES ($1, $3, 'working', 1, '09:00', '17:00'), ($1, $3, 'break', 1, '12:00', '13:00');
		INSERT INTO customers (tenant_id, id, display_name, phone)
		VALUES ($1, $4, 'Customer A', '+4915111111111'), ($1, $5, 'Customer B', '+4915222222222');
		INSERT INTO conversations (tenant_id, id, customer_id, channel, token_budget)
		VALUES ($1, $6, $4, 'demo', 100);`,
		DefaultTenantID, fixture.serviceID, fixture.staffID, fixture.customerA, fixture.customerB, fixture.conversationID)
	if err != nil {
		t.Fatalf("seed scheduling fixture: %v", err)
	}
	return pool, fixture
}

func assertNestedToolAttempts(t *testing.T, pool *pgxpool.Pool, conversationID string) {
	t.Helper()
	ctx := context.Background()
	var runID, iterationID, executionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO agent_runs (tenant_id, conversation_id, status, provider, model)
		VALUES ($1, $2, 'running', 'fake', 'deterministic') RETURNING id::text`,
		DefaultTenantID, conversationID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
			INSERT INTO agent_iterations (tenant_id, agent_run_id, iteration_no, status)
			VALUES ($1, $2, 1, 'succeeded') RETURNING id::text`, DefaultTenantID, runID).Scan(&iterationID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO tool_executions
		    (tenant_id, agent_run_id, agent_iteration_id, tool_call_id, call_index, call_count,
		     tool_name, contract_version, arguments_json, status)
		VALUES ($1, $2, $3, 'call-1', 1, 1, 'find_slots', '1.0.0', '{}', 'running')
		RETURNING id::text`, DefaultTenantID, runID, iterationID).Scan(&executionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tool_execution_attempts
		    (tenant_id, tool_execution_id, attempt_no, status, error_code, retryable)
		VALUES ($1, $2, 1, 'failed', 'TIMEOUT', true),
		       ($1, $2, 2, 'succeeded', NULL, false)`, DefaultTenantID, executionID); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `
		SELECT attempt_no FROM tool_execution_attempts
		WHERE tenant_id = $1 AND tool_execution_id = $2 ORDER BY attempt_no`, DefaultTenantID, executionID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var attempts []int
	for rows.Next() {
		var attempt int
		if err := rows.Scan(&attempt); err != nil {
			t.Fatal(err)
		}
		attempts = append(attempts, attempt)
	}
	if len(attempts) != 2 || attempts[0] != 1 || attempts[1] != 2 {
		t.Fatalf("unexpected nested attempts: %v", attempts)
	}
}

func assertConversationTokenHardCap(t *testing.T, pool *pgxpool.Pool, conversationID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		UPDATE conversations SET tokens_reserved = 101
		WHERE tenant_id = $1 AND id = $2`, DefaultTenantID, conversationID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("expected hard-cap check violation 23514, got %v", err)
	}
}
