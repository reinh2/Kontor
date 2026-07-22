package scheduling

import (
	"context"
	"errors"
	"fmt"
	"time"

	toolapi "github.com/reinhlord/kontor/internal/tools"
)

// ToolBackend adapts the scheduling repository to the deliberately narrow
// tools.Backend boundary. It never trusts a model-provided tenant: every call
// must match the fixed tenant bound into the repository at startup.
type ToolBackend struct {
	repository *PGXRepository
}

func NewToolBackend(repository *PGXRepository) *ToolBackend {
	return &ToolBackend{repository: repository}
}

func (b *ToolBackend) ListServices(ctx context.Context, tenantID string) ([]toolapi.Service, error) {
	if err := b.authorizeTenant(tenantID); err != nil {
		return nil, err
	}
	services, err := b.repository.ListServices(ctx)
	if err != nil {
		return nil, mapToolBackendError(err)
	}
	result := make([]toolapi.Service, 0, len(services))
	for _, service := range services {
		result = append(result, toolapi.Service{
			ID: service.ID, Name: service.Name, Description: service.Description,
			DurationMinutes:     minutes(service.Duration),
			BufferBeforeMinutes: minutes(service.BufferBefore),
			BufferAfterMinutes:  minutes(service.BufferAfter),
			Price:               &toolapi.Money{MinorUnits: service.PriceMinor, Currency: service.Currency},
		})
	}
	return result, nil
}

func (b *ToolBackend) ListStaff(ctx context.Context, tenantID, serviceID string) ([]toolapi.Staff, error) {
	if err := b.authorizeTenant(tenantID); err != nil {
		return nil, err
	}
	staff, err := b.repository.ListStaff(ctx, serviceID)
	if err != nil {
		return nil, mapToolBackendError(err)
	}
	result := make([]toolapi.Staff, 0, len(staff))
	for _, member := range staff {
		result = append(result, toolapi.Staff{
			ID: member.ID, DisplayName: member.DisplayName, Timezone: member.Timezone,
		})
	}
	return result, nil
}

func (b *ToolBackend) FindSlots(ctx context.Context, query toolapi.FindSlotsQuery) ([]toolapi.AvailableSlot, error) {
	if err := b.authorizeTenant(query.TenantID); err != nil {
		return nil, err
	}
	services, err := b.repository.ListServices(ctx)
	if err != nil {
		return nil, mapToolBackendError(err)
	}
	var selectedService Service
	for _, service := range services {
		if service.ID == query.ServiceID {
			selectedService = service
			break
		}
	}
	if selectedService.ID == "" {
		return nil, toolapi.ErrNotFoundOrNotOwned
	}
	staff, err := b.repository.ListStaff(ctx, query.ServiceID)
	if err != nil {
		return nil, mapToolBackendError(err)
	}
	staffByID := make(map[string]Staff, len(staff))
	for _, member := range staff {
		staffByID[member.ID] = member
	}
	if query.StaffID != "" {
		if _, found := staffByID[query.StaffID]; !found {
			return nil, toolapi.ErrNotFoundOrNotOwned
		}
	}
	slots, err := b.repository.FindSlots(ctx, FindSlotsRequest{
		ServiceID: query.ServiceID, StaffID: query.StaffID,
		From: query.DateFrom, To: query.DateTo, SlotInterval: 15 * time.Minute, Limit: 100,
	})
	if err != nil {
		return nil, mapToolBackendError(err)
	}
	result := make([]toolapi.AvailableSlot, 0, len(slots))
	for _, slot := range slots {
		member, found := staffByID[slot.StaffID]
		if !found {
			return nil, fmt.Errorf("%w: slot references unknown staff", toolapi.ErrDependencyUnavailable)
		}
		result = append(result, toolapi.AvailableSlot{
			ServiceID: selectedService.ID, ServiceName: selectedService.Name,
			StaffID: member.ID, StaffName: member.DisplayName,
			StartAt: slot.Start, EndAt: slot.End, Timezone: member.Timezone,
		})
	}
	return result, nil
}

func (b *ToolBackend) CreateBooking(ctx context.Context, command toolapi.CreateBookingCommand) (toolapi.CreateBookingOutcome, error) {
	if err := b.authorizeTenant(command.TenantID); err != nil {
		return toolapi.CreateBookingOutcome{}, err
	}
	if command.OwnerCustomerID == "" {
		return toolapi.CreateBookingOutcome{}, toolapi.ErrNotFoundOrNotOwned
	}
	services, err := b.repository.ListServices(ctx)
	if err != nil {
		return toolapi.CreateBookingOutcome{}, mapToolBackendError(err)
	}
	var service Service
	for _, candidate := range services {
		if candidate.ID == command.ServiceID {
			service = candidate
			break
		}
	}
	if service.ID == "" || !command.EndAt.Equal(command.StartAt.Add(service.Duration)) {
		// EndAt came from a signed slot claim. A mismatch means stale/corrupt
		// server state and must not be silently reinterpreted.
		return toolapi.CreateBookingOutcome{}, toolapi.ErrSlotUnavailable
	}
	staff, err := b.repository.ListStaff(ctx, command.ServiceID)
	if err != nil {
		return toolapi.CreateBookingOutcome{}, mapToolBackendError(err)
	}
	var member Staff
	for _, candidate := range staff {
		if candidate.ID == command.StaffID {
			member = candidate
			break
		}
	}
	if member.ID == "" || command.Timezone != member.Timezone {
		return toolapi.CreateBookingOutcome{}, toolapi.ErrSlotUnavailable
	}

	created, err := b.repository.CreateBooking(ctx, CreateBookingRequest{
		CustomerID: command.OwnerCustomerID, ConversationID: command.ConversationID,
		ServiceID: command.ServiceID, StaffID: command.StaffID, StartsAt: command.StartAt,
		Notes: command.Notes, IdempotencyKey: command.IdempotencyKey,
	})
	if err != nil {
		return toolapi.CreateBookingOutcome{}, mapToolBackendError(err)
	}
	return toolapi.CreateBookingOutcome{
		Booking: toolapi.Booking{
			ID: created.Booking.ID, Status: created.Booking.Status,
			ServiceID: service.ID, ServiceName: service.Name,
			StaffID: member.ID, StaffName: member.DisplayName,
			StartAt: created.Booking.StartsAt, EndAt: created.Booking.EndsAt,
			Timezone: member.Timezone, CustomerDisplayName: command.Customer.DisplayName,
			Version: int64(created.Booking.ScheduleVersion),
		},
		CalendarSync:        "noop",
		IdempotencyReplayed: created.Replayed,
	}, nil
}

// Escalate persists the model-requested hand-off under the same trusted
// customer/conversation scope used by booking tools. A repeated tool call in
// one agent run returns the original escalation.
func (b *ToolBackend) Escalate(ctx context.Context, command toolapi.EscalationCommand) (toolapi.EscalationOutcome, error) {
	if err := b.authorizeTenant(command.TenantID); err != nil {
		return toolapi.EscalationOutcome{}, err
	}
	if command.OwnerCustomerID == "" || command.ConversationID == "" ||
		command.ReasonCode == "" || command.Summary == "" {
		return toolapi.EscalationOutcome{}, toolapi.ErrNotFoundOrNotOwned
	}
	tx, err := b.repository.pool.Begin(ctx)
	if err != nil {
		return toolapi.EscalationOutcome{}, fmt.Errorf("%w: begin escalation: %v", toolapi.ErrDependencyUnavailable, err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `
		UPDATE conversations SET status='escalated',updated_at=now()
		WHERE tenant_id=$1 AND id=$2 AND customer_id=$3`,
		command.TenantID, command.ConversationID, command.OwnerCustomerID)
	if err != nil {
		return toolapi.EscalationOutcome{}, fmt.Errorf("%w: mark conversation escalated: %v", toolapi.ErrDependencyUnavailable, err)
	}
	if tag.RowsAffected() != 1 {
		return toolapi.EscalationOutcome{}, toolapi.ErrNotFoundOrNotOwned
	}

	var outcome toolapi.EscalationOutcome
	if command.AgentRunID != "" && command.ToolCallID != "" {
		err = tx.QueryRow(ctx, `
			WITH inserted AS (
				INSERT INTO escalations
					(tenant_id,conversation_id,customer_id,agent_run_id,source_tool_call_id,reason_code,summary)
				VALUES($1,$2,$3,$4,$5,$6,$7)
				ON CONFLICT (tenant_id,agent_run_id,source_tool_call_id)
					WHERE source_tool_call_id IS NOT NULL DO NOTHING
				RETURNING id::text,status
			)
			SELECT id,status,false FROM inserted
			UNION ALL
			SELECT id::text,status,true FROM escalations
			WHERE tenant_id=$1 AND agent_run_id=$4 AND source_tool_call_id=$5
			  AND NOT EXISTS (SELECT 1 FROM inserted)
			LIMIT 1`,
			command.TenantID, command.ConversationID, command.OwnerCustomerID,
			command.AgentRunID, command.ToolCallID, command.ReasonCode, command.Summary).
			Scan(&outcome.ID, &outcome.Status, &outcome.Replayed)
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO escalations
				(tenant_id,conversation_id,customer_id,reason_code,summary)
			VALUES($1,$2,$3,$4,$5)
			RETURNING id::text,status`,
			command.TenantID, command.ConversationID, command.OwnerCustomerID,
			command.ReasonCode, command.Summary).Scan(&outcome.ID, &outcome.Status)
	}
	if err != nil {
		return toolapi.EscalationOutcome{}, fmt.Errorf("%w: persist escalation: %v", toolapi.ErrDependencyUnavailable, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return toolapi.EscalationOutcome{}, fmt.Errorf("%w: commit escalation: %v", toolapi.ErrDependencyUnavailable, err)
	}
	return outcome, nil
}

func (b *ToolBackend) authorizeTenant(tenantID string) error {
	if b == nil || b.repository == nil {
		return fmt.Errorf("%w: scheduling repository is unavailable", toolapi.ErrDependencyUnavailable)
	}
	if tenantID == "" || tenantID != b.repository.tenantID {
		// Use the same indistinguishable response as an unowned resource.
		return toolapi.ErrNotFoundOrNotOwned
	}
	if b.repository.pool == nil {
		return fmt.Errorf("%w: scheduling repository is unavailable", toolapi.ErrDependencyUnavailable)
	}
	return nil
}

func mapToolBackendError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return toolapi.ErrNotFoundOrNotOwned
	case errors.Is(err, ErrSlotUnavailable):
		return toolapi.ErrSlotUnavailable
	case errors.Is(err, ErrIdempotencyConflict):
		return toolapi.ErrIdempotencyConflict
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %v", toolapi.ErrDependencyUnavailable, err)
	default:
		return fmt.Errorf("%w: %v", toolapi.ErrDependencyUnavailable, err)
	}
}

var _ toolapi.Backend = (*ToolBackend)(nil)
