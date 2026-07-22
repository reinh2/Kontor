package operatorhttp

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/agenttrace"
	"github.com/reinhlord/kontor/internal/identity"
	"github.com/reinhlord/kontor/internal/scheduling"
)

// MultiTenantPostgreSQL builds a short-lived tenant-scoped read/write backend
// from the validated operator principal in each request context. It is the
// Stage 6 replacement for the fixed-tenant Stage 5 PostgreSQL backend.
type MultiTenantPostgreSQL struct {
	pool *pgxpool.Pool
}

func NewMultiTenantPostgreSQL(pool *pgxpool.Pool) (*MultiTenantPostgreSQL, error) {
	if pool == nil {
		return nil, errors.New("operator multitenant postgres: nil pool")
	}
	return &MultiTenantPostgreSQL{pool: pool}, nil
}

func (s *MultiTenantPostgreSQL) scoped(ctx context.Context) (*PostgreSQL, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("operator multitenant postgres: nil backend")
	}
	principal, ok := identity.PrincipalFromContext(ctx)
	if !ok {
		return nil, identity.ErrSessionInvalid
	}
	backend, err := NewPostgreSQL(
		s.pool,
		agenttrace.NewStore(s.pool, principal.TenantID),
		scheduling.NewPGXRepository(s.pool, principal.TenantID),
		principal.TenantID,
		principal.Timezone,
	)
	if err != nil {
		return nil, err
	}
	backend.actorRef = principal.OperatorID
	return backend, nil
}

func (s *MultiTenantPostgreSQL) Dashboard(ctx context.Context, request DashboardRequest) (Dashboard, error) {
	backend, err := s.scoped(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	return backend.Dashboard(ctx, request)
}

func (s *MultiTenantPostgreSQL) ListRuns(ctx context.Context, request ListRunsRequest) (RunPage, error) {
	backend, err := s.scoped(ctx)
	if err != nil {
		return RunPage{}, err
	}
	return backend.ListRuns(ctx, request)
}

func (s *MultiTenantPostgreSQL) GetRun(ctx context.Context, runID string) (RunDetail, error) {
	backend, err := s.scoped(ctx)
	if err != nil {
		return RunDetail{}, err
	}
	return backend.GetRun(ctx, runID)
}

func (s *MultiTenantPostgreSQL) Calendar(ctx context.Context, request CalendarRequest) (Calendar, error) {
	backend, err := s.scoped(ctx)
	if err != nil {
		return Calendar{}, err
	}
	return backend.Calendar(ctx, request)
}

func (s *MultiTenantPostgreSQL) ListCustomers(ctx context.Context, request CustomerListRequest) (CustomerList, error) {
	backend, err := s.scoped(ctx)
	if err != nil {
		return CustomerList{}, err
	}
	return backend.ListCustomers(ctx, request)
}

func (s *MultiTenantPostgreSQL) CreateBooking(ctx context.Context, command CreateBookingCommand) (Booking, error) {
	backend, err := s.scoped(ctx)
	if err != nil {
		return Booking{}, err
	}
	return backend.CreateBooking(ctx, command)
}

func (s *MultiTenantPostgreSQL) RescheduleBooking(ctx context.Context, command RescheduleBookingCommand) (Booking, error) {
	backend, err := s.scoped(ctx)
	if err != nil {
		return Booking{}, err
	}
	return backend.RescheduleBooking(ctx, command)
}

func (s *MultiTenantPostgreSQL) CancelBooking(ctx context.Context, command CancelBookingCommand) (Booking, error) {
	backend, err := s.scoped(ctx)
	if err != nil {
		return Booking{}, err
	}
	return backend.CancelBooking(ctx, command)
}

var _ Backend = (*MultiTenantPostgreSQL)(nil)
