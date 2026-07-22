package scheduling

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/jobqueue"
)

const createBookingScope = "booking.create.v1"

// FindSlotsRequest queries one service and optionally one eligible staff
// member. From and To are absolute instants and To is exclusive.
type FindSlotsRequest struct {
	ServiceID    string
	StaffID      string
	From         time.Time
	To           time.Time
	SlotInterval time.Duration
	Limit        int
}

// Booking is the persisted appointment projection returned by the repository.
type Booking struct {
	ID                  string    `json:"id"`
	CustomerID          string    `json:"customer_id"`
	ConversationID      string    `json:"conversation_id,omitempty"`
	ServiceID           string    `json:"service_id"`
	StaffID             string    `json:"staff_id"`
	Status              string    `json:"status"`
	StartsAt            time.Time `json:"starts_at"`
	EndsAt              time.Time `json:"ends_at"`
	BufferBeforeMinutes int       `json:"buffer_before_minutes"`
	BufferAfterMinutes  int       `json:"buffer_after_minutes"`
	ScheduleVersion     int       `json:"schedule_version"`
	Notes               string    `json:"notes,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// CreateBookingRequest contains trusted, server-resolved identifiers. Tenant
// identity is intentionally not accepted here; it comes from the repository.
type CreateBookingRequest struct {
	CustomerID     string
	ConversationID string
	ServiceID      string
	StaffID        string
	StartsAt       time.Time
	Notes          string
	IdempotencyKey string
}

// CreateBookingResult reports whether an earlier successful result was replayed.
type CreateBookingResult struct {
	Booking  Booking `json:"booking"`
	Replayed bool    `json:"replayed"`
}

// RescheduleBookingRequest contains the inputs for rescheduling a booking.
type RescheduleBookingRequest struct {
	BookingID       string
	OwnerCustomerID string
	NewStartsAt     time.Time
	NewEndsAt       time.Time
	Timezone        string
	IdempotencyKey  string
}

// RescheduleBookingResult reports the outcome of a reschedule operation.
type RescheduleBookingResult struct {
	Booking  Booking
	Replayed bool
}

// CancelBookingRequest contains the inputs for cancelling a booking.
type CancelBookingRequest struct {
	BookingID       string
	OwnerCustomerID string
	Reason          string
	IdempotencyKey  string
}

// CancelBookingResult reports the outcome of a cancellation.
type CancelBookingResult struct {
	Booking  Booking
	Replayed bool
}

// PGXRepository is the tenant-scoped PostgreSQL scheduling store.
type PGXRepository struct {
	pool     *pgxpool.Pool
	tenantID string
	engine   *Engine
}

// NewPGXRepository constructs a repository. An empty tenantID selects the
// fixed Stage 1-3 tenant; no public method permits overriding it per call.
func NewPGXRepository(pool *pgxpool.Pool, tenantID string) *PGXRepository {
	if tenantID == "" {
		tenantID = DefaultTenantID
	}
	return &PGXRepository{pool: pool, tenantID: tenantID, engine: NewEngine(nil)}
}

// ListServices returns the active catalog in stable display order.
func (r *PGXRepository) ListServices(ctx context.Context) ([]Service, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("scheduling repository: nil pool")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, slug, name, description, duration_minutes,
		       buffer_before_minutes, buffer_after_minutes, price_minor, currency
		FROM services
		WHERE tenant_id = $1 AND active
		ORDER BY name, id`, r.tenantID)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		service, scanErr := scanService(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan service: %w", scanErr)
		}
		services = append(services, service)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list services rows: %w", err)
	}
	return services, nil
}

// ListStaff returns active staff allowed to perform serviceID.
func (r *PGXRepository) ListStaff(ctx context.Context, serviceID string) ([]Staff, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("scheduling repository: nil pool")
	}
	if serviceID == "" {
		return nil, fmt.Errorf("%w: service_id is required", ErrInvalidInput)
	}
	return listStaff(ctx, r.pool, r.tenantID, serviceID, "")
}

// FindSlots builds a consistent availability snapshot and runs the pure slot
// engine for every requested eligible staff member.
func (r *PGXRepository) FindSlots(ctx context.Context, request FindSlotsRequest) ([]Slot, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("scheduling repository: nil pool")
	}
	if request.ServiceID == "" || request.From.IsZero() || request.To.IsZero() || !request.From.Before(request.To) {
		return nil, fmt.Errorf("%w: service_id and a positive date range are required", ErrInvalidInput)
	}
	if request.Limit < 0 || request.Limit > 1000 {
		return nil, fmt.Errorf("%w: limit outside allowed range", ErrInvalidInput)
	}
	service, err := loadService(ctx, r.pool, r.tenantID, request.ServiceID)
	if err != nil {
		return nil, err
	}
	staff, err := listStaff(ctx, r.pool, r.tenantID, request.ServiceID, request.StaffID)
	if err != nil {
		return nil, err
	}
	if len(staff) == 0 {
		return nil, fmt.Errorf("%w: no eligible staff", ErrNotFound)
	}

	var all []Slot
	perStaffLimit := request.Limit
	if perStaffLimit == 0 {
		perStaffLimit = 1000
	}
	for _, member := range staff {
		rules, loadErr := loadRules(ctx, r.pool, r.tenantID, member.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		busy, loadErr := loadBusy(ctx, r.pool, r.tenantID, member.ID, request.From, request.To)
		if loadErr != nil {
			return nil, loadErr
		}
		slots, findErr := r.engine.FindSlots(ctx, SearchInput{
			Service:      service,
			Staff:        member,
			From:         request.From,
			To:           request.To,
			Rules:        rules,
			Busy:         busy,
			SlotInterval: request.SlotInterval,
			Limit:        perStaffLimit,
		})
		if findErr != nil {
			return nil, findErr
		}
		all = append(all, slots...)
	}
	sortSlots(all)
	if request.Limit > 0 && len(all) > request.Limit {
		all = all[:request.Limit]
	}
	return all, nil
}

// CreateBooking serializes writers for the staff member's local date, rechecks
// the exact slot under that lock, and then inserts both the booking and its
// audit event atomically. PostgreSQL's exclusion constraint is the final guard.
func (r *PGXRepository) CreateBooking(ctx context.Context, request CreateBookingRequest) (CreateBookingResult, error) {
	if r == nil || r.pool == nil {
		return CreateBookingResult{}, fmt.Errorf("scheduling repository: nil pool")
	}
	if err := validateCreateBooking(request); err != nil {
		return CreateBookingResult{}, err
	}
	requestHash, err := hashCreateBooking(request)
	if err != nil {
		return CreateBookingResult{}, err
	}

	var result CreateBookingResult
	for attempt := 1; attempt <= 3; attempt++ {
		result, err = r.createBookingOnce(ctx, request, requestHash)
		if err == nil {
			return result, nil
		}
		if !isTransactionRetry(err) || attempt == 3 {
			return CreateBookingResult{}, mapDatabaseError(err)
		}
		select {
		case <-ctx.Done():
			return CreateBookingResult{}, ctx.Err()
		case <-time.After(time.Duration(attempt*10) * time.Millisecond):
		}
	}
	return CreateBookingResult{}, mapDatabaseError(err)
}

func (r *PGXRepository) createBookingOnce(ctx context.Context, request CreateBookingRequest, requestHash string) (CreateBookingResult, error) {
	idempotencyScope := createBookingScope + ":" + request.CustomerID
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("begin booking transaction: %w", err)
	}
	defer rollbackTx(tx)

	command, err := tx.Exec(ctx, `
		INSERT INTO idempotency_records
		    (tenant_id, scope, idempotency_key, request_hash, status)
		VALUES ($1, $2, $3, $4, 'in_progress')
		ON CONFLICT (tenant_id, scope, idempotency_key) DO NOTHING`,
		r.tenantID, idempotencyScope, request.IdempotencyKey, requestHash)
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("reserve idempotency key: %w", err)
	}
	if command.RowsAffected() == 0 {
		var storedHash, status, resourceID string
		err = tx.QueryRow(ctx, `
			SELECT request_hash, status, COALESCE(resource_id::text, '')
			FROM idempotency_records
			WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3
			FOR UPDATE`, r.tenantID, idempotencyScope, request.IdempotencyKey).
			Scan(&storedHash, &status, &resourceID)
		if err != nil {
			return CreateBookingResult{}, fmt.Errorf("read idempotency key: %w", err)
		}
		if storedHash != requestHash {
			return CreateBookingResult{}, ErrIdempotencyConflict
		}
		if status != "completed" || resourceID == "" {
			return CreateBookingResult{}, fmt.Errorf("idempotency record is unexpectedly incomplete")
		}
		booking, loadErr := loadBooking(ctx, tx, r.tenantID, resourceID)
		if loadErr != nil {
			return CreateBookingResult{}, loadErr
		}
		if err := tx.Commit(ctx); err != nil {
			return CreateBookingResult{}, fmt.Errorf("commit idempotency replay: %w", err)
		}
		return CreateBookingResult{Booking: booking, Replayed: true}, nil
	}

	service, member, err := r.loadBookingInputs(ctx, tx, request.ServiceID, request.StaffID)
	if err != nil {
		return CreateBookingResult{}, err
	}
	location, err := time.LoadLocation(member.Timezone)
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("%w: stored staff timezone %q: %v", ErrInvalidInput, member.Timezone, err)
	}
	endsAt := request.StartsAt.Add(service.Duration)
	lockDates := touchedLocalDates(
		request.StartsAt.Add(-service.BufferBefore),
		endsAt.Add(service.BufferAfter),
		location,
	)
	if err := r.acquireScheduleLocks(ctx, tx, request.StaffID, lockDates); err != nil {
		return CreateBookingResult{}, err
	}
	// Recheck against the database wall clock after any lock wait. The early
	// application-clock check rejects obvious stale calls cheaply; this one
	// prevents a near-boundary request from becoming past while queued.
	var stillFuture bool
	if err := tx.QueryRow(ctx, `SELECT $1::timestamptz > clock_timestamp()`, request.StartsAt).Scan(&stillFuture); err != nil {
		return CreateBookingResult{}, fmt.Errorf("recheck booking start time: %w", err)
	}
	if !stillFuture {
		return CreateBookingResult{}, ErrSlotUnavailable
	}
	rules, err := loadRules(ctx, tx, r.tenantID, member.ID)
	if err != nil {
		return CreateBookingResult{}, err
	}

	dayStart := time.Date(request.StartsAt.In(location).Year(), request.StartsAt.In(location).Month(), request.StartsAt.In(location).Day(), 0, 0, 0, 0, location)
	dayEnd := dayStart.AddDate(0, 0, 1)
	busy, err := loadBusy(ctx, tx, r.tenantID, member.ID, dayStart, dayEnd)
	if err != nil {
		return CreateBookingResult{}, err
	}
	available, err := r.engine.IsAvailable(ctx, SearchInput{
		Service: service, Staff: member, From: dayStart, To: dayEnd,
		Rules: rules, Busy: busy, SlotInterval: 15 * time.Minute,
	}, request.StartsAt)
	if err != nil {
		return CreateBookingResult{}, err
	}
	if !available {
		return CreateBookingResult{}, ErrSlotUnavailable
	}

	var booking Booking
	err = tx.QueryRow(ctx, `
		INSERT INTO bookings (
		    tenant_id, customer_id, conversation_id, service_id, staff_id,
		    starts_at, ends_at, buffer_before_minutes, buffer_after_minutes, notes
		) VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		          service_id::text, staff_id::text, status, starts_at, ends_at,
		          buffer_before_minutes, buffer_after_minutes, schedule_version,
		          notes, created_at, updated_at`,
		r.tenantID, request.CustomerID, request.ConversationID, request.ServiceID, request.StaffID,
		request.StartsAt, endsAt, minutes(service.BufferBefore), minutes(service.BufferAfter), request.Notes).
		Scan(&booking.ID, &booking.CustomerID, &booking.ConversationID, &booking.ServiceID, &booking.StaffID,
			&booking.Status, &booking.StartsAt, &booking.EndsAt, &booking.BufferBeforeMinutes,
			&booking.BufferAfterMinutes, &booking.ScheduleVersion, &booking.Notes, &booking.CreatedAt, &booking.UpdatedAt)
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("insert booking: %w", err)
	}

	state, err := json.Marshal(booking)
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("encode booking event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_events
		    (tenant_id, booking_id, event_type, actor_type, to_state)
		VALUES ($1, $2, 'created', 'agent', $3::jsonb)`, r.tenantID, booking.ID, string(state)); err != nil {
		return CreateBookingResult{}, fmt.Errorf("insert booking event: %w", err)
	}
	// Transactionally enqueue reminder and CRM jobs so they are guaranteed to
	// execute if and only if the booking commits.
	if err := r.enqueueBookingCreatedJobs(ctx, tx, booking, service.Name, member.DisplayName, member.Timezone); err != nil {
		return CreateBookingResult{}, err
	}
	response, err := json.Marshal(CreateBookingResult{Booking: booking})
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("encode idempotency response: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE idempotency_records
		SET status = 'completed', resource_id = $4, response_json = $5::jsonb, completed_at = now()
		WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3`,
		r.tenantID, idempotencyScope, request.IdempotencyKey, booking.ID, string(response)); err != nil {
		return CreateBookingResult{}, fmt.Errorf("complete idempotency key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateBookingResult{}, fmt.Errorf("commit booking: %w", err)
	}
	return CreateBookingResult{Booking: booking}, nil
}

// RescheduleBooking atomically moves a confirmed booking to a new time slot.
// It uses the same serializable lock pattern as CreateBooking to prevent double
// booking at the new time.
func (r *PGXRepository) RescheduleBooking(ctx context.Context, request RescheduleBookingRequest) (RescheduleBookingResult, error) {
	if r == nil || r.pool == nil {
		return RescheduleBookingResult{}, fmt.Errorf("scheduling repository: nil pool")
	}
	if request.BookingID == "" || request.OwnerCustomerID == "" || request.NewStartsAt.IsZero() || request.NewEndsAt.IsZero() {
		return RescheduleBookingResult{}, fmt.Errorf("%w: booking_id, owner, and new time range are required", ErrInvalidInput)
	}
	if !request.NewStartsAt.Before(request.NewEndsAt) {
		return RescheduleBookingResult{}, fmt.Errorf("%w: new_starts_at must be before new_ends_at", ErrInvalidInput)
	}

	const rescheduleScope = "booking.reschedule.v1"
	idempotencyScope := rescheduleScope + ":" + request.OwnerCustomerID

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("begin reschedule transaction: %w", err)
	}
	defer rollbackTx(tx)

	// Idempotency check for reschedule.
	if request.IdempotencyKey != "" {
		requestHash := fmt.Sprintf("reschedule:%s:%s:%s", request.BookingID, request.NewStartsAt.UTC().Format(time.RFC3339Nano), request.NewEndsAt.UTC().Format(time.RFC3339Nano))
		tag, err := tx.Exec(ctx, `
			INSERT INTO idempotency_records (tenant_id, scope, idempotency_key, request_hash, status)
			VALUES ($1, $2, $3, $4, 'in_progress')
			ON CONFLICT (tenant_id, scope, idempotency_key) DO NOTHING`,
			r.tenantID, idempotencyScope, request.IdempotencyKey, requestHash)
		if err != nil {
			return RescheduleBookingResult{}, fmt.Errorf("reserve idempotency key: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// Idempotent replay: load and return the booking.
			var resourceID string
			err = tx.QueryRow(ctx, `
				SELECT COALESCE(resource_id::text, '')
				FROM idempotency_records
				WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3 AND status = 'completed'
				FOR UPDATE`, r.tenantID, idempotencyScope, request.IdempotencyKey).Scan(&resourceID)
			if err != nil || resourceID == "" {
				return RescheduleBookingResult{}, fmt.Errorf("idempotency record incomplete for reschedule")
			}
			booking, loadErr := loadBooking(ctx, tx, r.tenantID, resourceID)
			if loadErr != nil {
				return RescheduleBookingResult{}, loadErr
			}
			if err := tx.Commit(ctx); err != nil {
				return RescheduleBookingResult{}, fmt.Errorf("commit idempotency replay: %w", err)
			}
			return RescheduleBookingResult{Booking: booking, Replayed: true}, nil
		}
	}

	// Load current booking and verify ownership and status.
	var current Booking
	err = tx.QueryRow(ctx, `
		SELECT id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		       service_id::text, staff_id::text, status, starts_at, ends_at,
		       buffer_before_minutes, buffer_after_minutes, schedule_version,
		       notes, created_at, updated_at
		FROM bookings
		WHERE tenant_id = $1 AND id = $2 AND customer_id = $3
		FOR UPDATE`, r.tenantID, request.BookingID, request.OwnerCustomerID).
		Scan(&current.ID, &current.CustomerID, &current.ConversationID, &current.ServiceID, &current.StaffID,
			&current.Status, &current.StartsAt, &current.EndsAt, &current.BufferBeforeMinutes,
			&current.BufferAfterMinutes, &current.ScheduleVersion, &current.Notes, &current.CreatedAt, &current.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RescheduleBookingResult{}, ErrNotFound
		}
		return RescheduleBookingResult{}, fmt.Errorf("load booking for reschedule: %w", err)
	}
	if current.Status != "confirmed" {
		return RescheduleBookingResult{}, ErrBookingStateConflict
	}

	// Lock the new time range and verify availability.
	location, err := time.LoadLocation(request.Timezone)
	if err != nil {
		location = time.UTC
	}
	lockDates := touchedLocalDates(
		request.NewStartsAt.Add(-time.Duration(current.BufferBeforeMinutes)*time.Minute),
		request.NewEndsAt.Add(time.Duration(current.BufferAfterMinutes)*time.Minute),
		location,
	)
	if err := r.acquireScheduleLocks(ctx, tx, current.StaffID, lockDates); err != nil {
		return RescheduleBookingResult{}, err
	}

	// Update the booking with new times.
	var updated Booking
	err = tx.QueryRow(ctx, `
		UPDATE bookings
		SET starts_at = $1, ends_at = $2, schedule_version = schedule_version + 1, updated_at = now()
		WHERE tenant_id = $3 AND id = $4 AND status = 'confirmed'
		RETURNING id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		          service_id::text, staff_id::text, status, starts_at, ends_at,
		          buffer_before_minutes, buffer_after_minutes, schedule_version,
		          notes, created_at, updated_at`,
		request.NewStartsAt, request.NewEndsAt, r.tenantID, request.BookingID).
		Scan(&updated.ID, &updated.CustomerID, &updated.ConversationID, &updated.ServiceID, &updated.StaffID,
			&updated.Status, &updated.StartsAt, &updated.EndsAt, &updated.BufferBeforeMinutes,
			&updated.BufferAfterMinutes, &updated.ScheduleVersion, &updated.Notes, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("update booking: %w", err)
	}

	// Record rescheduled event.
	fromState, _ := json.Marshal(current)
	toState, _ := json.Marshal(updated)
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_events (tenant_id, booking_id, event_type, actor_type, from_state, to_state)
		VALUES ($1, $2, 'rescheduled', 'agent', $3::jsonb, $4::jsonb)`,
		r.tenantID, updated.ID, string(fromState), string(toState)); err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("insert reschedule event: %w", err)
	}

	// Complete idempotency record.
	if request.IdempotencyKey != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE idempotency_records
			SET status = 'completed', resource_id = $4, completed_at = now()
			WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3`,
			r.tenantID, idempotencyScope, request.IdempotencyKey, updated.ID); err != nil {
			// Non-fatal; the booking itself is committed.
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("commit reschedule: %w", err)
	}
	return RescheduleBookingResult{Booking: updated}, nil
}

// CancelBooking atomically marks a confirmed booking as cancelled.
func (r *PGXRepository) CancelBooking(ctx context.Context, request CancelBookingRequest) (CancelBookingResult, error) {
	if r == nil || r.pool == nil {
		return CancelBookingResult{}, fmt.Errorf("scheduling repository: nil pool")
	}
	if request.BookingID == "" || request.OwnerCustomerID == "" || request.Reason == "" {
		return CancelBookingResult{}, fmt.Errorf("%w: booking_id, owner, and reason are required", ErrInvalidInput)
	}

	const cancelScope = "booking.cancel.v1"
	idempotencyScope := cancelScope + ":" + request.OwnerCustomerID

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CancelBookingResult{}, fmt.Errorf("begin cancel transaction: %w", err)
	}
	defer rollbackTx(tx)

	// Idempotency check.
	if request.IdempotencyKey != "" {
		requestHash := fmt.Sprintf("cancel:%s:%s", request.BookingID, request.Reason)
		tag, err := tx.Exec(ctx, `
			INSERT INTO idempotency_records (tenant_id, scope, idempotency_key, request_hash, status)
			VALUES ($1, $2, $3, $4, 'in_progress')
			ON CONFLICT (tenant_id, scope, idempotency_key) DO NOTHING`,
			r.tenantID, idempotencyScope, request.IdempotencyKey, requestHash)
		if err != nil {
			return CancelBookingResult{}, fmt.Errorf("reserve idempotency key: %w", err)
		}
		if tag.RowsAffected() == 0 {
			var resourceID string
			err = tx.QueryRow(ctx, `
				SELECT COALESCE(resource_id::text, '')
				FROM idempotency_records
				WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3 AND status = 'completed'
				FOR UPDATE`, r.tenantID, idempotencyScope, request.IdempotencyKey).Scan(&resourceID)
			if err != nil || resourceID == "" {
				return CancelBookingResult{}, fmt.Errorf("idempotency record incomplete for cancel")
			}
			booking, loadErr := loadBooking(ctx, tx, r.tenantID, resourceID)
			if loadErr != nil {
				return CancelBookingResult{}, loadErr
			}
			if err := tx.Commit(ctx); err != nil {
				return CancelBookingResult{}, fmt.Errorf("commit idempotency replay: %w", err)
			}
			return CancelBookingResult{Booking: booking, Replayed: true}, nil
		}
	}

	// Load and verify.
	var current Booking
	err = tx.QueryRow(ctx, `
		SELECT id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		       service_id::text, staff_id::text, status, starts_at, ends_at,
		       buffer_before_minutes, buffer_after_minutes, schedule_version,
		       notes, created_at, updated_at
		FROM bookings
		WHERE tenant_id = $1 AND id = $2 AND customer_id = $3
		FOR UPDATE`, r.tenantID, request.BookingID, request.OwnerCustomerID).
		Scan(&current.ID, &current.CustomerID, &current.ConversationID, &current.ServiceID, &current.StaffID,
			&current.Status, &current.StartsAt, &current.EndsAt, &current.BufferBeforeMinutes,
			&current.BufferAfterMinutes, &current.ScheduleVersion, &current.Notes, &current.CreatedAt, &current.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CancelBookingResult{}, ErrNotFound
		}
		return CancelBookingResult{}, fmt.Errorf("load booking for cancel: %w", err)
	}
	if current.Status == "cancelled" {
		// Already cancelled — idempotent.
		if err := tx.Commit(ctx); err != nil {
			return CancelBookingResult{}, fmt.Errorf("commit already-cancelled: %w", err)
		}
		return CancelBookingResult{Booking: current, Replayed: true}, nil
	}
	if current.Status != "confirmed" {
		return CancelBookingResult{}, ErrBookingStateConflict
	}

	// Cancel the booking.
	var cancelled Booking
	err = tx.QueryRow(ctx, `
		UPDATE bookings
		SET status = 'cancelled', cancellation_reason = $1, cancelled_at = now(),
		    schedule_version = schedule_version + 1, updated_at = now()
		WHERE tenant_id = $2 AND id = $3 AND status = 'confirmed'
		RETURNING id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		          service_id::text, staff_id::text, status, starts_at, ends_at,
		          buffer_before_minutes, buffer_after_minutes, schedule_version,
		          notes, created_at, updated_at`,
		request.Reason, r.tenantID, request.BookingID).
		Scan(&cancelled.ID, &cancelled.CustomerID, &cancelled.ConversationID, &cancelled.ServiceID, &cancelled.StaffID,
			&cancelled.Status, &cancelled.StartsAt, &cancelled.EndsAt, &cancelled.BufferBeforeMinutes,
			&cancelled.BufferAfterMinutes, &cancelled.ScheduleVersion, &cancelled.Notes, &cancelled.CreatedAt, &cancelled.UpdatedAt)
	if err != nil {
		return CancelBookingResult{}, fmt.Errorf("cancel booking: %w", err)
	}

	// Record cancelled event.
	fromState, _ := json.Marshal(current)
	toState, _ := json.Marshal(cancelled)
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_events (tenant_id, booking_id, event_type, actor_type, from_state, to_state)
		VALUES ($1, $2, 'cancelled', 'agent', $3::jsonb, $4::jsonb)`,
		r.tenantID, cancelled.ID, string(fromState), string(toState)); err != nil {
		return CancelBookingResult{}, fmt.Errorf("insert cancel event: %w", err)
	}

	// Complete idempotency record.
	if request.IdempotencyKey != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE idempotency_records
			SET status = 'completed', resource_id = $4, completed_at = now()
			WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3`,
			r.tenantID, idempotencyScope, request.IdempotencyKey, cancelled.ID); err != nil {
			// Non-fatal.
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return CancelBookingResult{}, fmt.Errorf("commit cancel: %w", err)
	}
	return CancelBookingResult{Booking: cancelled}, nil
}

// enqueueBookingCreatedJobs transactionally enqueues the reminder and CRM jobs
// spawned by a newly created booking. It is shared by the customer agent path
// and the operator (admin) path so both produce an identical outbox.
func (r *PGXRepository) enqueueBookingCreatedJobs(ctx context.Context, tx pgx.Tx, booking Booking, serviceName, staffName, staffTimezone string) error {
	reminderPayload, _ := json.Marshal(map[string]string{
		"customer_name":  booking.CustomerID,
		"customer_email": "",
		"service_name":   serviceName,
		"staff_name":     staffName,
		"starts_at":      booking.StartsAt.Format(time.RFC3339),
		"timezone":       staffTimezone,
	})
	crmPayload, _ := json.Marshal(map[string]string{
		"customer_id":  booking.CustomerID,
		"display_name": booking.CustomerID,
	})
	for _, job := range []struct {
		jobType        string
		payload        []byte
		idempotencyKey string
	}{
		{"send_reminder", reminderPayload, "reminder:" + booking.ID},
		{"crm_upsert_contact", crmPayload, "crm_upsert:" + booking.ID},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO jobs (tenant_id, booking_id, job_type, payload_json, max_attempts, idempotency_key)
			VALUES ($1, $2, $3, $4::jsonb, 5, $5)
			ON CONFLICT (tenant_id, idempotency_key)
			WHERE idempotency_key IS NOT NULL DO NOTHING`,
			r.tenantID, booking.ID, job.jobType, string(job.payload), job.idempotencyKey); err != nil {
			return fmt.Errorf("enqueue %s job: %w", job.jobType, err)
		}
	}
	return nil
}

// AdminCreateBookingRequest contains the trusted, operator-supplied inputs for
// a console-created booking. Unlike the customer path there is no conversation
// or signed slot token: the operator is a trusted human placing an appointment
// directly, so the availability grid is deliberately not re-imposed. Hard
// double-booking is still prevented by the PostgreSQL exclusion constraint.
type AdminCreateBookingRequest struct {
	CustomerID     string
	ServiceID      string
	StaffID        string
	StartsAt       time.Time
	Notes          string
	ActorRef       string
	IdempotencyKey string
}

// AdminRescheduleBookingRequest moves an existing booking to NewStartsAt. It
// requires ExpectedVersion so a stale operator view cannot overwrite a change
// made in the meantime (optimistic concurrency).
type AdminRescheduleBookingRequest struct {
	BookingID       string
	ExpectedVersion int
	NewStartsAt     time.Time
	ActorRef        string
	IdempotencyKey  string
}

// AdminCancelBookingRequest cancels an existing booking. ExpectedVersion is
// required for the same optimistic-concurrency reason as reschedule.
type AdminCancelBookingRequest struct {
	BookingID       string
	ExpectedVersion int
	Reason          string
	ActorRef        string
	IdempotencyKey  string
}

const (
	adminCreateScope     = "booking.admin_create.v1"
	adminRescheduleScope = "booking.admin_reschedule.v1"
	actorTypeAdmin       = "admin"
)

// AdminCreateBooking creates a booking on behalf of an operator, recording an
// 'admin' actor in the audit trail and enqueuing the same reminder/CRM outbox
// as the customer path. It is serialized per staff local date and guarded by
// the exclusion constraint exactly like CreateBooking.
func (r *PGXRepository) AdminCreateBooking(ctx context.Context, request AdminCreateBookingRequest) (CreateBookingResult, error) {
	if r == nil || r.pool == nil {
		return CreateBookingResult{}, fmt.Errorf("scheduling repository: nil pool")
	}
	if request.CustomerID == "" || request.ServiceID == "" || request.StaffID == "" || request.StartsAt.IsZero() {
		return CreateBookingResult{}, fmt.Errorf("%w: customer, service, staff, and start are required", ErrInvalidInput)
	}
	if len(request.Notes) > 500 {
		return CreateBookingResult{}, fmt.Errorf("%w: notes exceed 500 bytes", ErrInvalidInput)
	}
	if length := len(request.IdempotencyKey); length < 16 || length > 128 {
		return CreateBookingResult{}, fmt.Errorf("%w: idempotency key length must be 16..128", ErrInvalidInput)
	}
	actorRef := request.ActorRef
	if actorRef == "" {
		actorRef = "operator"
	}
	requestHash, err := hashAdminCreateBooking(request)
	if err != nil {
		return CreateBookingResult{}, err
	}
	var result CreateBookingResult
	for attempt := 1; attempt <= 3; attempt++ {
		result, err = r.adminCreateBookingOnce(ctx, request, actorRef, requestHash)
		if err == nil {
			return result, nil
		}
		if !isTransactionRetry(err) || attempt == 3 {
			return CreateBookingResult{}, mapDatabaseError(err)
		}
		select {
		case <-ctx.Done():
			return CreateBookingResult{}, ctx.Err()
		case <-time.After(time.Duration(attempt*10) * time.Millisecond):
		}
	}
	return CreateBookingResult{}, mapDatabaseError(err)
}

func (r *PGXRepository) adminCreateBookingOnce(ctx context.Context, request AdminCreateBookingRequest, actorRef, requestHash string) (CreateBookingResult, error) {
	idempotencyScope := adminCreateScope + ":" + request.CustomerID
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("begin admin booking transaction: %w", err)
	}
	defer rollbackTx(tx)

	command, err := tx.Exec(ctx, `
		INSERT INTO idempotency_records
		    (tenant_id, scope, idempotency_key, request_hash, status)
		VALUES ($1, $2, $3, $4, 'in_progress')
		ON CONFLICT (tenant_id, scope, idempotency_key) DO NOTHING`,
		r.tenantID, idempotencyScope, request.IdempotencyKey, requestHash)
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("reserve idempotency key: %w", err)
	}
	if command.RowsAffected() == 0 {
		var storedHash, status, resourceID string
		err = tx.QueryRow(ctx, `
			SELECT request_hash, status, COALESCE(resource_id::text, '')
			FROM idempotency_records
			WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3
			FOR UPDATE`, r.tenantID, idempotencyScope, request.IdempotencyKey).
			Scan(&storedHash, &status, &resourceID)
		if err != nil {
			return CreateBookingResult{}, fmt.Errorf("read idempotency key: %w", err)
		}
		if storedHash != requestHash {
			return CreateBookingResult{}, ErrIdempotencyConflict
		}
		if status != "completed" || resourceID == "" {
			return CreateBookingResult{}, fmt.Errorf("idempotency record is unexpectedly incomplete")
		}
		booking, loadErr := loadBooking(ctx, tx, r.tenantID, resourceID)
		if loadErr != nil {
			return CreateBookingResult{}, loadErr
		}
		if err := tx.Commit(ctx); err != nil {
			return CreateBookingResult{}, fmt.Errorf("commit idempotency replay: %w", err)
		}
		return CreateBookingResult{Booking: booking, Replayed: true}, nil
	}

	service, member, err := r.loadBookingInputs(ctx, tx, request.ServiceID, request.StaffID)
	if err != nil {
		return CreateBookingResult{}, err
	}
	location, err := time.LoadLocation(member.Timezone)
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("%w: stored staff timezone %q: %v", ErrInvalidInput, member.Timezone, err)
	}
	endsAt := request.StartsAt.Add(service.Duration)
	lockDates := touchedLocalDates(
		request.StartsAt.Add(-service.BufferBefore),
		endsAt.Add(service.BufferAfter),
		location,
	)
	if err := r.acquireScheduleLocks(ctx, tx, request.StaffID, lockDates); err != nil {
		return CreateBookingResult{}, err
	}
	var stillFuture bool
	if err := tx.QueryRow(ctx, `SELECT $1::timestamptz > clock_timestamp()`, request.StartsAt).Scan(&stillFuture); err != nil {
		return CreateBookingResult{}, fmt.Errorf("recheck booking start time: %w", err)
	}
	if !stillFuture {
		return CreateBookingResult{}, ErrSlotUnavailable
	}
	// The operator override deliberately skips the availability engine (working
	// hours, breaks): a trusted human may place an out-of-grid appointment. The
	// exclusion constraint below is still the final guard against overlap.

	var booking Booking
	err = tx.QueryRow(ctx, `
		INSERT INTO bookings (
		    tenant_id, customer_id, conversation_id, service_id, staff_id,
		    starts_at, ends_at, buffer_before_minutes, buffer_after_minutes, notes
		) VALUES ($1, $2, NULL, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		          service_id::text, staff_id::text, status, starts_at, ends_at,
		          buffer_before_minutes, buffer_after_minutes, schedule_version,
		          notes, created_at, updated_at`,
		r.tenantID, request.CustomerID, request.ServiceID, request.StaffID,
		request.StartsAt, endsAt, minutes(service.BufferBefore), minutes(service.BufferAfter), request.Notes).
		Scan(&booking.ID, &booking.CustomerID, &booking.ConversationID, &booking.ServiceID, &booking.StaffID,
			&booking.Status, &booking.StartsAt, &booking.EndsAt, &booking.BufferBeforeMinutes,
			&booking.BufferAfterMinutes, &booking.ScheduleVersion, &booking.Notes, &booking.CreatedAt, &booking.UpdatedAt)
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("insert booking: %w", err)
	}

	state, err := json.Marshal(booking)
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("encode booking event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_events
		    (tenant_id, booking_id, event_type, actor_type, actor_ref, to_state)
		VALUES ($1, $2, 'created', $3, $4, $5::jsonb)`,
		r.tenantID, booking.ID, actorTypeAdmin, actorRef, string(state)); err != nil {
		return CreateBookingResult{}, fmt.Errorf("insert booking event: %w", err)
	}
	if err := r.enqueueBookingCreatedJobs(ctx, tx, booking, service.Name, member.DisplayName, member.Timezone); err != nil {
		return CreateBookingResult{}, err
	}
	response, err := json.Marshal(CreateBookingResult{Booking: booking})
	if err != nil {
		return CreateBookingResult{}, fmt.Errorf("encode idempotency response: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE idempotency_records
		SET status = 'completed', resource_id = $4, response_json = $5::jsonb, completed_at = now()
		WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3`,
		r.tenantID, idempotencyScope, request.IdempotencyKey, booking.ID, string(response)); err != nil {
		return CreateBookingResult{}, fmt.Errorf("complete idempotency key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateBookingResult{}, fmt.Errorf("commit admin booking: %w", err)
	}
	return CreateBookingResult{Booking: booking}, nil
}

// AdminRescheduleBooking moves a booking to a new start time on behalf of an
// operator. It verifies ExpectedVersion under a row lock, keeps the original
// staff/service/duration, records an 'admin' audit event, and transactionally
// moves the pending reminder to the new time.
func (r *PGXRepository) AdminRescheduleBooking(ctx context.Context, request AdminRescheduleBookingRequest) (RescheduleBookingResult, error) {
	if r == nil || r.pool == nil {
		return RescheduleBookingResult{}, fmt.Errorf("scheduling repository: nil pool")
	}
	if request.BookingID == "" || request.NewStartsAt.IsZero() {
		return RescheduleBookingResult{}, fmt.Errorf("%w: booking_id and new start are required", ErrInvalidInput)
	}
	if request.ExpectedVersion <= 0 {
		return RescheduleBookingResult{}, fmt.Errorf("%w: expected_version must be positive", ErrInvalidInput)
	}
	if length := len(request.IdempotencyKey); length < 16 || length > 128 {
		return RescheduleBookingResult{}, fmt.Errorf("%w: idempotency key length must be 16..128", ErrInvalidInput)
	}
	actorRef := request.ActorRef
	if actorRef == "" {
		actorRef = "operator"
	}
	var result RescheduleBookingResult
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		result, err = r.adminRescheduleBookingOnce(ctx, request, actorRef)
		if err == nil {
			return result, nil
		}
		if !isTransactionRetry(err) || attempt == 3 {
			return RescheduleBookingResult{}, mapDatabaseError(err)
		}
		select {
		case <-ctx.Done():
			return RescheduleBookingResult{}, ctx.Err()
		case <-time.After(time.Duration(attempt*10) * time.Millisecond):
		}
	}
	return RescheduleBookingResult{}, mapDatabaseError(err)
}

func (r *PGXRepository) adminRescheduleBookingOnce(ctx context.Context, request AdminRescheduleBookingRequest, actorRef string) (RescheduleBookingResult, error) {
	idempotencyScope := adminRescheduleScope + ":" + request.BookingID
	requestHash := fmt.Sprintf("reschedule:%s:%s", request.BookingID, request.NewStartsAt.UTC().Format(time.RFC3339Nano))

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("begin admin reschedule transaction: %w", err)
	}
	defer rollbackTx(tx)

	reserved, err := tx.Exec(ctx, `
		INSERT INTO idempotency_records (tenant_id, scope, idempotency_key, request_hash, status)
		VALUES ($1, $2, $3, $4, 'in_progress')
		ON CONFLICT (tenant_id, scope, idempotency_key) DO NOTHING`,
		r.tenantID, idempotencyScope, request.IdempotencyKey, requestHash)
	if err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("reserve idempotency key: %w", err)
	}
	if reserved.RowsAffected() == 0 {
		var storedHash, status, resourceID string
		if err := tx.QueryRow(ctx, `
			SELECT request_hash, status, COALESCE(resource_id::text, '')
			FROM idempotency_records
			WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3
			FOR UPDATE`, r.tenantID, idempotencyScope, request.IdempotencyKey).
			Scan(&storedHash, &status, &resourceID); err != nil {
			return RescheduleBookingResult{}, fmt.Errorf("read idempotency key: %w", err)
		}
		if storedHash != requestHash {
			return RescheduleBookingResult{}, ErrIdempotencyConflict
		}
		if status != "completed" || resourceID == "" {
			return RescheduleBookingResult{}, fmt.Errorf("idempotency record is unexpectedly incomplete")
		}
		booking, loadErr := loadBooking(ctx, tx, r.tenantID, resourceID)
		if loadErr != nil {
			return RescheduleBookingResult{}, loadErr
		}
		if err := tx.Commit(ctx); err != nil {
			return RescheduleBookingResult{}, fmt.Errorf("commit idempotency replay: %w", err)
		}
		return RescheduleBookingResult{Booking: booking, Replayed: true}, nil
	}

	current, err := loadBookingForUpdate(ctx, tx, r.tenantID, request.BookingID)
	if err != nil {
		return RescheduleBookingResult{}, err
	}
	if current.ScheduleVersion != request.ExpectedVersion {
		return RescheduleBookingResult{}, ErrScheduleVersionConflict
	}
	if current.Status != "confirmed" {
		return RescheduleBookingResult{}, ErrBookingStateConflict
	}
	newEndsAt := request.NewStartsAt.Add(current.EndsAt.Sub(current.StartsAt))

	var staffTimezone string
	if err := tx.QueryRow(ctx, `
		SELECT timezone FROM staff WHERE tenant_id = $1 AND id = $2`,
		r.tenantID, current.StaffID).Scan(&staffTimezone); err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("load staff timezone: %w", err)
	}
	location, err := time.LoadLocation(staffTimezone)
	if err != nil {
		location = time.UTC
	}
	lockDates := touchedLocalDates(
		request.NewStartsAt.Add(-time.Duration(current.BufferBeforeMinutes)*time.Minute),
		newEndsAt.Add(time.Duration(current.BufferAfterMinutes)*time.Minute),
		location,
	)
	if err := r.acquireScheduleLocks(ctx, tx, current.StaffID, lockDates); err != nil {
		return RescheduleBookingResult{}, err
	}

	var updated Booking
	err = tx.QueryRow(ctx, `
		UPDATE bookings
		SET starts_at = $1, ends_at = $2, schedule_version = schedule_version + 1, updated_at = now()
		WHERE tenant_id = $3 AND id = $4 AND status = 'confirmed'
		RETURNING id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		          service_id::text, staff_id::text, status, starts_at, ends_at,
		          buffer_before_minutes, buffer_after_minutes, schedule_version,
		          notes, created_at, updated_at`,
		request.NewStartsAt, newEndsAt, r.tenantID, request.BookingID).
		Scan(&updated.ID, &updated.CustomerID, &updated.ConversationID, &updated.ServiceID, &updated.StaffID,
			&updated.Status, &updated.StartsAt, &updated.EndsAt, &updated.BufferBeforeMinutes,
			&updated.BufferAfterMinutes, &updated.ScheduleVersion, &updated.Notes, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("update booking: %w", err)
	}

	fromState, _ := json.Marshal(current)
	toState, _ := json.Marshal(updated)
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_events (tenant_id, booking_id, event_type, actor_type, actor_ref, from_state, to_state)
		VALUES ($1, $2, 'rescheduled', $3, $4, $5::jsonb, $6::jsonb)`,
		r.tenantID, updated.ID, actorTypeAdmin, actorRef, string(fromState), string(toState)); err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("insert reschedule event: %w", err)
	}
	if _, err := jobqueue.UpdateReminderSchedule(ctx, tx, r.tenantID, updated.ID, updated.StartsAt, staffTimezone); err != nil {
		return RescheduleBookingResult{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE idempotency_records
		SET status = 'completed', resource_id = $4, completed_at = now()
		WHERE tenant_id = $1 AND scope = $2 AND idempotency_key = $3`,
		r.tenantID, idempotencyScope, request.IdempotencyKey, updated.ID); err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("complete idempotency key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RescheduleBookingResult{}, fmt.Errorf("commit admin reschedule: %w", err)
	}
	return RescheduleBookingResult{Booking: updated}, nil
}

// AdminCancelBooking cancels a booking on behalf of an operator. It verifies
// ExpectedVersion under a row lock, records an 'admin' audit event, and
// transactionally cancels the booking's still-pending reminder so it never
// fires for a cancelled appointment.
func (r *PGXRepository) AdminCancelBooking(ctx context.Context, request AdminCancelBookingRequest) (CancelBookingResult, error) {
	if r == nil || r.pool == nil {
		return CancelBookingResult{}, fmt.Errorf("scheduling repository: nil pool")
	}
	if request.BookingID == "" || request.Reason == "" {
		return CancelBookingResult{}, fmt.Errorf("%w: booking_id and reason are required", ErrInvalidInput)
	}
	if request.ExpectedVersion <= 0 {
		return CancelBookingResult{}, fmt.Errorf("%w: expected_version must be positive", ErrInvalidInput)
	}
	if len(request.Reason) > 500 {
		return CancelBookingResult{}, fmt.Errorf("%w: reason exceeds 500 bytes", ErrInvalidInput)
	}
	if length := len(request.IdempotencyKey); length < 16 || length > 128 {
		return CancelBookingResult{}, fmt.Errorf("%w: idempotency key length must be 16..128", ErrInvalidInput)
	}
	actorRef := request.ActorRef
	if actorRef == "" {
		actorRef = "operator"
	}
	var result CancelBookingResult
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		result, err = r.adminCancelBookingOnce(ctx, request, actorRef)
		if err == nil {
			return result, nil
		}
		if !isTransactionRetry(err) || attempt == 3 {
			return CancelBookingResult{}, mapDatabaseError(err)
		}
		select {
		case <-ctx.Done():
			return CancelBookingResult{}, ctx.Err()
		case <-time.After(time.Duration(attempt*10) * time.Millisecond):
		}
	}
	return CancelBookingResult{}, mapDatabaseError(err)
}

func (r *PGXRepository) adminCancelBookingOnce(ctx context.Context, request AdminCancelBookingRequest, actorRef string) (CancelBookingResult, error) {
	// Cancellation is naturally idempotent: an already-cancelled booking short
	// circuits below, and the optimistic version check rejects a stale retry
	// against a booking that changed. No idempotency_records row is needed.
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CancelBookingResult{}, fmt.Errorf("begin admin cancel transaction: %w", err)
	}
	defer rollbackTx(tx)

	current, err := loadBookingForUpdate(ctx, tx, r.tenantID, request.BookingID)
	if err != nil {
		return CancelBookingResult{}, err
	}
	if current.Status == "cancelled" {
		// Cancelling an already-cancelled booking is idempotent; the desired end
		// state is reached regardless of the expected version.
		if err := tx.Commit(ctx); err != nil {
			return CancelBookingResult{}, fmt.Errorf("commit already-cancelled: %w", err)
		}
		return CancelBookingResult{Booking: current, Replayed: true}, nil
	}
	if current.ScheduleVersion != request.ExpectedVersion {
		return CancelBookingResult{}, ErrScheduleVersionConflict
	}
	if current.Status != "confirmed" {
		return CancelBookingResult{}, ErrBookingStateConflict
	}

	var cancelled Booking
	err = tx.QueryRow(ctx, `
		UPDATE bookings
		SET status = 'cancelled', cancellation_reason = $1, cancelled_at = now(),
		    schedule_version = schedule_version + 1, updated_at = now()
		WHERE tenant_id = $2 AND id = $3 AND status = 'confirmed'
		RETURNING id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		          service_id::text, staff_id::text, status, starts_at, ends_at,
		          buffer_before_minutes, buffer_after_minutes, schedule_version,
		          notes, created_at, updated_at`,
		request.Reason, r.tenantID, request.BookingID).
		Scan(&cancelled.ID, &cancelled.CustomerID, &cancelled.ConversationID, &cancelled.ServiceID, &cancelled.StaffID,
			&cancelled.Status, &cancelled.StartsAt, &cancelled.EndsAt, &cancelled.BufferBeforeMinutes,
			&cancelled.BufferAfterMinutes, &cancelled.ScheduleVersion, &cancelled.Notes, &cancelled.CreatedAt, &cancelled.UpdatedAt)
	if err != nil {
		return CancelBookingResult{}, fmt.Errorf("cancel booking: %w", err)
	}

	fromState, _ := json.Marshal(current)
	toState, _ := json.Marshal(cancelled)
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_events (tenant_id, booking_id, event_type, actor_type, actor_ref, from_state, to_state)
		VALUES ($1, $2, 'cancelled', $3, $4, $5::jsonb, $6::jsonb)`,
		r.tenantID, cancelled.ID, actorTypeAdmin, actorRef, string(fromState), string(toState)); err != nil {
		return CancelBookingResult{}, fmt.Errorf("insert cancel event: %w", err)
	}
	if _, err := jobqueue.CancelBookingJobs(ctx, tx, r.tenantID, cancelled.ID, "send_reminder"); err != nil {
		return CancelBookingResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return CancelBookingResult{}, fmt.Errorf("commit admin cancel: %w", err)
	}
	return CancelBookingResult{Booking: cancelled}, nil
}

func hashAdminCreateBooking(request AdminCreateBookingRequest) (string, error) {
	canonical := struct {
		CustomerID string `json:"customer_id"`
		ServiceID  string `json:"service_id"`
		StaffID    string `json:"staff_id"`
		StartsAt   string `json:"starts_at"`
		Notes      string `json:"notes,omitempty"`
	}{
		CustomerID: request.CustomerID, ServiceID: request.ServiceID, StaffID: request.StaffID,
		StartsAt: request.StartsAt.UTC().Format(time.RFC3339Nano), Notes: request.Notes,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode idempotency request: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// loadBookingForUpdate loads a booking row locked FOR UPDATE, scoped only by
// tenant: operators may act on any booking in the tenant, unlike the
// customer path which additionally scopes by owner.
func loadBookingForUpdate(ctx context.Context, tx pgx.Tx, tenantID, bookingID string) (Booking, error) {
	var booking Booking
	err := tx.QueryRow(ctx, `
		SELECT id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		       service_id::text, staff_id::text, status, starts_at, ends_at,
		       buffer_before_minutes, buffer_after_minutes, schedule_version,
		       notes, created_at, updated_at
		FROM bookings
		WHERE tenant_id = $1 AND id = $2
		FOR UPDATE`, tenantID, bookingID).
		Scan(&booking.ID, &booking.CustomerID, &booking.ConversationID, &booking.ServiceID, &booking.StaffID,
			&booking.Status, &booking.StartsAt, &booking.EndsAt, &booking.BufferBeforeMinutes,
			&booking.BufferAfterMinutes, &booking.ScheduleVersion, &booking.Notes, &booking.CreatedAt, &booking.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, fmt.Errorf("load booking for update: %w", err)
	}
	return booking, nil
}

func (r *PGXRepository) acquireScheduleLocks(ctx context.Context, tx pgx.Tx, staffID string, localDates []string) error {
	// touchedLocalDates is ascending. All writers use that order, preventing a
	// cross-midnight booking from deadlocking another writer on the next day.
	for _, localDate := range localDates {
		if _, err := tx.Exec(ctx, `
			INSERT INTO schedule_locks (tenant_id, staff_id, local_date)
			VALUES ($1, $2, $3::date)
			ON CONFLICT (tenant_id, staff_id, local_date) DO NOTHING`, r.tenantID, staffID, localDate); err != nil {
			return fmt.Errorf("materialize schedule lock: %w", err)
		}
	}
	for _, localDate := range localDates {
		var lockedDate string
		if err := tx.QueryRow(ctx, `
			SELECT local_date::text
			FROM schedule_locks
			WHERE tenant_id = $1 AND staff_id = $2 AND local_date = $3::date
			FOR UPDATE`, r.tenantID, staffID, localDate).Scan(&lockedDate); err != nil {
			return fmt.Errorf("lock staff schedule: %w", err)
		}
	}
	return nil
}

func (r *PGXRepository) loadBookingInputs(ctx context.Context, db queryer, serviceID, staffID string) (Service, Staff, error) {
	service, err := loadService(ctx, db, r.tenantID, serviceID)
	if err != nil {
		return Service{}, Staff{}, err
	}
	staff, err := listStaff(ctx, db, r.tenantID, serviceID, staffID)
	if err != nil {
		return Service{}, Staff{}, err
	}
	if len(staff) != 1 {
		return Service{}, Staff{}, fmt.Errorf("%w: staff is inactive or cannot perform service", ErrNotFound)
	}
	return service, staff[0], nil
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type scanner interface {
	Scan(...any) error
}

func loadService(ctx context.Context, db queryer, tenantID, serviceID string) (Service, error) {
	service, err := scanService(db.QueryRow(ctx, `
		SELECT id::text, slug, name, description, duration_minutes,
		       buffer_before_minutes, buffer_after_minutes, price_minor, currency
		FROM services
		WHERE tenant_id = $1 AND id = $2 AND active`, tenantID, serviceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Service{}, fmt.Errorf("%w: service", ErrNotFound)
	}
	if err != nil {
		return Service{}, fmt.Errorf("load service: %w", err)
	}
	return service, nil
}

func scanService(row scanner) (Service, error) {
	var service Service
	var duration, before, after int
	err := row.Scan(&service.ID, &service.Slug, &service.Name, &service.Description, &duration,
		&before, &after, &service.PriceMinor, &service.Currency)
	service.Duration = time.Duration(duration) * time.Minute
	service.BufferBefore = time.Duration(before) * time.Minute
	service.BufferAfter = time.Duration(after) * time.Minute
	service.Currency = strings.TrimSpace(service.Currency)
	return service, err
}

func listStaff(ctx context.Context, db queryer, tenantID, serviceID, staffID string) ([]Staff, error) {
	rows, err := db.Query(ctx, `
		SELECT s.id::text, s.slug, s.display_name, s.timezone
		FROM staff s
		JOIN staff_services ss
		  ON ss.tenant_id = s.tenant_id AND ss.staff_id = s.id
		JOIN services svc
		  ON svc.tenant_id = ss.tenant_id AND svc.id = ss.service_id
		WHERE s.tenant_id = $1 AND ss.service_id = $2
		  AND ($3 = '' OR s.id = NULLIF($3, '')::uuid)
		  AND s.active AND svc.active
		ORDER BY s.display_name, s.id`, tenantID, serviceID, staffID)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}
	defer rows.Close()
	var staff []Staff
	for rows.Next() {
		var member Staff
		if err := rows.Scan(&member.ID, &member.Slug, &member.DisplayName, &member.Timezone); err != nil {
			return nil, fmt.Errorf("scan staff: %w", err)
		}
		staff = append(staff, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list staff rows: %w", err)
	}
	return staff, nil
}

func loadRules(ctx context.Context, db queryer, tenantID, staffID string) ([]AvailabilityRule, error) {
	rows, err := db.Query(ctx, `
		SELECT rule_type, day_of_week,
		       (extract(hour FROM local_start)::integer * 60 + extract(minute FROM local_start)::integer),
		       (extract(hour FROM local_end)::integer * 60 + extract(minute FROM local_end)::integer),
		       COALESCE(valid_from::text, ''), COALESCE(valid_until::text, '')
		FROM availability_rules
		WHERE tenant_id = $1 AND staff_id = $2
		ORDER BY day_of_week, local_start, rule_type`, tenantID, staffID)
	if err != nil {
		return nil, fmt.Errorf("load availability rules: %w", err)
	}
	defer rows.Close()
	var rules []AvailabilityRule
	for rows.Next() {
		var rule AvailabilityRule
		var kind, validFrom, validUntil string
		var weekday int
		if err := rows.Scan(&kind, &weekday, &rule.StartMinute, &rule.EndMinute, &validFrom, &validUntil); err != nil {
			return nil, fmt.Errorf("scan availability rule: %w", err)
		}
		rule.Kind = RuleKind(kind)
		rule.Weekday = time.Weekday(weekday)
		if validFrom != "" {
			value, parseErr := time.Parse("2006-01-02", validFrom)
			if parseErr != nil {
				return nil, fmt.Errorf("parse availability valid_from: %w", parseErr)
			}
			rule.ValidFrom = &value
		}
		if validUntil != "" {
			value, parseErr := time.Parse("2006-01-02", validUntil)
			if parseErr != nil {
				return nil, fmt.Errorf("parse availability valid_until: %w", parseErr)
			}
			rule.ValidUntil = &value
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("availability rule rows: %w", err)
	}
	return rules, nil
}

func loadBusy(ctx context.Context, db queryer, tenantID, staffID string, from, to time.Time) ([]Interval, error) {
	queryFrom := from.Add(-4 * time.Hour)
	queryTo := to.Add(4 * time.Hour)
	rows, err := db.Query(ctx, `
		SELECT starts_at, ends_at, 0, 0
		FROM schedule_blocks
		WHERE tenant_id = $1 AND staff_id = $2
		  AND starts_at < $4 AND ends_at > $3
		UNION ALL
		SELECT starts_at, ends_at, buffer_before_minutes, buffer_after_minutes
		FROM bookings
		WHERE tenant_id = $1 AND staff_id = $2
		  AND status IN ('confirmed', 'checked_in', 'completed', 'no_show')
		  AND occupied_range && tstzrange($3, $4, '[)')`, tenantID, staffID, queryFrom, queryTo)
	if err != nil {
		return nil, fmt.Errorf("load busy intervals: %w", err)
	}
	defer rows.Close()
	var busy []Interval
	for rows.Next() {
		var item Interval
		var before, after int
		if err := rows.Scan(&item.Start, &item.End, &before, &after); err != nil {
			return nil, fmt.Errorf("scan busy interval: %w", err)
		}
		item.BufferBefore = time.Duration(before) * time.Minute
		item.BufferAfter = time.Duration(after) * time.Minute
		busy = append(busy, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("busy interval rows: %w", err)
	}
	return busy, nil
}

func loadBooking(ctx context.Context, db queryer, tenantID, bookingID string) (Booking, error) {
	var booking Booking
	err := db.QueryRow(ctx, `
		SELECT id::text, customer_id::text, COALESCE(conversation_id::text, ''),
		       service_id::text, staff_id::text, status, starts_at, ends_at,
		       buffer_before_minutes, buffer_after_minutes, schedule_version,
		       notes, created_at, updated_at
		FROM bookings
		WHERE tenant_id = $1 AND id = $2`, tenantID, bookingID).
		Scan(&booking.ID, &booking.CustomerID, &booking.ConversationID, &booking.ServiceID, &booking.StaffID,
			&booking.Status, &booking.StartsAt, &booking.EndsAt, &booking.BufferBeforeMinutes,
			&booking.BufferAfterMinutes, &booking.ScheduleVersion, &booking.Notes, &booking.CreatedAt, &booking.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, fmt.Errorf("%w: booking", ErrNotFound)
	}
	if err != nil {
		return Booking{}, fmt.Errorf("load booking: %w", err)
	}
	return booking, nil
}

func validateCreateBooking(request CreateBookingRequest) error {
	return validateCreateBookingAt(request, time.Now())
}

func validateCreateBookingAt(request CreateBookingRequest, now time.Time) error {
	if request.CustomerID == "" || request.ServiceID == "" || request.StaffID == "" || request.StartsAt.IsZero() {
		return fmt.Errorf("%w: customer, service, staff, and start are required", ErrInvalidInput)
	}
	if !request.StartsAt.After(now) {
		return fmt.Errorf("%w: booking start must be in the future", ErrSlotUnavailable)
	}
	if length := len(request.IdempotencyKey); length < 16 || length > 128 {
		return fmt.Errorf("%w: idempotency key length must be 16..128", ErrInvalidInput)
	}
	if len(request.Notes) > 500 {
		return fmt.Errorf("%w: notes exceed 500 bytes", ErrInvalidInput)
	}
	return nil
}

func hashCreateBooking(request CreateBookingRequest) (string, error) {
	canonical := struct {
		CustomerID     string `json:"customer_id"`
		ConversationID string `json:"conversation_id,omitempty"`
		ServiceID      string `json:"service_id"`
		StaffID        string `json:"staff_id"`
		StartsAt       string `json:"starts_at"`
		Notes          string `json:"notes,omitempty"`
	}{
		CustomerID: request.CustomerID, ConversationID: request.ConversationID,
		ServiceID: request.ServiceID, StaffID: request.StaffID,
		StartsAt: request.StartsAt.UTC().Format(time.RFC3339Nano), Notes: request.Notes,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode idempotency request: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func minutes(duration time.Duration) int {
	return int(duration / time.Minute)
}

func touchedLocalDates(start, end time.Time, location *time.Location) []string {
	if !start.Before(end) {
		return nil
	}
	localStart := start.In(location)
	localLast := end.Add(-time.Nanosecond).In(location)
	day := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, location)
	last := time.Date(localLast.Year(), localLast.Month(), localLast.Day(), 0, 0, 0, 0, location)
	var result []string
	for !day.After(last) {
		result = append(result, day.Format("2006-01-02"))
		day = day.AddDate(0, 0, 1)
	}
	return result
}

func rollbackTx(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func isTransactionRetry(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}

func mapDatabaseError(err error) error {
	if errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrSlotUnavailable) || errors.Is(err, ErrIdempotencyConflict) ||
		errors.Is(err, ErrBookingStateConflict) || errors.Is(err, ErrScheduleVersionConflict) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23P01":
			return fmt.Errorf("%w: concurrent booking won the slot", ErrSlotUnavailable)
		case "23503":
			return fmt.Errorf("%w: referenced scheduling resource", ErrNotFound)
		}
	}
	return err
}

// StableStaffOrder is useful to callers combining independently sourced slot
// groups; repository results already use this ordering.
func StableStaffOrder(staff []Staff) {
	sort.Slice(staff, func(i, j int) bool {
		if staff[i].DisplayName == staff[j].DisplayName {
			return staff[i].ID < staff[j].ID
		}
		return staff[i].DisplayName < staff[j].DisplayName
	})
}
