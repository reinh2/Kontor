package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestAdminBookingCommands exercises the operator (admin) booking lifecycle
// against a real database: an 'admin' audit actor, the optimistic
// schedule_version guard, and the transactional reminder-job lifecycle.
func TestAdminBookingCommands(t *testing.T) {
	pool, fixture := integrationFixture(t)
	repository := NewPGXRepository(pool, DefaultTenantID)
	ctx := context.Background()

	createStart := fixture.day.Add(10 * time.Hour)
	created, err := repository.AdminCreateBooking(ctx, AdminCreateBookingRequest{
		CustomerID:     fixture.customerA,
		ServiceID:      fixture.serviceID,
		StaffID:        fixture.staffID,
		StartsAt:       createStart,
		Notes:          "operator walk-in",
		IdempotencyKey: "admin-create-000000001",
	})
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}
	if created.Replayed || created.Booking.ID == "" || created.Booking.ScheduleVersion != 1 {
		t.Fatalf("unexpected admin create result: %#v", created)
	}
	bookingID := created.Booking.ID

	// The audit trail must record an 'admin' actor, not the customer 'agent'.
	assertLatestBookingEvent(t, pool, bookingID, "created", "admin", "operator")

	// A pending reminder must exist carrying the created start time.
	status, remindAt := reminderJob(t, pool, bookingID)
	if status != "pending" {
		t.Fatalf("reminder status after create = %q, want pending", status)
	}
	if !remindAt.Equal(createStart.UTC().Truncate(time.Second)) {
		t.Fatalf("reminder starts_at after create = %s, want %s", remindAt, createStart.UTC())
	}

	// An idempotent replay returns the same booking without a second effect.
	replay, err := repository.AdminCreateBooking(ctx, AdminCreateBookingRequest{
		CustomerID: fixture.customerA, ServiceID: fixture.serviceID, StaffID: fixture.staffID,
		StartsAt: createStart, Notes: "operator walk-in", IdempotencyKey: "admin-create-000000001",
	})
	if err != nil || !replay.Replayed || replay.Booking.ID != bookingID {
		t.Fatalf("admin create replay = %#v, err = %v", replay, err)
	}

	// A stale expected version is rejected before any mutation.
	rescheduleStart := fixture.day.Add(11 * time.Hour)
	if _, err := repository.AdminRescheduleBooking(ctx, AdminRescheduleBookingRequest{
		BookingID: bookingID, ExpectedVersion: 99, NewStartsAt: rescheduleStart,
		IdempotencyKey: "admin-reschedule-00001",
	}); !errors.Is(err, ErrScheduleVersionConflict) {
		t.Fatalf("stale reschedule error = %v, want ErrScheduleVersionConflict", err)
	}

	// The correct expected version reschedules, bumps the version, records an
	// 'admin' event, and moves the pending reminder.
	rescheduled, err := repository.AdminRescheduleBooking(ctx, AdminRescheduleBookingRequest{
		BookingID: bookingID, ExpectedVersion: 1, NewStartsAt: rescheduleStart,
		IdempotencyKey: "admin-reschedule-00001",
	})
	if err != nil {
		t.Fatalf("admin reschedule: %v", err)
	}
	if rescheduled.Booking.ScheduleVersion != 2 || !rescheduled.Booking.StartsAt.Equal(rescheduleStart) {
		t.Fatalf("unexpected reschedule result: %#v", rescheduled.Booking)
	}
	assertLatestBookingEvent(t, pool, bookingID, "rescheduled", "admin", "operator")
	status, remindAt = reminderJob(t, pool, bookingID)
	if status != "pending" || !remindAt.Equal(rescheduleStart.UTC().Truncate(time.Second)) {
		t.Fatalf("reminder after reschedule = %q at %s, want pending at %s", status, remindAt, rescheduleStart.UTC())
	}

	// A stale expected version is rejected for cancel too.
	if _, err := repository.AdminCancelBooking(ctx, AdminCancelBookingRequest{
		BookingID: bookingID, ExpectedVersion: 1, Reason: "stale",
		IdempotencyKey: "admin-cancel-000000001",
	}); !errors.Is(err, ErrScheduleVersionConflict) {
		t.Fatalf("stale cancel error = %v, want ErrScheduleVersionConflict", err)
	}

	// The correct version cancels, records an 'admin' event, and retires the
	// pending reminder so it never fires.
	cancelled, err := repository.AdminCancelBooking(ctx, AdminCancelBookingRequest{
		BookingID: bookingID, ExpectedVersion: 2, Reason: "customer cancelled",
		IdempotencyKey: "admin-cancel-000000001",
	})
	if err != nil {
		t.Fatalf("admin cancel: %v", err)
	}
	if cancelled.Booking.Status != "cancelled" || cancelled.Booking.ScheduleVersion != 3 {
		t.Fatalf("unexpected cancel result: %#v", cancelled.Booking)
	}
	assertLatestBookingEvent(t, pool, bookingID, "cancelled", "admin", "operator")
	if status, _ := reminderJob(t, pool, bookingID); status != "cancelled" {
		t.Fatalf("reminder status after cancel = %q, want cancelled", status)
	}

	// Cancelling again is idempotent.
	again, err := repository.AdminCancelBooking(ctx, AdminCancelBookingRequest{
		BookingID: bookingID, ExpectedVersion: 3, Reason: "customer cancelled",
		IdempotencyKey: "admin-cancel-000000002",
	})
	if err != nil || !again.Replayed || again.Booking.Status != "cancelled" {
		t.Fatalf("idempotent re-cancel = %#v, err = %v", again, err)
	}
}

// TestAdminCreateBookingRejectsOverlap proves the operator override still
// cannot double-book a staff member: the exclusion constraint holds.
func TestAdminCreateBookingRejectsOverlap(t *testing.T) {
	pool, fixture := integrationFixture(t)
	repository := NewPGXRepository(pool, DefaultTenantID)
	ctx := context.Background()

	start := fixture.day.Add(14 * time.Hour)
	if _, err := repository.AdminCreateBooking(ctx, AdminCreateBookingRequest{
		CustomerID: fixture.customerA, ServiceID: fixture.serviceID, StaffID: fixture.staffID,
		StartsAt: start, IdempotencyKey: "admin-overlap-000000001",
	}); err != nil {
		t.Fatalf("first admin create: %v", err)
	}
	// A second booking for the same staff overlapping the first (service is 30m
	// with 10m buffers, so 15 minutes later still collides) must be refused.
	_, err := repository.AdminCreateBooking(ctx, AdminCreateBookingRequest{
		CustomerID: fixture.customerB, ServiceID: fixture.serviceID, StaffID: fixture.staffID,
		StartsAt: start.Add(15 * time.Minute), IdempotencyKey: "admin-overlap-000000002",
	})
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Fatalf("overlapping admin create error = %v, want ErrSlotUnavailable", err)
	}
}

func assertLatestBookingEvent(t *testing.T, pool *pgxpool.Pool, bookingID, eventType, actorType, actorRef string) {
	t.Helper()
	var gotType, gotActorType, gotActorRef string
	if err := pool.QueryRow(context.Background(), `
		SELECT event_type, actor_type, COALESCE(actor_ref, '')
		FROM booking_events
		WHERE tenant_id = $1 AND booking_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, DefaultTenantID, bookingID).Scan(&gotType, &gotActorType, &gotActorRef); err != nil {
		t.Fatalf("load latest booking event: %v", err)
	}
	if gotType != eventType || gotActorType != actorType || gotActorRef != actorRef {
		t.Fatalf("latest booking event = (%q, %q, %q), want (%q, %q, %q)",
			gotType, gotActorType, gotActorRef, eventType, actorType, actorRef)
	}
}

// reminderJob returns the current status and the payload starts_at (truncated
// to whole seconds, in UTC) of a booking's send_reminder job.
func reminderJob(t *testing.T, pool *pgxpool.Pool, bookingID string) (string, time.Time) {
	t.Helper()
	var status, startsAt string
	if err := pool.QueryRow(context.Background(), `
		SELECT status, payload_json->>'starts_at'
		FROM jobs
		WHERE tenant_id = $1 AND booking_id = $2 AND job_type = 'send_reminder'`,
		DefaultTenantID, bookingID).Scan(&status, &startsAt); err != nil {
		t.Fatalf("load reminder job: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339, startsAt)
	if err != nil {
		t.Fatalf("parse reminder starts_at %q: %v", startsAt, err)
	}
	return status, parsed.UTC().Truncate(time.Second)
}
