// Package operatorhttp exposes the Stage 5 operator read API. It is scoped to
// the configured fixed tenant and is mounted only when an explicit admin token
// is configured; Stage 6 replaces that token with operator identities.
package operatorhttp

import (
	"context"
	"errors"
	"time"

	"github.com/reinhlord/kontor/internal/agenttrace"
)

type Session struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name"`
	Timezone   string `json:"timezone"`
	Currency   string `json:"currency"`
}

type DashboardRequest struct {
	Days int
}

type Dashboard struct {
	GeneratedAt   time.Time            `json:"generated_at"`
	PeriodDays    int                  `json:"period_days"`
	KPIs          DashboardKPIs        `json:"kpis"`
	BookingSeries []BookingSeriesPoint `json:"booking_series"`
	RunOutcomes   []RunOutcomeCount    `json:"run_outcomes"`
	RecentRuns    []RunSummary         `json:"recent_runs"`
	Attention     []AttentionItem      `json:"attention"`
}

type DashboardKPIs struct {
	BookingsToday   int64   `json:"bookings_today"`
	TotalRuns       int64   `json:"total_runs"`
	SuccessRate     float64 `json:"success_rate"`
	MedianLatencyMS int64   `json:"median_latency_ms"`
	OpenEscalations int64   `json:"open_escalations"`
	TotalTokens     int64   `json:"total_tokens"`
}

type BookingSeriesPoint struct {
	Date    string `json:"date"`
	Channel string `json:"channel"`
	Count   int64  `json:"count"`
}

type RunOutcomeCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type AttentionItem struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	RunID     string    `json:"run_id,omitempty"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ListRunsRequest struct {
	Limit   int
	Cursor  string
	Status  string
	Channel string
	Query   string
	From    *time.Time
	To      *time.Time
}

type RunPage struct {
	Items      []RunSummary `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

type RunSummary struct {
	ID               string     `json:"id"`
	ConversationID   string     `json:"conversation_id"`
	CustomerName     string     `json:"customer_name"`
	Channel          string     `json:"channel"`
	Status           string     `json:"status"`
	DurationMS       *int       `json:"duration_ms"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	TotalTokens      int        `json:"total_tokens"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

type RunDetail struct {
	Run                agenttrace.RunTrace `json:"run"`
	Customer           Customer            `json:"customer"`
	Channel            string              `json:"channel"`
	ConversationStatus string              `json:"conversation_status"`
	Messages           []Message           `json:"messages"`
	MessagesTruncated  bool                `json:"messages_truncated"`
	Bookings           []Booking           `json:"bookings"`
	Escalation         *Escalation         `json:"escalation,omitempty"`
}

type Customer struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
}

type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Escalation struct {
	ID         string    `json:"id"`
	ReasonCode string    `json:"reason_code"`
	Summary    string    `json:"summary"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type CalendarRequest struct {
	From time.Time
	To   time.Time
}

type Calendar struct {
	From     string          `json:"from"`
	To       string          `json:"to"`
	Timezone string          `json:"timezone"`
	Staff    []Staff         `json:"staff"`
	Services []Service       `json:"services"`
	Bookings []Booking       `json:"bookings"`
	Blocks   []ScheduleBlock `json:"blocks"`
}

type Staff struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}

type Service struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	DurationMinutes int    `json:"duration_minutes"`
	PriceMinor      int64  `json:"price_minor"`
	Currency        string `json:"currency"`
}

type Booking struct {
	ID              string    `json:"id"`
	CustomerID      string    `json:"customer_id"`
	CustomerName    string    `json:"customer_name"`
	CustomerEmail   string    `json:"customer_email,omitempty"`
	CustomerPhone   string    `json:"customer_phone,omitempty"`
	ConversationID  string    `json:"conversation_id,omitempty"`
	ServiceID       string    `json:"service_id"`
	ServiceName     string    `json:"service_name"`
	StaffID         string    `json:"staff_id"`
	StaffName       string    `json:"staff_name"`
	Status          string    `json:"status"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	ScheduleVersion int       `json:"schedule_version"`
	Notes           string    `json:"notes,omitempty"`
}

type ScheduleBlock struct {
	ID       string    `json:"id"`
	StaffID  string    `json:"staff_id"`
	Kind     string    `json:"kind"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Note     string    `json:"note,omitempty"`
}

// CreateBookingCommand is the operator-supplied input for creating a booking
// directly from the console.
type CreateBookingCommand struct {
	CustomerID     string
	ServiceID      string
	StaffID        string
	StartsAt       time.Time
	Notes          string
	IdempotencyKey string
}

// RescheduleBookingCommand moves a booking to a new start time. ExpectedVersion
// is the optimistic-concurrency guard the operator loaded the booking at.
type RescheduleBookingCommand struct {
	BookingID       string
	ExpectedVersion int
	StartsAt        time.Time
	IdempotencyKey  string
}

// CancelBookingCommand cancels a booking. ExpectedVersion is the optimistic
// guard, as for reschedule.
type CancelBookingCommand struct {
	BookingID       string
	ExpectedVersion int
	Reason          string
	IdempotencyKey  string
}

// Command failures the HTTP layer maps to problem responses. The PostgreSQL
// backend translates scheduling-domain errors into these so the handler never
// depends on the scheduling package.
var (
	ErrInvalidCommand       = errors.New("operator command is invalid")
	ErrBookingNotFound      = errors.New("booking not found")
	ErrVersionConflict      = errors.New("schedule version conflict")
	ErrSlotUnavailable      = errors.New("slot is no longer available")
	ErrBookingStateConflict = errors.New("booking state conflict")
)

// CustomerListRequest searches the tenant's customers for the create-booking
// picker. An empty query returns the first page ordered by name.
type CustomerListRequest struct {
	Query string
	Limit int
}

// CustomerList is the customer picker payload.
type CustomerList struct {
	Items []Customer `json:"items"`
}

// Backend keeps the HTTP/auth boundary testable without PostgreSQL.
type Backend interface {
	Dashboard(context.Context, DashboardRequest) (Dashboard, error)
	ListRuns(context.Context, ListRunsRequest) (RunPage, error)
	GetRun(context.Context, string) (RunDetail, error)
	Calendar(context.Context, CalendarRequest) (Calendar, error)
	ListCustomers(context.Context, CustomerListRequest) (CustomerList, error)
	CreateBooking(context.Context, CreateBookingCommand) (Booking, error)
	RescheduleBooking(context.Context, RescheduleBookingCommand) (Booking, error)
	CancelBooking(context.Context, CancelBookingCommand) (Booking, error)
}
