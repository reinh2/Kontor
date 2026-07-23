package scheduling

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// mapDatabaseError is the seam that turns a raw PostgreSQL failure into a
// domain sentinel a caller can act on. Getting it wrong is what made an
// overlapping reschedule look like a retryable dependency outage, so its
// classification is pinned here without needing a database.
func TestMapDatabaseErrorClassifiesConstraintViolations(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   error
		want error
	}{
		{
			name: "exclusion constraint means the slot was taken",
			in:   &pgconn.PgError{Code: "23P01", Message: "conflicting key value violates exclusion constraint"},
			want: ErrSlotUnavailable,
		},
		{
			name: "foreign key violation means a missing resource",
			in:   &pgconn.PgError{Code: "23503", Message: "insert violates foreign key constraint"},
			want: ErrNotFound,
		},
		{
			name: "wrapped exclusion constraint is still classified",
			in:   fmt.Errorf("create booking: %w", &pgconn.PgError{Code: "23P01"}),
			want: ErrSlotUnavailable,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := mapDatabaseError(testCase.in)
			if !errors.Is(got, testCase.want) {
				t.Fatalf("mapDatabaseError(%v) = %v, want it to wrap %v", testCase.in, got, testCase.want)
			}
		})
	}
}

func TestMapDatabaseErrorPreservesDomainSentinels(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		ErrInvalidInput, ErrNotFound, ErrSlotUnavailable,
		ErrIdempotencyConflict, ErrBookingStateConflict, ErrScheduleVersionConflict,
	}
	for _, sentinel := range sentinels {
		wrapped := fmt.Errorf("repository: %w", sentinel)
		if got := mapDatabaseError(wrapped); !errors.Is(got, sentinel) {
			t.Fatalf("mapDatabaseError lost sentinel %v: got %v", sentinel, got)
		}
	}
}

func TestMapDatabaseErrorLeavesUnknownFailuresAlone(t *testing.T) {
	t.Parallel()

	// An unrecognized failure must not be dressed up as a domain outcome: the
	// caller has to see it as an infrastructure error.
	original := &pgconn.PgError{Code: "53300", Message: "too many connections"}
	got := mapDatabaseError(original)
	if !errors.Is(got, original) {
		t.Fatalf("mapDatabaseError rewrote an unknown failure: got %v", got)
	}
	for _, sentinel := range []error{ErrSlotUnavailable, ErrNotFound, ErrInvalidInput} {
		if errors.Is(got, sentinel) {
			t.Fatalf("mapDatabaseError misclassified %v as %v", original, sentinel)
		}
	}
}

// isTransactionRetry decides whether a serializable transaction is replayed.
// Classifying too broadly would replay a genuine failure; too narrowly turns a
// routine serialization conflict into a customer-visible error.
func TestIsTransactionRetryOnlyMatchesSerializationFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   error
		want bool
	}{
		{"serialization failure", &pgconn.PgError{Code: "40001"}, true},
		{"deadlock detected", &pgconn.PgError{Code: "40P01"}, true},
		{"wrapped serialization failure", fmt.Errorf("commit: %w", &pgconn.PgError{Code: "40001"}), true},
		{"exclusion constraint is not retryable", &pgconn.PgError{Code: "23P01"}, false},
		{"plain error is not retryable", errors.New("boom"), false},
		{"nil is not retryable", nil, false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if got := isTransactionRetry(testCase.in); got != testCase.want {
				t.Fatalf("isTransactionRetry(%v) = %t, want %t", testCase.in, got, testCase.want)
			}
		})
	}
}

// The operator idempotency digest must identify an appointment, so an
// accidental double submit is absorbed while a genuinely different request is
// not.
func TestHashAdminCreateBookingIsStableAndFactSensitive(t *testing.T) {
	t.Parallel()

	startsAt := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	base := AdminCreateBookingRequest{
		CustomerID: "11111111-1111-4111-8111-111111111111",
		ServiceID:  "22222222-2222-4222-8222-222222222222",
		StaffID:    "33333333-3333-4333-8333-333333333333",
		StartsAt:   startsAt,
		Notes:      "window seat",
	}

	baseDigest, err := hashAdminCreateBooking(base)
	if err != nil {
		t.Fatalf("hashAdminCreateBooking: %v", err)
	}
	if len(baseDigest) != 64 {
		t.Fatalf("digest = %q, want 64 hex characters", baseDigest)
	}

	repeat, err := hashAdminCreateBooking(base)
	if err != nil {
		t.Fatalf("hashAdminCreateBooking repeat: %v", err)
	}
	if repeat != baseDigest {
		t.Fatalf("digest is not stable: %q then %q", baseDigest, repeat)
	}

	// The same instant expressed in another zone is the same appointment.
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	sameInstant := base
	sameInstant.StartsAt = startsAt.In(berlin)
	shifted, err := hashAdminCreateBooking(sameInstant)
	if err != nil {
		t.Fatalf("hashAdminCreateBooking shifted: %v", err)
	}
	if shifted != baseDigest {
		t.Fatalf("digest changed for the same instant in another zone: %q vs %q", shifted, baseDigest)
	}

	// Fields that do not identify the appointment must not change the digest,
	// and every field that does must change it.
	ignored := base
	ignored.ActorRef = "operator-7"
	ignored.IdempotencyKey = "a-different-key"
	if got, err := hashAdminCreateBooking(ignored); err != nil || got != baseDigest {
		t.Fatalf("digest changed for non-identifying fields: %q vs %q (err %v)", got, baseDigest, err)
	}

	variants := map[string]AdminCreateBookingRequest{
		"customer": {CustomerID: "other", ServiceID: base.ServiceID, StaffID: base.StaffID, StartsAt: startsAt, Notes: base.Notes},
		"service":  {CustomerID: base.CustomerID, ServiceID: "other", StaffID: base.StaffID, StartsAt: startsAt, Notes: base.Notes},
		"staff":    {CustomerID: base.CustomerID, ServiceID: base.ServiceID, StaffID: "other", StartsAt: startsAt, Notes: base.Notes},
		"start":    {CustomerID: base.CustomerID, ServiceID: base.ServiceID, StaffID: base.StaffID, StartsAt: startsAt.Add(time.Minute), Notes: base.Notes},
		"notes":    {CustomerID: base.CustomerID, ServiceID: base.ServiceID, StaffID: base.StaffID, StartsAt: startsAt, Notes: "aisle seat"},
	}
	for name, variant := range variants {
		got, err := hashAdminCreateBooking(variant)
		if err != nil {
			t.Fatalf("hashAdminCreateBooking %s: %v", name, err)
		}
		if got == baseDigest {
			t.Fatalf("digest ignored a change to %s", name)
		}
	}
}

func TestStableStaffOrderIsDeterministic(t *testing.T) {
	t.Parallel()

	// Two staff members share a display name, so only the ID can break the tie.
	staff := []Staff{
		{ID: "c", DisplayName: "Nadia P."},
		{ID: "a", DisplayName: "Nadia P."},
		{ID: "b", DisplayName: "Adam K."},
	}
	StableStaffOrder(staff)

	want := []string{"b", "a", "c"}
	for index, id := range want {
		if staff[index].ID != id {
			t.Fatalf("order = %v, want IDs %v", staff, want)
		}
	}

	// Sorting an already-sorted slice must not disturb it.
	StableStaffOrder(staff)
	for index, id := range want {
		if staff[index].ID != id {
			t.Fatalf("re-sorting changed the order: %v", staff)
		}
	}
}

func TestMinutesTruncatesTowardZero(t *testing.T) {
	t.Parallel()

	cases := map[time.Duration]int{
		0:                             0,
		30 * time.Second:              0,
		time.Minute:                   1,
		90 * time.Second:              1,
		2 * time.Hour:                 120,
		-15 * time.Minute:             -15,
		time.Minute + time.Nanosecond: 1,
	}
	for duration, want := range cases {
		if got := minutes(duration); got != want {
			t.Fatalf("minutes(%v) = %d, want %d", duration, got, want)
		}
	}
}
