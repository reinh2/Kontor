package crm

import (
	"context"
	"log/slog"
	"time"
)

// Contact represents a customer in the CRM system.
type Contact struct {
	TenantID    string
	CustomerID  string
	DisplayName string
	Email       string
	Phone       string
	Company     string
	Locale      string
}

// Deal represents a booking-linked deal in the CRM.
type Deal struct {
	TenantID    string
	BookingID   string
	ContactRef  string
	ServiceName string
	StaffName   string
	StartsAt    time.Time
	Amount      int64  // minor units
	Currency    string
}

// UpsertResult is returned after creating/updating a CRM contact.
type UpsertResult struct {
	Provider   string // "log", "hubspot", "csv"
	ContactRef string // CRM-scoped reference for the contact (used in create_deal)
	Created    bool   // true if new, false if updated
}

// DealResult is returned after creating a CRM deal.
type DealResult struct {
	Provider string
	DealID   string
	Created  bool
}

// CRM is the boundary for CRM operations.
// Implementations must be safe for concurrent use.
type CRM interface {
	// UpsertContact creates or updates the customer's CRM contact.
	// Must be idempotent for the same customer ID.
	UpsertContact(ctx context.Context, contact Contact) (UpsertResult, error)

	// CreateDeal creates a deal/opportunity linked to a booking.
	// Must be idempotent for the same booking ID.
	CreateDeal(ctx context.Context, deal Deal) (DealResult, error)
}

// LogCRM is a no-op driver that logs CRM operations. Suitable for demo mode.
type LogCRM struct {
	logger *slog.Logger
}

func NewLogCRM(logger *slog.Logger) *LogCRM {
	return &LogCRM{logger: logger}
}

func (c *LogCRM) UpsertContact(ctx context.Context, contact Contact) (UpsertResult, error) {
	c.logger.InfoContext(ctx, "crm: upsert contact (log driver)",
		"tenant_id", contact.TenantID,
		"customer_id", contact.CustomerID,
		"display_name", contact.DisplayName,
		"email", contact.Email,
	)
	return UpsertResult{
		Provider:   "log",
		ContactRef: "ctr_v1_log_" + contact.CustomerID,
		Created:    true,
	}, nil
}

func (c *LogCRM) CreateDeal(ctx context.Context, deal Deal) (DealResult, error) {
	c.logger.InfoContext(ctx, "crm: create deal (log driver)",
		"tenant_id", deal.TenantID,
		"booking_id", deal.BookingID,
		"contact_ref", deal.ContactRef,
		"service", deal.ServiceName,
		"amount", deal.Amount,
		"currency", deal.Currency,
	)
	return DealResult{
		Provider: "log",
		DealID:   "deal_log_" + deal.BookingID,
		Created:  true,
	}, nil
}
