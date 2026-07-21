package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestMemoryTokenBudgetSettlesAndEnforcesPerConversationCap(t *testing.T) {
	t.Parallel()
	budget, err := NewMemoryTokenBudget(100)
	if err != nil {
		t.Fatal(err)
	}

	reservation, err := budget.Reserve(context.Background(), "conversation-a", 80)
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.Settle(context.Background(), 30); err != nil {
		t.Fatal(err)
	}
	if got := budget.Accounted("conversation-a"); got != 30 {
		t.Fatalf("accounted = %d, want 30", got)
	}
	if _, err := budget.Reserve(context.Background(), "conversation-a", 71); !errors.Is(err, ErrTokenBudgetExceeded) {
		t.Fatalf("Reserve error = %v, want ErrTokenBudgetExceeded", err)
	}
	if _, err := budget.Reserve(context.Background(), "conversation-b", 100); err != nil {
		t.Fatalf("independent conversation reserve: %v", err)
	}
}

func TestMemoryTokenBudgetCountsConcurrentReservations(t *testing.T) {
	t.Parallel()
	budget, err := NewMemoryTokenBudget(100)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wait.Done()
			<-start
			_, reserveErr := budget.Reserve(context.Background(), "same", 60)
			results <- reserveErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes, refusals := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrTokenBudgetExceeded):
			refusals++
		default:
			t.Fatalf("unexpected reservation error: %v", result)
		}
	}
	if successes != 1 || refusals != 1 {
		t.Fatalf("successes=%d refusals=%d, want 1 and 1", successes, refusals)
	}
}

func TestMemoryTokenBudgetKeepsReservationWhenUsageExceedsIt(t *testing.T) {
	t.Parallel()
	budget, err := NewMemoryTokenBudget(100)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := budget.Reserve(context.Background(), "conversation", 40)
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.Settle(context.Background(), 41); !errors.Is(err, ErrUsageExceedsReservation) {
		t.Fatalf("Settle error = %v, want ErrUsageExceedsReservation", err)
	}
	if got := budget.Accounted("conversation"); got != 40 {
		t.Fatalf("accounted = %d, want full reservation 40", got)
	}
}
