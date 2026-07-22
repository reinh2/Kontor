// Package tenants owns Stage 6 tenant provisioning, public tenant lookup,
// and tenant-owned channel configuration.
package tenants

import (
	"bytes"
	"encoding/json"
	"time"
)

type Tenant struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
	Currency string `json:"currency"`
	// WidgetOrigin is transport metadata used only by the host/CORS boundary;
	// it is omitted from tenant responses because it is configuration, not a
	// customer-facing business identity.
	WidgetOrigin string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ChannelConfig is the owner-managed tenant channel configuration. Secrets are
// accepted on write but never returned from public methods or JSON responses.
type ChannelConfig struct {
	WidgetOrigin          string `json:"widget_origin"`
	TelegramEnabled       bool   `json:"telegram_enabled"`
	TelegramBotToken      string `json:"-"`
	TelegramWebhookSecret string `json:"-"`
}

// UnmarshalJSON accepts channel credentials on tenant-owner writes while the
// json:"-" fields above ensure that reads never reveal them. The local
// decoder preserves the enclosing API's strict unknown-field contract.
func (c *ChannelConfig) UnmarshalJSON(data []byte) error {
	var input struct {
		WidgetOrigin          string `json:"widget_origin"`
		TelegramEnabled       bool   `json:"telegram_enabled"`
		TelegramBotToken      string `json:"telegram_bot_token"`
		TelegramWebhookSecret string `json:"telegram_webhook_secret"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return err
	}
	*c = ChannelConfig{
		WidgetOrigin:          input.WidgetOrigin,
		TelegramEnabled:       input.TelegramEnabled,
		TelegramBotToken:      input.TelegramBotToken,
		TelegramWebhookSecret: input.TelegramWebhookSecret,
	}
	return nil
}

type ServiceInput struct {
	Slug                string `json:"slug"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	DurationMinutes     int    `json:"duration_minutes"`
	BufferBeforeMinutes int    `json:"buffer_before_minutes"`
	BufferAfterMinutes  int    `json:"buffer_after_minutes"`
	PriceMinor          int64  `json:"price_minor"`
	Currency            string `json:"currency"`
}

type AvailabilityRuleInput struct {
	RuleType   string `json:"rule_type"`
	DayOfWeek  int    `json:"day_of_week"`
	LocalStart string `json:"local_start"`
	LocalEnd   string `json:"local_end"`
}

type StaffInput struct {
	Slug         string                  `json:"slug"`
	DisplayName  string                  `json:"display_name"`
	Timezone     string                  `json:"timezone"`
	ServiceSlugs []string                `json:"service_slugs"`
	Availability []AvailabilityRuleInput `json:"availability"`
}

// ProvisionInput is deliberately complete: committing it creates a usable
// business, not a tenant shell that must be repaired through environment vars.
type ProvisionInput struct {
	Slug     string         `json:"slug"`
	Name     string         `json:"name"`
	Timezone string         `json:"timezone"`
	Currency string         `json:"currency"`
	Owner    OwnerInput     `json:"owner"`
	Channels ChannelConfig  `json:"channels"`
	Services []ServiceInput `json:"services"`
	Staff    []StaffInput   `json:"staff"`
}

type OwnerInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type TelegramCredentials struct {
	BotToken      string
	WebhookDigest [32]byte
}

// LegacyBootstrapInput contains the one-time owner and widget configuration
// for a tenant that existed before Stage 6.
type LegacyBootstrapInput struct {
	TenantID     string
	TenantSlug   string
	WidgetOrigin string
	Owner        OwnerInput
	// Telegram carries the optional Stage 5 global channel only during the
	// explicit legacy adoption transaction. New and existing Stage 6 tenants
	// are configured through owner-authorized channel APIs instead.
	Telegram ChannelConfig
}

// LegacyBootstrapResult distinguishes the one mutating bootstrap from a safe
// exact retry. It contains no credentials or session tokens.
type LegacyBootstrapResult struct {
	Applied bool
}
