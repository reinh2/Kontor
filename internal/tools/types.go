// Package tools implements the model-facing, policy-enforcing tool gateway.
// Model supplied arguments are deliberately kept separate from TrustedContext.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	ContractVersion   = "1.0.0"
	ResultSchemaV1    = "kontor.tool-result.v1"
	ToolListServices  = "list_services"
	ToolListStaff     = "list_staff"
	ToolFindSlots     = "find_slots"
	ToolCreateBooking = "create_booking"
	ToolReschedule    = "reschedule_booking"
	ToolCancel        = "cancel_booking"
	ToolUpsertContact = "upsert_crm_contact"
	ToolCreateDeal    = "create_deal"
	ToolEscalate      = "escalate_to_human"
)

type Capability string

const (
	CapabilityScheduleRead         Capability = "schedule:read"
	CapabilityBookingCreateSelf    Capability = "booking:create:self"
	CapabilityBookingWriteSelf     Capability = "booking:write:self"
	CapabilityCRMContactSelf       Capability = "crm:contact:self"
	CapabilityCRMDealAfterBook     Capability = "crm:deal:after_booking"
	CapabilityConversationEscalate Capability = "conversation:escalate:self"
)

// TrustedContext is created from the authenticated channel/session. None of
// these values may be copied from LLM arguments.
type TrustedContext struct {
	TenantID         string
	CustomerID       string
	ConversationID   string
	InboundMessageID string
	Capabilities     map[Capability]bool
}

func (c TrustedContext) Allows(capability Capability) bool {
	return c.Capabilities != nil && c.Capabilities[capability]
}

func (c TrustedContext) valid() bool {
	return c.TenantID != "" && c.CustomerID != "" && c.ConversationID != ""
}

// Call is the provider-neutral representation of one LLM function call.
type Call struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Definition is suitable for conversion to an OpenAI/OpenRouter function
// declaration. Parameters is a complete Draft 2020-12 JSON Schema.
type Definition struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ResultStatus string

const (
	StatusSuccess              ResultStatus = "success"
	StatusConfirmationRequired ResultStatus = "confirmation_required"
	StatusError                ResultStatus = "error"
)

type Result struct {
	SchemaVersion   string                `json:"schema_version"`
	Tool            string                `json:"tool"`
	ContractVersion string                `json:"contract_version"`
	CallID          string                `json:"call_id"`
	Status          ResultStatus          `json:"status"`
	Data            any                   `json:"data,omitempty"`
	Confirmation    *ConfirmationProposal `json:"confirmation,omitempty"`
	Error           *ToolError            `json:"error,omitempty"`
	Meta            ResultMeta            `json:"meta"`
}

type ResultMeta struct {
	DurationMS          int64 `json:"duration_ms"`
	IdempotencyReplayed bool  `json:"idempotency_replayed"`
}

type ErrorCode string

const (
	CodeInvalidArgument       ErrorCode = "INVALID_ARGUMENT"
	CodeToolNotAllowed        ErrorCode = "TOOL_NOT_ALLOWED"
	CodePolicyDenied          ErrorCode = "POLICY_DENIED"
	CodeNotFoundOrNotOwned    ErrorCode = "NOT_FOUND_OR_NOT_OWNED"
	CodeConfirmationInvalid   ErrorCode = "CONFIRMATION_INVALID"
	CodeConfirmationExpired   ErrorCode = "CONFIRMATION_EXPIRED"
	CodeConfirmationStale     ErrorCode = "CONFIRMATION_STALE"
	CodeIdempotencyConflict   ErrorCode = "IDEMPOTENCY_CONFLICT"
	CodeSlotUnavailable       ErrorCode = "SLOT_UNAVAILABLE"
	CodeBookingStateConflict  ErrorCode = "BOOKING_STATE_CONFLICT"
	CodeDependencyUnavailable ErrorCode = "DEPENDENCY_UNAVAILABLE"
	CodeInternal              ErrorCode = "INTERNAL"
	CodeNotImplemented        ErrorCode = "NOT_IMPLEMENTED"
)

type ToolError struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	Retryable  bool      `json:"retryable"`
	Resolution string    `json:"resolution"`
}

type Money struct {
	MinorUnits int64  `json:"minor_units"`
	Currency   string `json:"currency"`
}

type Service struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Description         string `json:"description,omitempty"`
	DurationMinutes     int    `json:"duration_minutes"`
	BufferBeforeMinutes int    `json:"buffer_before_minutes"`
	BufferAfterMinutes  int    `json:"buffer_after_minutes"`
	Price               *Money `json:"price"`
}

type Staff struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}

// AvailableSlot is returned by the scheduling backend. The gateway turns it
// into a model-visible Slot by minting a scoped, expiring token.
type AvailableSlot struct {
	ServiceID   string
	ServiceName string
	StaffID     string
	StaffName   string
	StartAt     time.Time
	EndAt       time.Time
	Timezone    string
}

type Slot struct {
	SlotToken string    `json:"slot_token"`
	ServiceID string    `json:"service_id"`
	StaffID   string    `json:"staff_id"`
	StaffName string    `json:"staff_name"`
	StartAt   time.Time `json:"start_at"`
	EndAt     time.Time `json:"end_at"`
	Timezone  string    `json:"timezone"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ListServicesData struct {
	Services []Service `json:"services"`
}

type ListStaffData struct {
	Staff []Staff `json:"staff"`
}

type FindSlotsData struct {
	Slots            []Slot    `json:"slots"`
	AvailabilityAsOf time.Time `json:"availability_as_of"`
}

type CustomerProfile struct {
	DisplayName string         `json:"display_name"`
	Contact     ContactDetails `json:"contact"`
	Company     string         `json:"company,omitempty"`
	Locale      string         `json:"locale,omitempty"`
}

type ContactDetails struct {
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

type Booking struct {
	ID                  string    `json:"id"`
	Status              string    `json:"status"`
	ServiceID           string    `json:"service_id"`
	ServiceName         string    `json:"service_name"`
	StaffID             string    `json:"staff_id"`
	StaffName           string    `json:"staff_name"`
	StartAt             time.Time `json:"start_at"`
	EndAt               time.Time `json:"end_at"`
	Timezone            string    `json:"timezone"`
	CustomerDisplayName string    `json:"customer_display_name"`
	Version             int64     `json:"version"`
}

type FindSlotsQuery struct {
	TenantID  string
	ServiceID string
	StaffID   string
	DateFrom  time.Time
	DateTo    time.Time
}

// CreateBookingCommand contains trusted ownership and verified slot claims.
// Backend implementations must still re-check availability transactionally.
type CreateBookingCommand struct {
	TenantID        string
	OwnerCustomerID string
	ConversationID  string
	ServiceID       string
	StaffID         string
	StartAt         time.Time
	EndAt           time.Time
	Timezone        string
	Customer        CustomerProfile
	Notes           string
	IdempotencyKey  string
}

type CreateBookingOutcome struct {
	Booking             Booking
	CalendarSync        string
	IdempotencyReplayed bool
}

type CreateBookingData struct {
	Booking      Booking `json:"booking"`
	CalendarSync string  `json:"calendar_sync"`
}

// Backend is intentionally narrow so scheduling/Postgres can be adapted
// without depending on agent/provider types.
type Backend interface {
	ListServices(ctx context.Context, tenantID string) ([]Service, error)
	ListStaff(ctx context.Context, tenantID, serviceID string) ([]Staff, error)
	FindSlots(ctx context.Context, query FindSlotsQuery) ([]AvailableSlot, error)
	CreateBooking(ctx context.Context, command CreateBookingCommand) (CreateBookingOutcome, error)
}

var (
	ErrNotFoundOrNotOwned    = errors.New("not found or not owned")
	ErrSlotUnavailable       = errors.New("slot unavailable")
	ErrBookingStateConflict  = errors.New("booking state conflict")
	ErrIdempotencyConflict   = errors.New("idempotency conflict")
	ErrDependencyUnavailable = errors.New("dependency unavailable")
)
