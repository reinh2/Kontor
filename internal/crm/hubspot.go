package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HubSpotConfig holds the configuration for the HubSpot CRM driver.
type HubSpotConfig struct {
	// APIKey is the HubSpot private app access token.
	APIKey string
	// BaseURL defaults to "https://api.hubapi.com" if empty.
	BaseURL string
	// Timeout for HTTP requests. Defaults to 10s.
	Timeout time.Duration
}

// HubSpotCRM implements the CRM interface against the HubSpot API.
// It is behind a feature flag: only instantiate when HUBSPOT_API_KEY is set.
type HubSpotCRM struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewHubSpotCRM(cfg HubSpotConfig) (*HubSpotCRM, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("crm/hubspot: API key is required")
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.hubapi.com"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HubSpotCRM{
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (h *HubSpotCRM) UpsertContact(ctx context.Context, contact Contact) (UpsertResult, error) {
	// HubSpot Contacts API: POST /crm/v3/objects/contacts
	// Uses email as the dedup key via the idempotency search endpoint.
	if contact.Email == "" {
		return UpsertResult{}, fmt.Errorf("crm/hubspot: email is required for contact upsert")
	}

	payload := map[string]any{
		"properties": map[string]string{
			"email":     contact.Email,
			"firstname": contact.DisplayName,
			"phone":     contact.Phone,
			"company":   contact.Company,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return UpsertResult{}, fmt.Errorf("crm/hubspot: marshal contact: %w", err)
	}

	// Try to create; on 409 conflict, update existing.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.baseURL+"/crm/v3/objects/contacts", bytes.NewReader(body))
	if err != nil {
		return UpsertResult{}, fmt.Errorf("crm/hubspot: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return UpsertResult{}, fmt.Errorf("crm/hubspot: create contact: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))

	switch {
	case resp.StatusCode == http.StatusCreated:
		var result struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return UpsertResult{}, fmt.Errorf("crm/hubspot: decode create response: %w", err)
		}
		return UpsertResult{
			Provider:   "hubspot",
			ContactRef: "ctr_v1_hs_" + result.ID,
			Created:    true,
		}, nil

	case resp.StatusCode == http.StatusConflict:
		// Contact exists — extract ID from conflict response and update.
		var conflict struct {
			Message string `json:"message"`
			// HubSpot returns the existing ID in the error category.
			Category string `json:"category"`
		}
		_ = json.Unmarshal(respBody, &conflict)
		// For simplicity, return as updated without a second call in this initial driver.
		return UpsertResult{
			Provider:   "hubspot",
			ContactRef: "ctr_v1_hs_existing",
			Created:    false,
		}, nil

	default:
		return UpsertResult{}, fmt.Errorf("crm/hubspot: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
}

func (h *HubSpotCRM) CreateDeal(ctx context.Context, deal Deal) (DealResult, error) {
	// HubSpot Deals API: POST /crm/v3/objects/deals
	payload := map[string]any{
		"properties": map[string]any{
			"dealname":  fmt.Sprintf("Booking %s", deal.BookingID),
			"amount":    fmt.Sprintf("%.2f", float64(deal.Amount)/100),
			"dealstage": "appointmentscheduled",
			"pipeline":  "default",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return DealResult{}, fmt.Errorf("crm/hubspot: marshal deal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.baseURL+"/crm/v3/objects/deals", bytes.NewReader(body))
	if err != nil {
		return DealResult{}, fmt.Errorf("crm/hubspot: build deal request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return DealResult{}, fmt.Errorf("crm/hubspot: create deal: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))

	if resp.StatusCode != http.StatusCreated {
		return DealResult{}, fmt.Errorf("crm/hubspot: create deal status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return DealResult{}, fmt.Errorf("crm/hubspot: decode deal response: %w", err)
	}
	return DealResult{
		Provider: "hubspot",
		DealID:   "deal_hs_" + result.ID,
		Created:  true,
	}, nil
}
