package scheduling

import (
	"context"
	"errors"
	"testing"

	toolapi "github.com/reinhlord/kontor/internal/tools"
)

func TestToolBackendRejectsTenantBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	backend := NewToolBackend(&PGXRepository{tenantID: DefaultTenantID})
	_, err := backend.ListServices(context.Background(), "00000000-0000-4000-8000-000000000099")
	if !errors.Is(err, toolapi.ErrNotFoundOrNotOwned) {
		t.Fatalf("expected indistinguishable tenant refusal, got %v", err)
	}
	_, err = backend.ListServices(context.Background(), DefaultTenantID)
	if !errors.Is(err, toolapi.ErrDependencyUnavailable) {
		t.Fatalf("expected unavailable repository for matching tenant, got %v", err)
	}
}

func TestMapToolBackendErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input error
		want  error
	}{
		{ErrNotFound, toolapi.ErrNotFoundOrNotOwned},
		{ErrSlotUnavailable, toolapi.ErrSlotUnavailable},
		{ErrIdempotencyConflict, toolapi.ErrIdempotencyConflict},
		{context.DeadlineExceeded, toolapi.ErrDependencyUnavailable},
		{errors.New("postgres unavailable"), toolapi.ErrDependencyUnavailable},
	}
	for _, test := range tests {
		if got := mapToolBackendError(test.input); !errors.Is(got, test.want) {
			t.Errorf("mapToolBackendError(%v) = %v, want wrapping %v", test.input, got, test.want)
		}
	}
}
