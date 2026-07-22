package operatorhttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/agenttrace"
	"github.com/reinhlord/kontor/internal/platform/ids"
	"github.com/reinhlord/kontor/internal/scheduling"
)

type traceReader interface {
	GetRun(context.Context, string) (agenttrace.RunTrace, error)
}

// bookingCommander is the narrow admin-write surface the operator console needs
// from the scheduling repository. It is intentionally separate from the
// customer-facing tools.Gateway so operator writes carry an 'admin' audit
// actor, an optimistic version check, and transactional reminder updates.
type bookingCommander interface {
	AdminCreateBooking(context.Context, scheduling.AdminCreateBookingRequest) (scheduling.CreateBookingResult, error)
	AdminRescheduleBooking(context.Context, scheduling.AdminRescheduleBookingRequest) (scheduling.RescheduleBookingResult, error)
	AdminCancelBooking(context.Context, scheduling.AdminCancelBookingRequest) (scheduling.CancelBookingResult, error)
}

// PostgreSQL is the tenant-scoped read model for the operator console, plus the
// admin-write commands that delegate to the scheduling repository.
type PostgreSQL struct {
	pool     *pgxpool.Pool
	trace    traceReader
	commands bookingCommander
	tenantID string
	timezone string
	location *time.Location
	now      func() time.Time
}

func NewPostgreSQL(pool *pgxpool.Pool, trace traceReader, commands bookingCommander, tenantID, timezone string) (*PostgreSQL, error) {
	if pool == nil {
		return nil, errors.New("operator postgres: nil pool")
	}
	if trace == nil {
		return nil, errors.New("operator postgres: nil trace reader")
	}
	if commands == nil {
		return nil, errors.New("operator postgres: nil booking commander")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("operator postgres: invalid timezone %q: %w", timezone, err)
	}
	return &PostgreSQL{
		pool: pool, trace: trace, commands: commands, tenantID: tenantID, timezone: timezone,
		location: location, now: time.Now,
	}, nil
}

func (s *PostgreSQL) Dashboard(ctx context.Context, request DashboardRequest) (Dashboard, error) {
	if request.Days != 7 && request.Days != 30 && request.Days != 90 {
		return Dashboard{}, fmt.Errorf("dashboard period must be 7, 30, or 90 days")
	}
	now := s.now().UTC()
	since := now.AddDate(0, 0, -request.Days)
	result := Dashboard{GeneratedAt: now, PeriodDays: request.Days}

	if err := s.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM bookings
		WHERE tenant_id=$1 AND status <> 'cancelled'
		  AND (starts_at AT TIME ZONE $2)::date = ($3::timestamptz AT TIME ZONE $2)::date`,
		s.tenantID, s.timezone, now).Scan(&result.KPIs.BookingsToday); err != nil {
		return Dashboard{}, fmt.Errorf("dashboard bookings today: %w", err)
	}

	var completedRuns, terminalRuns int64
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status='completed'),
		       count(*) FILTER (WHERE status <> 'running'),
		       COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY duration_ms)
		         FILTER (WHERE duration_ms IS NOT NULL), 0)::bigint,
		       COALESCE(sum(prompt_tokens + completion_tokens), 0)::bigint
		FROM agent_runs
		WHERE tenant_id=$1 AND started_at >= $2`, s.tenantID, since).
		Scan(&result.KPIs.TotalRuns, &completedRuns, &terminalRuns,
			&result.KPIs.MedianLatencyMS, &result.KPIs.TotalTokens); err != nil {
		return Dashboard{}, fmt.Errorf("dashboard run metrics: %w", err)
	}
	if terminalRuns > 0 {
		result.KPIs.SuccessRate = float64(completedRuns) / float64(terminalRuns)
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM escalations
		WHERE tenant_id=$1 AND status IN ('open','claimed')`, s.tenantID).
		Scan(&result.KPIs.OpenEscalations); err != nil {
		return Dashboard{}, fmt.Errorf("dashboard open escalations: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT to_char((b.created_at AT TIME ZONE $2)::date, 'YYYY-MM-DD'),
		       COALESCE(c.channel, 'operator'), count(*)
		FROM bookings b
		LEFT JOIN conversations c
		  ON c.tenant_id=b.tenant_id AND c.id=b.conversation_id
		WHERE b.tenant_id=$1 AND b.created_at >= $3
		GROUP BY 1,2
		ORDER BY 1,2`, s.tenantID, s.timezone, since)
	if err != nil {
		return Dashboard{}, fmt.Errorf("dashboard booking series: %w", err)
	}
	for rows.Next() {
		var item BookingSeriesPoint
		if err := rows.Scan(&item.Date, &item.Channel, &item.Count); err != nil {
			rows.Close()
			return Dashboard{}, fmt.Errorf("scan dashboard booking series: %w", err)
		}
		result.BookingSeries = append(result.BookingSeries, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Dashboard{}, fmt.Errorf("dashboard booking series rows: %w", err)
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, `
		SELECT status, count(*)
		FROM agent_runs
		WHERE tenant_id=$1 AND started_at >= $2
		GROUP BY status ORDER BY status`, s.tenantID, since)
	if err != nil {
		return Dashboard{}, fmt.Errorf("dashboard run outcomes: %w", err)
	}
	for rows.Next() {
		var item RunOutcomeCount
		if err := rows.Scan(&item.Status, &item.Count); err != nil {
			rows.Close()
			return Dashboard{}, fmt.Errorf("scan dashboard run outcome: %w", err)
		}
		result.RunOutcomes = append(result.RunOutcomes, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Dashboard{}, fmt.Errorf("dashboard run outcome rows: %w", err)
	}
	rows.Close()

	recent, err := s.listRunSummaries(ctx, ListRunsRequest{Limit: 8})
	if err != nil {
		return Dashboard{}, err
	}
	result.RecentRuns = recent.Items

	rows, err = s.pool.Query(ctx, `
		SELECT kind,id,run_id,title,summary,created_at
		FROM (
			SELECT 'escalation'::text AS kind,e.id::text AS id,
			       COALESCE(e.agent_run_id::text,'') AS run_id,
			       e.reason_code AS title,e.summary,e.created_at
			FROM escalations e
			WHERE e.tenant_id=$1 AND e.status IN ('open','claimed')
			UNION ALL
			SELECT 'failed_run',ar.id::text,ar.id::text,
			       COALESCE(NULLIF(ar.error_code,''),'run failed'),
			       COALESCE(ar.error_message,''),ar.started_at
			FROM agent_runs ar
			WHERE ar.tenant_id=$1 AND ar.started_at >= $2
			  AND ar.status IN ('failed','budget_exhausted')
			  AND NOT EXISTS (
				SELECT 1 FROM escalations e
				WHERE e.tenant_id=ar.tenant_id AND e.agent_run_id=ar.id
				  AND e.status IN ('open','claimed')
			  )
		) attention
		ORDER BY created_at DESC,id DESC
		LIMIT 8`, s.tenantID, since)
	if err != nil {
		return Dashboard{}, fmt.Errorf("dashboard attention: %w", err)
	}
	for rows.Next() {
		var item AttentionItem
		if err := rows.Scan(&item.Kind, &item.ID, &item.RunID, &item.Title, &item.Summary, &item.CreatedAt); err != nil {
			rows.Close()
			return Dashboard{}, fmt.Errorf("scan dashboard attention: %w", err)
		}
		result.Attention = append(result.Attention, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Dashboard{}, fmt.Errorf("dashboard attention rows: %w", err)
	}
	rows.Close()

	result.BookingSeries = nonNil(result.BookingSeries)
	result.RunOutcomes = nonNil(result.RunOutcomes)
	result.RecentRuns = nonNil(result.RecentRuns)
	result.Attention = nonNil(result.Attention)
	return result, nil
}

func (s *PostgreSQL) ListRuns(ctx context.Context, request ListRunsRequest) (RunPage, error) {
	return s.listRunSummaries(ctx, request)
}

func (s *PostgreSQL) listRunSummaries(ctx context.Context, request ListRunsRequest) (RunPage, error) {
	if request.Limit <= 0 {
		request.Limit = 50
	}
	if request.Limit > 100 {
		request.Limit = 100
	}
	var cursorAt *time.Time
	var cursorID string
	if request.Cursor != "" {
		cursor, err := decodeRunCursor(request.Cursor)
		if err != nil {
			return RunPage{}, err
		}
		cursorAt = &cursor.StartedAt
		cursorID = cursor.ID
	}
	query := strings.TrimSpace(request.Query)
	rows, err := s.pool.Query(ctx, `
		SELECT ar.id::text,ar.conversation_id::text,
		       COALESCE(NULLIF(cu.display_name,''),'Unknown customer'),c.channel,
		       ar.status,ar.duration_ms,ar.prompt_tokens,ar.completion_tokens,
		       ar.started_at,ar.finished_at
		FROM agent_runs ar
		JOIN conversations c
		  ON c.tenant_id=ar.tenant_id AND c.id=ar.conversation_id
		LEFT JOIN customers cu
		  ON cu.tenant_id=c.tenant_id AND cu.id=c.customer_id
		WHERE ar.tenant_id=$1
		  AND ($2='' OR ar.status=$2)
		  AND ($3='' OR c.channel=$3)
		  AND ($4='' OR ar.id::text ILIKE '%' || $4 || '%'
		       OR COALESCE(cu.display_name,'') ILIKE '%' || $4 || '%')
		  AND ($5::timestamptz IS NULL OR ar.started_at >= $5)
		  AND ($6::timestamptz IS NULL OR ar.started_at < $6)
		  AND ($7::timestamptz IS NULL OR ar.started_at < $7
		       OR (ar.started_at=$7 AND ar.id < NULLIF($8,'')::uuid))
		ORDER BY ar.started_at DESC,ar.id DESC
		LIMIT $9`, s.tenantID, request.Status, request.Channel, query,
		request.From, request.To, cursorAt, cursorID, request.Limit+1)
	if err != nil {
		return RunPage{}, fmt.Errorf("list operator runs: %w", err)
	}
	defer rows.Close()
	page := RunPage{}
	for rows.Next() {
		var item RunSummary
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.CustomerName, &item.Channel,
			&item.Status, &item.DurationMS, &item.PromptTokens, &item.CompletionTokens,
			&item.StartedAt, &item.FinishedAt); err != nil {
			return RunPage{}, fmt.Errorf("scan operator run: %w", err)
		}
		item.TotalTokens = item.PromptTokens + item.CompletionTokens
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return RunPage{}, fmt.Errorf("operator run rows: %w", err)
	}
	if len(page.Items) > request.Limit {
		page.Items = page.Items[:request.Limit]
		page.NextCursor = encodeRunCursor(runCursor{
			StartedAt: page.Items[len(page.Items)-1].StartedAt,
			ID:        page.Items[len(page.Items)-1].ID,
		})
	}
	page.Items = nonNil(page.Items)
	return page, nil
}

func (s *PostgreSQL) GetRun(ctx context.Context, runID string) (RunDetail, error) {
	run, err := s.trace.GetRun(ctx, runID)
	if err != nil {
		return RunDetail{}, err
	}
	result := RunDetail{Run: run}
	err = s.pool.QueryRow(ctx, `
		SELECT c.channel,c.status,COALESCE(cu.id::text,''),
		       COALESCE(cu.display_name,'Unknown customer'),COALESCE(cu.email,''),COALESCE(cu.phone,'')
		FROM conversations c
		LEFT JOIN customers cu
		  ON cu.tenant_id=c.tenant_id AND cu.id=c.customer_id
		WHERE c.tenant_id=$1 AND c.id=$2`, s.tenantID, run.ConversationID).
		Scan(&result.Channel, &result.ConversationStatus, &result.Customer.ID,
			&result.Customer.DisplayName, &result.Customer.Email, &result.Customer.Phone)
	if err != nil {
		return RunDetail{}, fmt.Errorf("load operator run conversation: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id,role,content,created_at
		FROM (
			SELECT id::text AS id,role,content,created_at
			FROM messages
			WHERE tenant_id=$1 AND conversation_id=$2
			ORDER BY created_at DESC,id DESC
			LIMIT 501
		) recent
		ORDER BY created_at,id`, s.tenantID, run.ConversationID)
	if err != nil {
		return RunDetail{}, fmt.Errorf("load operator run messages: %w", err)
	}
	for rows.Next() {
		var item Message
		if err := rows.Scan(&item.ID, &item.Role, &item.Content, &item.CreatedAt); err != nil {
			rows.Close()
			return RunDetail{}, fmt.Errorf("scan operator run message: %w", err)
		}
		result.Messages = append(result.Messages, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return RunDetail{}, fmt.Errorf("operator message rows: %w", err)
	}
	rows.Close()
	if len(result.Messages) > 500 {
		result.Messages = result.Messages[len(result.Messages)-500:]
		result.MessagesTruncated = true
	}

	result.Bookings, err = s.listBookings(ctx, `b.conversation_id=$2`, run.ConversationID)
	if err != nil {
		return RunDetail{}, err
	}
	var escalation Escalation
	err = s.pool.QueryRow(ctx, `
		SELECT id::text,reason_code,summary,status,created_at
		FROM escalations
		WHERE tenant_id=$1 AND agent_run_id=$2
		ORDER BY created_at DESC,id DESC LIMIT 1`, s.tenantID, runID).
		Scan(&escalation.ID, &escalation.ReasonCode, &escalation.Summary, &escalation.Status, &escalation.CreatedAt)
	if err == nil {
		result.Escalation = &escalation
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return RunDetail{}, fmt.Errorf("load operator run escalation: %w", err)
	}
	result.Messages = nonNil(result.Messages)
	result.Bookings = nonNil(result.Bookings)
	return result, nil
}

func (s *PostgreSQL) Calendar(ctx context.Context, request CalendarRequest) (Calendar, error) {
	if request.From.IsZero() || request.To.IsZero() || !request.From.Before(request.To) {
		return Calendar{}, errors.New("calendar requires a positive range")
	}
	result := Calendar{
		From:     request.From.In(s.location).Format("2006-01-02"),
		To:       request.To.In(s.location).Format("2006-01-02"),
		Timezone: s.timezone,
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id::text,display_name,timezone
		FROM staff WHERE tenant_id=$1 AND active
		ORDER BY display_name,id`, s.tenantID)
	if err != nil {
		return Calendar{}, fmt.Errorf("list operator calendar staff: %w", err)
	}
	for rows.Next() {
		var item Staff
		if err := rows.Scan(&item.ID, &item.DisplayName, &item.Timezone); err != nil {
			rows.Close()
			return Calendar{}, fmt.Errorf("scan operator calendar staff: %w", err)
		}
		result.Staff = append(result.Staff, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Calendar{}, fmt.Errorf("operator calendar staff rows: %w", err)
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, `
		SELECT id::text,name,duration_minutes,price_minor,currency
		FROM services WHERE tenant_id=$1 AND active
		ORDER BY name,id`, s.tenantID)
	if err != nil {
		return Calendar{}, fmt.Errorf("list operator calendar services: %w", err)
	}
	for rows.Next() {
		var item Service
		if err := rows.Scan(&item.ID, &item.Name, &item.DurationMinutes, &item.PriceMinor, &item.Currency); err != nil {
			rows.Close()
			return Calendar{}, fmt.Errorf("scan operator calendar service: %w", err)
		}
		item.Currency = strings.TrimSpace(item.Currency)
		result.Services = append(result.Services, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Calendar{}, fmt.Errorf("operator calendar service rows: %w", err)
	}
	rows.Close()

	result.Bookings, err = s.listBookings(ctx, `b.status <> 'cancelled' AND b.starts_at < $2 AND b.ends_at > $3`, request.To, request.From)
	if err != nil {
		return Calendar{}, err
	}
	rows, err = s.pool.Query(ctx, `
		SELECT id::text,staff_id::text,kind,starts_at,ends_at,note
		FROM schedule_blocks
		WHERE tenant_id=$1 AND starts_at < $2 AND ends_at > $3
		ORDER BY starts_at,id`, s.tenantID, request.To, request.From)
	if err != nil {
		return Calendar{}, fmt.Errorf("list operator calendar blocks: %w", err)
	}
	for rows.Next() {
		var item ScheduleBlock
		if err := rows.Scan(&item.ID, &item.StaffID, &item.Kind, &item.StartsAt, &item.EndsAt, &item.Note); err != nil {
			rows.Close()
			return Calendar{}, fmt.Errorf("scan operator calendar block: %w", err)
		}
		result.Blocks = append(result.Blocks, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Calendar{}, fmt.Errorf("operator calendar block rows: %w", err)
	}
	rows.Close()

	result.Staff = nonNil(result.Staff)
	result.Services = nonNil(result.Services)
	result.Bookings = nonNil(result.Bookings)
	result.Blocks = nonNil(result.Blocks)
	return result, nil
}

func (s *PostgreSQL) listBookings(ctx context.Context, condition string, arguments ...any) ([]Booking, error) {
	query := `
		SELECT b.id::text,b.customer_id::text,cu.display_name,COALESCE(cu.email,''),COALESCE(cu.phone,''),
		       COALESCE(b.conversation_id::text,''),b.service_id::text,svc.name,
		       b.staff_id::text,st.display_name,b.status,b.starts_at,b.ends_at,
		       b.schedule_version,b.notes
		FROM bookings b
		JOIN customers cu ON cu.tenant_id=b.tenant_id AND cu.id=b.customer_id
		JOIN services svc ON svc.tenant_id=b.tenant_id AND svc.id=b.service_id
		JOIN staff st ON st.tenant_id=b.tenant_id AND st.id=b.staff_id
		WHERE b.tenant_id=$1 AND ` + condition + `
		ORDER BY b.starts_at,b.id`
	args := make([]any, 0, len(arguments)+1)
	args = append(args, s.tenantID)
	args = append(args, arguments...)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list operator bookings: %w", err)
	}
	defer rows.Close()
	var result []Booking
	for rows.Next() {
		var item Booking
		if err := rows.Scan(&item.ID, &item.CustomerID, &item.CustomerName, &item.CustomerEmail,
			&item.CustomerPhone, &item.ConversationID, &item.ServiceID, &item.ServiceName,
			&item.StaffID, &item.StaffName, &item.Status, &item.StartsAt, &item.EndsAt,
			&item.ScheduleVersion, &item.Notes); err != nil {
			return nil, fmt.Errorf("scan operator booking: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("operator booking rows: %w", err)
	}
	return nonNil(result), nil
}

type runCursor struct {
	StartedAt time.Time `json:"started_at"`
	ID        string    `json:"id"`
}

func encodeRunCursor(cursor runCursor) string {
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeRunCursor(encoded string) (runCursor, error) {
	if len(encoded) > 1024 {
		return runCursor{}, errors.New("run cursor is too long")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return runCursor{}, errors.New("run cursor is invalid")
	}
	var cursor runCursor
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil || cursor.StartedAt.IsZero() || !validUUID(cursor.ID) {
		return runCursor{}, errors.New("run cursor is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return runCursor{}, errors.New("run cursor is invalid")
	}
	return cursor, nil
}

// ListCustomers backs the operator console's create-booking customer picker.
// An empty query returns the first page of customers ordered by name.
func (s *PostgreSQL) ListCustomers(ctx context.Context, request CustomerListRequest) (CustomerList, error) {
	limit := request.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	query := strings.TrimSpace(request.Query)
	rows, err := s.pool.Query(ctx, `
		SELECT id::text,display_name,COALESCE(email,''),COALESCE(phone,'')
		FROM customers
		WHERE tenant_id=$1
		  AND ($2='' OR display_name ILIKE '%'||$2||'%'
		       OR email ILIKE '%'||$2||'%' OR phone ILIKE '%'||$2||'%')
		ORDER BY display_name,id
		LIMIT $3`, s.tenantID, query, limit)
	if err != nil {
		return CustomerList{}, fmt.Errorf("list operator customers: %w", err)
	}
	defer rows.Close()
	var result CustomerList
	for rows.Next() {
		var item Customer
		if err := rows.Scan(&item.ID, &item.DisplayName, &item.Email, &item.Phone); err != nil {
			return CustomerList{}, fmt.Errorf("scan operator customer: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return CustomerList{}, fmt.Errorf("operator customer rows: %w", err)
	}
	result.Items = nonNil(result.Items)
	return result, nil
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

// CreateBooking places an operator-created booking and returns the fully
// joined calendar projection. A missing idempotency key is generated so a
// single logical create is safe to retry at the transport layer.
func (s *PostgreSQL) CreateBooking(ctx context.Context, command CreateBookingCommand) (Booking, error) {
	result, err := s.commands.AdminCreateBooking(ctx, scheduling.AdminCreateBookingRequest{
		CustomerID:     command.CustomerID,
		ServiceID:      command.ServiceID,
		StaffID:        command.StaffID,
		StartsAt:       command.StartsAt,
		Notes:          command.Notes,
		ActorRef:       "operator",
		IdempotencyKey: orGeneratedKey(command.IdempotencyKey),
	})
	if err != nil {
		return Booking{}, mapCommandError(err)
	}
	return s.bookingByID(ctx, result.Booking.ID)
}

// RescheduleBooking moves a booking, enforcing the optimistic version the
// operator loaded it at.
func (s *PostgreSQL) RescheduleBooking(ctx context.Context, command RescheduleBookingCommand) (Booking, error) {
	result, err := s.commands.AdminRescheduleBooking(ctx, scheduling.AdminRescheduleBookingRequest{
		BookingID:       command.BookingID,
		ExpectedVersion: command.ExpectedVersion,
		NewStartsAt:     command.StartsAt,
		ActorRef:        "operator",
		IdempotencyKey:  orGeneratedKey(command.IdempotencyKey),
	})
	if err != nil {
		return Booking{}, mapCommandError(err)
	}
	return s.bookingByID(ctx, result.Booking.ID)
}

// CancelBooking cancels a booking, enforcing the optimistic version the
// operator loaded it at.
func (s *PostgreSQL) CancelBooking(ctx context.Context, command CancelBookingCommand) (Booking, error) {
	result, err := s.commands.AdminCancelBooking(ctx, scheduling.AdminCancelBookingRequest{
		BookingID:       command.BookingID,
		ExpectedVersion: command.ExpectedVersion,
		Reason:          command.Reason,
		ActorRef:        "operator",
		IdempotencyKey:  orGeneratedKey(command.IdempotencyKey),
	})
	if err != nil {
		return Booking{}, mapCommandError(err)
	}
	return s.bookingByID(ctx, result.Booking.ID)
}

func (s *PostgreSQL) bookingByID(ctx context.Context, bookingID string) (Booking, error) {
	bookings, err := s.listBookings(ctx, `b.id = $2`, bookingID)
	if err != nil {
		return Booking{}, err
	}
	if len(bookings) == 0 {
		return Booking{}, ErrBookingNotFound
	}
	return bookings[0], nil
}

func orGeneratedKey(key string) string {
	if key != "" {
		return key
	}
	return "op-" + ids.New()
}

// mapCommandError translates scheduling-domain errors into the operatorhttp
// sentinels the HTTP layer maps to problem responses. Unknown errors are
// returned unchanged and surface as an internal error.
func mapCommandError(err error) error {
	switch {
	case errors.Is(err, scheduling.ErrScheduleVersionConflict):
		return ErrVersionConflict
	case errors.Is(err, scheduling.ErrNotFound):
		return ErrBookingNotFound
	case errors.Is(err, scheduling.ErrSlotUnavailable):
		return ErrSlotUnavailable
	case errors.Is(err, scheduling.ErrBookingStateConflict):
		return ErrBookingStateConflict
	case errors.Is(err, scheduling.ErrIdempotencyConflict):
		return ErrBookingStateConflict
	case errors.Is(err, scheduling.ErrInvalidInput):
		return ErrInvalidCommand
	default:
		return err
	}
}
