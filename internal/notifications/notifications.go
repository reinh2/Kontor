package notifications

import (
	"context"
	"log/slog"
	"time"
)

// Reminder contains the data needed to send a booking reminder.
type Reminder struct {
	TenantID      string
	BookingID     string
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
	ServiceName   string
	StaffName     string
	StartsAt      time.Time
	Timezone      string
}

// SendResult reports the outcome of a notification delivery attempt.
type SendResult struct {
	Provider   string // e.g. "log", "email", "sms"
	ExternalID string // provider-specific message ID, empty for log driver
}

// Notifier is the boundary for sending customer notifications.
// Implementations must be safe for concurrent use.
type Notifier interface {
	// SendReminder delivers a booking reminder to the customer.
	// It should be idempotent when called with the same booking ID.
	SendReminder(ctx context.Context, reminder Reminder) (SendResult, error)
}

// LogNotifier is a no-op driver that logs reminder sends. Suitable for the
// demo mode and testing.
type LogNotifier struct {
	logger *slog.Logger
}

// NewLogNotifier creates a LogNotifier that writes reminder events to the
// provided structured logger.
func NewLogNotifier(logger *slog.Logger) *LogNotifier {
	return &LogNotifier{logger: logger}
}

// SendReminder logs the reminder details and returns a successful result with
// provider set to "log".
func (n *LogNotifier) SendReminder(ctx context.Context, reminder Reminder) (SendResult, error) {
	n.logger.InfoContext(ctx, "notification: reminder sent (log driver)",
		"tenant_id", reminder.TenantID,
		"booking_id", reminder.BookingID,
		"customer_name", reminder.CustomerName,
		"customer_email", reminder.CustomerEmail,
		"service", reminder.ServiceName,
		"staff", reminder.StaffName,
		"starts_at", reminder.StartsAt.Format(time.RFC3339),
	)
	return SendResult{Provider: "log"}, nil
}
