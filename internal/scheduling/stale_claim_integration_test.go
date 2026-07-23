package scheduling

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/reinhlord/kontor/internal/jobqueue"
)

// TestRequeueStaleClaimsRecoversAbandonedReminder proves the transactional
// outbox survives a worker that dies between claiming a job and recording its
// terminal state. A booking's send_reminder job is forced into a stale 'claimed'
// state (as a crashed worker would leave it) and the reaper must return it to
// 'pending' so delivery is retried rather than silently lost. It lives in the
// scheduling package because that is where the integration fixture produces a
// real booking with a real outbox job.
func TestRequeueStaleClaimsRecoversAbandonedReminder(t *testing.T) {
	pool, fixture := integrationFixture(t)
	repository := NewPGXRepository(pool, DefaultTenantID)
	queue := jobqueue.New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	created, err := repository.CreateBooking(ctx, CreateBookingRequest{
		CustomerID: fixture.customerA, ConversationID: fixture.conversationID,
		ServiceID: fixture.serviceID, StaffID: fixture.staffID,
		StartsAt: fixture.day.Add(10 * time.Hour), IdempotencyKey: "stale-claim-reminder",
	})
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	bookingID := created.Booking.ID

	// Simulate a worker that claimed the reminder (attempts incremented) and then
	// died before Complete/Fail: the row is stuck 'claimed' with an old lease.
	if _, err := pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'claimed', claimed_at = now() - interval '10 minutes', attempts = 1
		WHERE tenant_id = $1 AND booking_id = $2 AND job_type = 'send_reminder'`,
		DefaultTenantID, bookingID); err != nil {
		t.Fatalf("strand reminder job: %v", err)
	}

	requeued, deadLettered, err := queue.RequeueStaleClaims(ctx, time.Minute)
	if err != nil {
		t.Fatalf("requeue stale claims: %v", err)
	}
	if requeued != 1 || deadLettered != 0 {
		t.Fatalf("reaper result = requeued %d / dead-lettered %d, want 1 / 0", requeued, deadLettered)
	}

	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM jobs
		WHERE tenant_id = $1 AND booking_id = $2 AND job_type = 'send_reminder'`,
		DefaultTenantID, bookingID).Scan(&status); err != nil {
		t.Fatalf("reload reminder job: %v", err)
	}
	if status != "pending" {
		t.Fatalf("reminder status after reaper = %q, want pending", status)
	}
}

// TestRequeueStaleClaimsDeadLettersExhausted proves a stale claim that has
// already used its last attempt is dead-lettered rather than retried forever, so
// a job that reliably crashes a worker cannot loop indefinitely.
func TestRequeueStaleClaimsDeadLettersExhausted(t *testing.T) {
	pool, fixture := integrationFixture(t)
	repository := NewPGXRepository(pool, DefaultTenantID)
	queue := jobqueue.New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	created, err := repository.CreateBooking(ctx, CreateBookingRequest{
		CustomerID: fixture.customerA, ConversationID: fixture.conversationID,
		ServiceID: fixture.serviceID, StaffID: fixture.staffID,
		StartsAt: fixture.day.Add(10 * time.Hour), IdempotencyKey: "stale-claim-exhausted",
	})
	if err != nil {
		t.Fatalf("create booking: %v", err)
	}
	bookingID := created.Booking.ID

	var jobID string
	if err := pool.QueryRow(ctx, `
		UPDATE jobs
		SET status = 'claimed', claimed_at = now() - interval '10 minutes',
		    attempts = max_attempts
		WHERE tenant_id = $1 AND booking_id = $2 AND job_type = 'send_reminder'
		RETURNING id::text`,
		DefaultTenantID, bookingID).Scan(&jobID); err != nil {
		t.Fatalf("strand exhausted reminder job: %v", err)
	}

	requeued, deadLettered, err := queue.RequeueStaleClaims(ctx, time.Minute)
	if err != nil {
		t.Fatalf("requeue stale claims: %v", err)
	}
	if requeued != 0 || deadLettered != 1 {
		t.Fatalf("reaper result = requeued %d / dead-lettered %d, want 0 / 1", requeued, deadLettered)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1`, jobID).Scan(&status); err != nil {
		t.Fatalf("reload reminder job: %v", err)
	}
	if status != "dead" {
		t.Fatalf("exhausted reminder status after reaper = %q, want dead", status)
	}

	var deadLetterCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM dead_letter_jobs
		WHERE tenant_id = $1 AND original_job_id = $2`,
		DefaultTenantID, jobID).Scan(&deadLetterCount); err != nil {
		t.Fatalf("count dead-letter rows: %v", err)
	}
	if deadLetterCount != 1 {
		t.Fatalf("dead-letter rows = %d, want 1", deadLetterCount)
	}
}
