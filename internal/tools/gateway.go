package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"
)

const maxSlotSearchRange = 31 * 24 * time.Hour

type Config struct {
	Backend         Backend
	SlotSigningKey  []byte
	Confirmations   ConfirmationStore
	Now             func() time.Time
	SlotTokenTTL    time.Duration
	ConfirmationTTL time.Duration
}

type Gateway struct {
	backend         Backend
	signer          *SlotSigner
	confirmations   ConfirmationStore
	definitions     map[string]compiledDefinition
	now             func() time.Time
	slotTokenTTL    time.Duration
	confirmationTTL time.Duration
}

func NewGateway(config Config) (*Gateway, error) {
	if config.Backend == nil {
		return nil, errors.New("tools backend is required")
	}
	if config.Confirmations == nil {
		return nil, errors.New("confirmation store is required")
	}
	signer, err := NewSlotSigner(config.SlotSigningKey)
	if err != nil {
		return nil, err
	}
	compiled, err := compileDefinitions()
	if err != nil {
		return nil, err
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.SlotTokenTTL <= 0 {
		config.SlotTokenTTL = 5 * time.Minute
	}
	if config.ConfirmationTTL <= 0 {
		config.ConfirmationTTL = 10 * time.Minute
	}
	return &Gateway{
		backend:         config.Backend,
		signer:          signer,
		confirmations:   config.Confirmations,
		definitions:     compiled,
		now:             config.Now,
		slotTokenTTL:    config.SlotTokenTTL,
		confirmationTTL: config.ConfirmationTTL,
	}, nil
}

// Execute validates the call against the exact allowlist before dispatch.
func (g *Gateway) Execute(ctx context.Context, trusted TrustedContext, call Call) (out Result) {
	started := g.now()
	out = Result{
		SchemaVersion: ResultSchemaV1, Tool: call.Name, ContractVersion: ContractVersion,
		CallID: call.ID, Status: StatusError,
	}
	defer func() {
		out.Meta.DurationMS = g.now().Sub(started).Milliseconds()
	}()

	definition, allowed := g.definitions[call.Name]
	if !allowed {
		return g.failure(out, CodeToolNotAllowed, "tool is not in the server allowlist", false, "escalate")
	}
	arguments, err := validateArguments(definition.schema, call.Arguments)
	if err != nil {
		return g.failure(out, CodeInvalidArgument, err.Error(), false, "fix_arguments")
	}
	if !trusted.valid() {
		return g.failure(out, CodePolicyDenied, "trusted execution context is incomplete", false, "escalate")
	}
	if capability := requiredCapability(call.Name); !trusted.Allows(capability) {
		return g.failure(out, CodePolicyDenied, "authenticated principal is not permitted to call this tool", false, "escalate")
	}

	switch call.Name {
	case ToolListServices:
		return g.listServices(ctx, trusted, out)
	case ToolListStaff:
		return g.listStaff(ctx, trusted, out, arguments)
	case ToolFindSlots:
		return g.findSlots(ctx, trusted, out, arguments)
	case ToolCreateBooking:
		return g.createBooking(ctx, trusted, out, arguments)
	default:
		return g.failure(out, CodeNotImplemented, "tool is part of the v1 allowlist but is not implemented in Stage 1", false, "escalate")
	}
}

func requiredCapability(tool string) Capability {
	switch tool {
	case ToolListServices, ToolListStaff, ToolFindSlots:
		return CapabilityScheduleRead
	case ToolCreateBooking:
		return CapabilityBookingCreateSelf
	case ToolReschedule, ToolCancel:
		return CapabilityBookingWriteSelf
	case ToolUpsertContact:
		return CapabilityCRMContactSelf
	case ToolCreateDeal:
		return CapabilityCRMDealAfterBook
	case ToolEscalate:
		return CapabilityConversationEscalate
	default:
		return Capability("never")
	}
}

func (g *Gateway) listServices(ctx context.Context, trusted TrustedContext, result Result) Result {
	services, err := g.backend.ListServices(ctx, trusted.TenantID)
	if err != nil {
		return g.backendFailure(result, err)
	}
	return g.success(result, ListServicesData{Services: nonNilServices(services)}, false)
}

func (g *Gateway) listStaff(ctx context.Context, trusted TrustedContext, result Result, arguments map[string]any) Result {
	serviceID := arguments["service_id"].(string)
	staff, err := g.backend.ListStaff(ctx, trusted.TenantID, serviceID)
	if err != nil {
		return g.backendFailure(result, err)
	}
	return g.success(result, ListStaffData{Staff: nonNilStaff(staff)}, false)
}

func (g *Gateway) findSlots(ctx context.Context, trusted TrustedContext, result Result, arguments map[string]any) Result {
	dateFrom, err := time.Parse(time.RFC3339, arguments["date_from"].(string))
	if err != nil {
		return g.failure(result, CodeInvalidArgument, "date_from must be RFC3339", false, "fix_arguments")
	}
	dateTo, err := time.Parse(time.RFC3339, arguments["date_to"].(string))
	if err != nil {
		return g.failure(result, CodeInvalidArgument, "date_to must be RFC3339", false, "fix_arguments")
	}
	if !dateFrom.Before(dateTo) || dateTo.Sub(dateFrom) > maxSlotSearchRange {
		return g.failure(result, CodeInvalidArgument, "date_to must be after date_from and the range must not exceed 31 days", false, "fix_arguments")
	}
	query := FindSlotsQuery{
		TenantID: trusted.TenantID, ServiceID: arguments["service_id"].(string),
		DateFrom: dateFrom, DateTo: dateTo,
	}
	if staffID, ok := arguments["staff_id"].(string); ok {
		query.StaffID = staffID
	}
	available, err := g.backend.FindSlots(ctx, query)
	if err != nil {
		return g.backendFailure(result, err)
	}
	now := g.now()
	slots := make([]Slot, 0, len(available))
	for _, candidate := range available {
		if candidate.ServiceID == "" {
			candidate.ServiceID = query.ServiceID
		}
		if candidate.ServiceID != query.ServiceID || candidate.StaffID == "" ||
			(query.StaffID != "" && candidate.StaffID != query.StaffID) || candidate.Timezone == "" ||
			candidate.StartAt.Before(dateFrom) || candidate.EndAt.After(dateTo) ||
			!candidate.StartAt.Before(candidate.EndAt) {
			return g.failure(result, CodeInternal, "scheduling backend returned an invalid slot", false, "escalate")
		}
		if _, err := time.LoadLocation(candidate.Timezone); err != nil {
			return g.failure(result, CodeInternal, "scheduling backend returned an invalid timezone", false, "escalate")
		}
		expiresAt := now.Add(g.slotTokenTTL)
		token, err := g.signer.Sign(SlotClaims{
			TenantID: trusted.TenantID, ConversationID: trusted.ConversationID,
			ServiceID: candidate.ServiceID, ServiceName: candidate.ServiceName,
			StaffID: candidate.StaffID, StaffName: candidate.StaffName,
			StartAt: candidate.StartAt, EndAt: candidate.EndAt,
			Timezone: candidate.Timezone, ExpiresAt: expiresAt,
		})
		if err != nil {
			return g.failure(result, CodeInternal, "could not issue a slot token", false, "escalate")
		}
		slots = append(slots, Slot{
			SlotToken: token, ServiceID: candidate.ServiceID, StaffID: candidate.StaffID,
			StaffName: candidate.StaffName, StartAt: candidate.StartAt, EndAt: candidate.EndAt,
			Timezone: candidate.Timezone, ExpiresAt: expiresAt,
		})
	}
	return g.success(result, FindSlotsData{Slots: slots, AvailabilityAsOf: now}, false)
}

type createBookingArguments struct {
	SlotToken      string          `json:"slot_token"`
	Customer       CustomerProfile `json:"customer"`
	Notes          string          `json:"notes,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	ConfirmationID string          `json:"confirmation_id,omitempty"`
}

func (g *Gateway) createBooking(ctx context.Context, trusted TrustedContext, result Result, object map[string]any) Result {
	raw, _ := json.Marshal(object)
	var arguments createBookingArguments
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return g.failure(result, CodeInvalidArgument, "arguments could not be decoded", false, "fix_arguments")
	}
	now := g.now()
	claims, err := g.signer.Verify(arguments.SlotToken, trusted, now)
	if err != nil {
		switch {
		case errors.Is(err, ErrExpiredSlotToken):
			return g.failure(result, CodeSlotUnavailable, "slot token has expired; find a new slot", false, "find_another_slot")
		case errors.Is(err, ErrSlotTokenScope):
			return g.failure(result, CodePolicyDenied, "slot token is not valid for this conversation", false, "escalate")
		default:
			return g.failure(result, CodeInvalidArgument, "slot token is invalid or has been tampered with", false, "find_another_slot")
		}
	}
	binding := confirmationBinding(trusted, ToolCreateBooking, object)
	if arguments.ConfirmationID == "" {
		confirmationExpiresAt := now.Add(g.confirmationTTL)
		if claims.ExpiresAt.Before(confirmationExpiresAt) {
			confirmationExpiresAt = claims.ExpiresAt
		}
		proposal, err := g.confirmations.Propose(ctx, binding, ConfirmationProposal{
			Action: ToolCreateBooking, Title: "Confirm this booking",
			Facts: []ConfirmationFact{
				{Label: "Service", Value: firstNonEmpty(claims.ServiceName, claims.ServiceID)},
				{Label: "Staff", Value: firstNonEmpty(claims.StaffName, claims.StaffID)},
				{Label: "Starts", Value: claims.StartAt.Format(time.RFC3339)},
				{Label: "Ends", Value: claims.EndAt.Format(time.RFC3339)},
				{Label: "Customer", Value: arguments.Customer.DisplayName},
			},
			ExpiresAt: confirmationExpiresAt,
		}, now)
		if err != nil {
			return g.failure(result, CodeInternal, "could not create confirmation proposal", true, "retry")
		}
		result.Status = StatusConfirmationRequired
		result.Confirmation = &proposal
		result.Error = nil
		return result
	}
	if err := g.confirmations.VerifyAuthorized(ctx, arguments.ConfirmationID, binding, now); err != nil {
		switch {
		case errors.Is(err, ErrConfirmationExpired):
			return g.failure(result, CodeConfirmationExpired, "confirmation has expired", false, "ask_customer")
		case errors.Is(err, ErrConfirmationStale):
			return g.failure(result, CodeConfirmationStale, "confirmation does not match these arguments", false, "ask_customer")
		default:
			return g.failure(result, CodeConfirmationInvalid, "confirmation is not authorized or was already used", false, "ask_customer")
		}
	}
	outcome, err := g.backend.CreateBooking(ctx, CreateBookingCommand{
		TenantID: trusted.TenantID, OwnerCustomerID: trusted.CustomerID,
		ConversationID: trusted.ConversationID, ServiceID: claims.ServiceID,
		StaffID: claims.StaffID, StartAt: claims.StartAt, EndAt: claims.EndAt,
		Timezone: claims.Timezone, Customer: arguments.Customer, Notes: arguments.Notes,
		IdempotencyKey: arguments.IdempotencyKey,
	})
	if err != nil {
		return g.backendFailure(result, err)
	}
	if err := g.confirmations.MarkConsumed(ctx, arguments.ConfirmationID, binding, now); err != nil {
		// The booking is already safely committed and idempotent. Surface an
		// internal error so the caller retries; replay will not duplicate it.
		return g.failure(result, CodeInternal, "booking succeeded but confirmation finalization must be retried", true, "retry")
	}
	calendarSync := outcome.CalendarSync
	if calendarSync == "" {
		calendarSync = "queued"
	}
	return g.success(result, CreateBookingData{
		Booking: outcome.Booking, CalendarSync: calendarSync,
	}, outcome.IdempotencyReplayed)
}

func confirmationBinding(trusted TrustedContext, tool string, arguments map[string]any) ConfirmationBinding {
	copyArguments := make(map[string]any, len(arguments))
	for key, value := range arguments {
		if key != "confirmation_id" {
			copyArguments[key] = value
		}
	}
	canonical, _ := json.Marshal(copyArguments)
	return ConfirmationBinding{
		TenantID: trusted.TenantID, OwnerCustomerID: trusted.CustomerID,
		ConversationID: trusted.ConversationID, ProposedFromMessageID: trusted.InboundMessageID,
		Tool: tool, ArgumentsHash: sha256.Sum256(canonical), ArgumentsJSON: canonical,
	}
}

func (g *Gateway) success(result Result, data any, replayed bool) Result {
	result.Status = StatusSuccess
	result.Data = data
	result.Error = nil
	result.Confirmation = nil
	result.Meta.IdempotencyReplayed = replayed
	return result
}

func (g *Gateway) failure(result Result, code ErrorCode, message string, retryable bool, resolution string) Result {
	result.Status = StatusError
	result.Data = nil
	result.Confirmation = nil
	result.Error = &ToolError{Code: code, Message: message, Retryable: retryable, Resolution: resolution}
	return result
}

func (g *Gateway) backendFailure(result Result, err error) Result {
	switch {
	case errors.Is(err, ErrNotFoundOrNotOwned):
		return g.failure(result, CodeNotFoundOrNotOwned, "resource was not found or is not owned by this customer", false, "escalate")
	case errors.Is(err, ErrSlotUnavailable):
		return g.failure(result, CodeSlotUnavailable, "slot is no longer available", false, "find_another_slot")
	case errors.Is(err, ErrBookingStateConflict):
		return g.failure(result, CodeBookingStateConflict, "booking state changed", false, "ask_customer")
	case errors.Is(err, ErrIdempotencyConflict):
		return g.failure(result, CodeIdempotencyConflict, "idempotency key was already used with different arguments", false, "escalate")
	case errors.Is(err, ErrDependencyUnavailable):
		return g.failure(result, CodeDependencyUnavailable, "a required dependency is unavailable", true, "retry")
	default:
		return g.failure(result, CodeInternal, "tool execution failed", false, "escalate")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown"
}

func nonNilServices(values []Service) []Service {
	if values == nil {
		return []Service{}
	}
	return values
}

func nonNilStaff(values []Staff) []Staff {
	if values == nil {
		return []Staff{}
	}
	return values
}
