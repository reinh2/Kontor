package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRescheduleBookingRejectsOverlap proves the customer reschedule path maps
// the PostgreSQL exclusion-constraint violation to ErrSlotUnavailable instead of
// leaking a raw, misleadingly retryable dependency error. Regression for the
// customer RescheduleBooking/CancelBooking paths previously lacking the
// serializable-retry loop and mapDatabaseError wrapper that CreateBooking and
// all Admin* paths use.
func TestRescheduleBookingRejectsOverlap(t *testing.T) {
	pool, fixture := integrationFixture(t)
	repository := NewPGXRepository(pool, DefaultTenantID)
	ctx := context.Background()

	firstStart := fixture.day.Add(10 * time.Hour)
	if _, err := repository.CreateBooking(ctx, CreateBookingRequest{
		CustomerID: fixture.customerA, ConversationID: fixture.conversationID,
		ServiceID: fixture.serviceID, StaffID: fixture.staffID,
		StartsAt: firstStart, IdempotencyKey: "reschedule-conflict-b1",
	}); err != nil {
		t.Fatalf("create first booking: %v", err)
	}

	secondStart := fixture.day.Add(14 * time.Hour)
	second, err := repository.CreateBooking(ctx, CreateBookingRequest{
		CustomerID: fixture.customerB, ServiceID: fixture.serviceID, StaffID: fixture.staffID,
		StartsAt: secondStart, IdempotencyKey: "reschedule-conflict-b2",
	})
	if err != nil {
		t.Fatalf("create second booking: %v", err)
	}

	// Rescheduling the second booking onto the first booking's slot collides on
	// the exclusion constraint. The fix must surface that as ErrSlotUnavailable.
	_, err = repository.RescheduleBooking(ctx, RescheduleBookingRequest{
		BookingID:       second.Booking.ID,
		OwnerCustomerID: fixture.customerB,
		NewStartsAt:     firstStart,
		NewEndsAt:       firstStart.Add(30 * time.Minute),
		Timezone:        "Europe/Berlin",
		IdempotencyKey:  "reschedule-conflict-move",
	})
	if !errors.Is(err, ErrSlotUnavailable) {
		t.Fatalf("overlapping customer reschedule error = %v, want ErrSlotUnavailable", err)
	}

	// The rejected move must not have been applied.
	var startsAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT starts_at FROM bookings WHERE tenant_id = $1 AND id = $2`,
		DefaultTenantID, second.Booking.ID).Scan(&startsAt); err != nil {
		t.Fatalf("reload second booking: %v", err)
	}
	if !startsAt.Equal(secondStart) {
		t.Fatalf("second booking moved despite rejected reschedule: starts_at = %s, want %s", startsAt, secondStart)
	}
}
