package tenants

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reinhlord/kontor/internal/platform/ids"
)

type Service struct {
	ID                  string `json:"id"`
	Slug                string `json:"slug"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	DurationMinutes     int    `json:"duration_minutes"`
	BufferBeforeMinutes int    `json:"buffer_before_minutes"`
	BufferAfterMinutes  int    `json:"buffer_after_minutes"`
	PriceMinor          int64  `json:"price_minor"`
	Currency            string `json:"currency"`
}

type Staff struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
}

func (s *Store) CreateService(ctx context.Context, tenantID string, input ServiceInput) (Service, error) {
	if s == nil || s.pool == nil || tenantID == "" || !validService(input) {
		return Service{}, ErrInvalidInput
	}
	var service Service
	err := s.pool.QueryRow(ctx, `
		INSERT INTO services(
			tenant_id,slug,name,description,duration_minutes,buffer_before_minutes,
			buffer_after_minutes,price_minor,currency
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id::text,slug,name,description,duration_minutes,buffer_before_minutes,
		          buffer_after_minutes,price_minor,currency`,
		tenantID, input.Slug, strings.TrimSpace(input.Name), input.Description,
		input.DurationMinutes, input.BufferBeforeMinutes, input.BufferAfterMinutes,
		input.PriceMinor, input.Currency,
	).Scan(&service.ID, &service.Slug, &service.Name, &service.Description,
		&service.DurationMinutes, &service.BufferBeforeMinutes, &service.BufferAfterMinutes,
		&service.PriceMinor, &service.Currency)
	if err != nil {
		return Service{}, mapProvisionError(err)
	}
	return service, nil
}

func (s *Store) CreateStaff(ctx context.Context, tenantID string, input StaffInput) (Staff, error) {
	if s == nil || s.pool == nil || tenantID == "" || !validStaffInput(input) {
		return Staff{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Staff{}, fmt.Errorf("tenants: begin create staff: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	staff := Staff{ID: ids.New(), Slug: input.Slug, DisplayName: strings.TrimSpace(input.DisplayName), Timezone: input.Timezone}
	if _, err := tx.Exec(ctx, `
		INSERT INTO staff(tenant_id,id,slug,display_name,timezone)
		VALUES($1,$2,$3,$4,$5)`, tenantID, staff.ID, staff.Slug, staff.DisplayName, staff.Timezone); err != nil {
		return Staff{}, mapProvisionError(err)
	}
	for _, serviceSlug := range input.ServiceSlugs {
		tag, err := tx.Exec(ctx, `
			INSERT INTO staff_services(tenant_id,staff_id,service_id)
			SELECT $1,$2,id FROM services WHERE tenant_id=$1 AND slug=$3 AND active`,
			tenantID, staff.ID, serviceSlug)
		if err != nil {
			return Staff{}, fmt.Errorf("tenants: assign staff service: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return Staff{}, ErrInvalidInput
		}
	}
	if err := insertAvailability(ctx, tx, tenantID, staff.ID, input.Availability); err != nil {
		return Staff{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Staff{}, fmt.Errorf("tenants: commit create staff: %w", err)
	}
	return staff, nil
}

func (s *Store) AddAvailability(ctx context.Context, tenantID, staffID string, rules []AvailabilityRuleInput) error {
	if s == nil || s.pool == nil || tenantID == "" || staffID == "" || len(rules) == 0 || !validRules(rules) {
		return ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tenants: begin add availability: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM staff WHERE tenant_id=$1 AND id=$2)`, tenantID, staffID).Scan(&exists); err != nil {
		return fmt.Errorf("tenants: check staff: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	if err := insertAvailability(ctx, tx, tenantID, staffID, rules); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("tenants: commit availability: %w", err)
	}
	return nil
}

func (s *Store) UpdateChannels(ctx context.Context, tenantID string, input ChannelConfig) error {
	if s == nil || s.pool == nil || tenantID == "" {
		return ErrInvalidInput
	}
	widgetOrigin, err := CanonicalWidgetOrigin(input.WidgetOrigin)
	if err != nil {
		return ErrInvalidInput
	}
	input.WidgetOrigin = widgetOrigin
	ciphertext, nonce, digest, err := s.prepareChannelConfig(input)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE tenant_channels
		SET widget_origin=$2,telegram_bot_token_ciphertext=$3,telegram_bot_token_nonce=$4,
		    telegram_webhook_secret_digest=$5,telegram_enabled=$6,updated_at=now()
		WHERE tenant_id=$1`, tenantID, input.WidgetOrigin, ciphertext, nonce, nullableDigest(digest), input.TelegramEnabled)
	if err != nil {
		return fmt.Errorf("tenants: update channels: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ChannelConfig(ctx context.Context, tenantID string) (ChannelConfig, error) {
	if s == nil || s.pool == nil || tenantID == "" {
		return ChannelConfig{}, ErrNotFound
	}
	var config ChannelConfig
	err := s.pool.QueryRow(ctx, `
		SELECT widget_origin,telegram_enabled FROM tenant_channels WHERE tenant_id=$1`, tenantID,
	).Scan(&config.WidgetOrigin, &config.TelegramEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChannelConfig{}, ErrNotFound
	}
	if err != nil {
		return ChannelConfig{}, fmt.Errorf("tenants: get channels: %w", err)
	}
	return config, nil
}

func validService(service ServiceInput) bool {
	return slugPattern.MatchString(service.Slug) && strings.TrimSpace(service.Name) != "" && len(service.Name) <= 200 &&
		len(service.Description) <= 2000 && service.DurationMinutes >= 5 && service.DurationMinutes <= 1440 &&
		service.BufferBeforeMinutes >= 0 && service.BufferBeforeMinutes <= 240 &&
		service.BufferAfterMinutes >= 0 && service.BufferAfterMinutes <= 240 && service.PriceMinor >= 0 &&
		len(service.Currency) == 3 && service.Currency == strings.ToUpper(service.Currency)
}

func validStaffInput(member StaffInput) bool {
	if !slugPattern.MatchString(member.Slug) || strings.TrimSpace(member.DisplayName) == "" || len(member.DisplayName) > 200 || len(member.ServiceSlugs) == 0 || len(member.Availability) == 0 {
		return false
	}
	if _, err := time.LoadLocation(member.Timezone); err != nil {
		return false
	}
	seen := map[string]struct{}{}
	for _, serviceSlug := range member.ServiceSlugs {
		if !slugPattern.MatchString(serviceSlug) {
			return false
		}
		if _, duplicate := seen[serviceSlug]; duplicate {
			return false
		}
		seen[serviceSlug] = struct{}{}
	}
	return validRules(member.Availability)
}

func validRules(rules []AvailabilityRuleInput) bool {
	for _, rule := range rules {
		if (rule.RuleType != "working" && rule.RuleType != "break") || rule.DayOfWeek < 0 || rule.DayOfWeek > 6 || !validLocalTimeRange(rule.LocalStart, rule.LocalEnd) {
			return false
		}
	}
	return true
}

func insertAvailability(ctx context.Context, tx pgx.Tx, tenantID, staffID string, rules []AvailabilityRuleInput) error {
	for _, rule := range rules {
		if _, err := tx.Exec(ctx, `
			INSERT INTO availability_rules(tenant_id,id,staff_id,rule_type,day_of_week,local_start,local_end)
			VALUES($1,$2,$3,$4,$5,$6::time,$7::time)`,
			tenantID, ids.New(), staffID, rule.RuleType, rule.DayOfWeek, rule.LocalStart, rule.LocalEnd); err != nil {
			return fmt.Errorf("tenants: insert availability rule: %w", err)
		}
	}
	return nil
}
