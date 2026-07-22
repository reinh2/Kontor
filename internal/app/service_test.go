package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTurnAdmissionFailsWithTypedOverloadAfterBoundedWait(t *testing.T) {
	service := &Service{
		turnAdmission: make(chan struct{}, 1),
		admissionWait: 10 * time.Millisecond,
	}
	service.turnAdmission <- struct{}{}

	startedAt := time.Now()
	release, err := service.acquireTurnAdmission(context.Background())
	if release != nil {
		release()
		t.Fatal("overloaded admission unexpectedly returned a release function")
	}
	if !errors.Is(err, ErrTurnOverloaded) {
		t.Fatalf("admission error=%v, want ErrTurnOverloaded", err)
	}
	var overload *TurnOverloadError
	if !errors.As(err, &overload) || overload.Waited != 10*time.Millisecond {
		t.Fatalf("admission error type=%T value=%#v", err, overload)
	}
	if elapsed := time.Since(startedAt); elapsed > 250*time.Millisecond {
		t.Fatalf("overload response took %s, want a bounded short wait", elapsed)
	}
}

func TestTurnAdmissionHonorsEarlierContextCancellation(t *testing.T) {
	service := &Service{
		turnAdmission: make(chan struct{}, 1),
		admissionWait: time.Second,
	}
	service.turnAdmission <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	release, err := service.acquireTurnAdmission(ctx)
	if release != nil {
		release()
		t.Fatal("cancelled admission unexpectedly returned a release function")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("admission error=%v, want context cancellation", err)
	}
}

func TestSafeErrorTruncatesAtUTF8Boundary(t *testing.T) {
	input := strings.Repeat("a", 1899) + "€" + strings.Repeat("z", 20)
	got := safeError(errors.New(input))
	if !utf8.ValidString(got) {
		t.Fatalf("safeError returned invalid UTF-8: %q", got[len(got)-8:])
	}
	if len(got) != 1899 || got != strings.Repeat("a", 1899) {
		t.Fatalf("safeError length=%d suffix=%q, want 1899 ASCII bytes", len(got), got[len(got)-8:])
	}
}
