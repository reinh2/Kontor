package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	demoHaircutServiceID = "10000000-0000-4000-8000-000000000001"
	demoAlexStaffID      = "20000000-0000-4000-8000-000000000001"
	// AuthorizedActionPrefix marks a server-injected, previously confirmed
	// action. User messages can never create this trusted system role marker.
	AuthorizedActionPrefix = "KONTOR_AUTHORIZED_ACTION_V1 "
)

// DemoConfig customizes the reusable zero-key booking scenario.
type DemoConfig struct {
	Now           func() time.Time
	ServiceID     string
	StaffID       string
	CustomerName  string
	CustomerEmail string
	Timezone      string
}

// DemoAdapter deterministically drives the Stage 1 haircut scenario from
// message history. Unlike FakeAdapter it is not a finite queue, so it can serve
// repeated zero-key demo conversations. It deliberately emits list_services
// and list_staff together to exercise multi-tool responses.
type DemoAdapter struct {
	now           func() time.Time
	serviceID     string
	staffID       string
	customerName  string
	customerEmail string
	timezone      *time.Location
}

// NewDemoAdapter returns a deterministic local model substitute.
func NewDemoAdapter(config DemoConfig) (*DemoAdapter, error) {
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.ServiceID == "" {
		config.ServiceID = demoHaircutServiceID
	}
	if config.StaffID == "" {
		config.StaffID = demoAlexStaffID
	}
	if config.CustomerName == "" {
		config.CustomerName = "Demo Customer"
	}
	if config.CustomerEmail == "" {
		config.CustomerEmail = "demo@example.com"
	}
	if config.Timezone == "" {
		config.Timezone = "Europe/Berlin"
	}
	location, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return nil, fmt.Errorf("llm demo: load timezone: %w", err)
	}
	return &DemoAdapter{
		now:           config.Now,
		serviceID:     config.ServiceID,
		staffID:       config.StaffID,
		customerName:  config.CustomerName,
		customerEmail: config.CustomerEmail,
		timezone:      location,
	}, nil
}

// Complete implements Adapter without network access or mutable scenario
// state; the same history always produces the same response.
func (a *DemoAdapter) Complete(ctx context.Context, request Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	response := Response{
		Model:   "kontor/demo-v1",
		Usage:   Usage{InputTokens: 80, OutputTokens: 48, TotalTokens: 128},
		Message: Message{Role: RoleAssistant},
	}
	if authorized, ok := authorizedAction(request.Messages); ok {
		hash := sha256.Sum256(append([]byte(authorized.Tool), authorized.Arguments...))
		response.ID = "demo-authorized-action"
		response.FinishReason = "tool_calls"
		response.Message.ToolCalls = []ToolCall{{
			ID:        "demo-call-authorized-" + hex.EncodeToString(hash[:6]),
			Name:      authorized.Tool,
			Arguments: append(json.RawMessage(nil), authorized.Arguments...),
		}}
		return response, nil
	}

	createResult, hasCreateResult := latestToolResult(request.Messages, "create_booking")
	if hasCreateResult {
		status := jsonStringAt(createResult, "status")
		switch status {
		case "success":
			response.ID = "demo-booking-complete"
			response.FinishReason = "stop"
			response.Message.Content = "Your appointment is booked. I’ve sent the confirmation details."
			return response, nil
		case "confirmation_required":
			confirmationID := jsonStringAt(createResult, "confirmation", "id")
			if latestMessageRole(request.Messages) == RoleUser && isAffirmative(latestUserText(request.Messages)) {
				arguments, ok := latestAssistantToolArguments(request.Messages, "create_booking")
				if ok && confirmationID != "" {
					var object map[string]any
					if json.Unmarshal(arguments, &object) == nil {
						object["confirmation_id"] = confirmationID
						confirmedArguments, _ := json.Marshal(object)
						response.ID = "demo-confirm-booking"
						response.FinishReason = "tool_calls"
						response.Message.ToolCalls = []ToolCall{{
							ID: "demo-call-create-confirmed", Name: "create_booking", Arguments: confirmedArguments,
						}}
						return response, nil
					}
				}
			}
			response.ID = "demo-request-confirmation"
			response.FinishReason = "stop"
			response.Message.Content = "Please confirm the booking summary shown above. Nothing will be booked until you explicitly confirm."
			return response, nil
		case "error":
			response.ID = "demo-booking-error"
			response.FinishReason = "stop"
			response.Message.Content = "I couldn’t complete that booking. Please choose another slot or ask for a human."
			return response, nil
		}
	}

	findResult, hasFindResult := latestToolResult(request.Messages, "find_slots")
	if hasFindResult {
		slotToken := findFirstString(findResult, "slot_token")
		if slotToken == "" {
			response.ID = "demo-no-slots"
			response.FinishReason = "stop"
			response.Message.Content = "I couldn’t find an available demo slot."
			return response, nil
		}
		keyHash := sha256.Sum256([]byte(slotToken))
		arguments, _ := json.Marshal(map[string]any{
			"slot_token": slotToken,
			"customer": map[string]any{
				"display_name": a.customerName,
				"contact":      map[string]any{"email": a.customerEmail},
			},
			"idempotency_key": "demo-booking-" + hex.EncodeToString(keyHash[:8]),
		})
		response.ID = "demo-propose-booking"
		response.FinishReason = "tool_calls"
		response.Message.ToolCalls = []ToolCall{{ID: "demo-call-create", Name: "create_booking", Arguments: arguments}}
		return response, nil
	}

	if _, hasStaff := latestToolResult(request.Messages, "list_staff"); hasStaff {
		dateFrom, dateTo := a.nextThursdayEvening()
		arguments, _ := json.Marshal(map[string]any{
			"service_id": a.serviceID,
			"staff_id":   a.staffID,
			"date_from":  dateFrom.Format(time.RFC3339),
			"date_to":    dateTo.Format(time.RFC3339),
		})
		response.ID = "demo-find-slots"
		response.FinishReason = "tool_calls"
		response.Message.ToolCalls = []ToolCall{{ID: "demo-call-find-slots", Name: "find_slots", Arguments: arguments}}
		return response, nil
	}

	staffArguments, _ := json.Marshal(map[string]any{"service_id": a.serviceID})
	response.ID = "demo-discover"
	response.FinishReason = "tool_calls"
	response.Message.ToolCalls = []ToolCall{
		{ID: "demo-call-list-services", Name: "list_services", Arguments: json.RawMessage(`{}`)},
		{ID: "demo-call-list-staff", Name: "list_staff", Arguments: staffArguments},
	}
	return response, nil
}

type demoAuthorizedAction struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

func authorizedAction(messages []Message) (demoAuthorizedAction, bool) {
	if len(messages) == 0 {
		return demoAuthorizedAction{}, false
	}
	last := messages[len(messages)-1]
	if last.Role != RoleSystem || !strings.HasPrefix(last.Content, AuthorizedActionPrefix) {
		return demoAuthorizedAction{}, false
	}
	var action demoAuthorizedAction
	if json.Unmarshal([]byte(strings.TrimPrefix(last.Content, AuthorizedActionPrefix)), &action) != nil ||
		action.Tool == "" || len(action.Arguments) == 0 || !json.Valid(action.Arguments) {
		return demoAuthorizedAction{}, false
	}
	return action, true
}

func (a *DemoAdapter) nextThursdayEvening() (time.Time, time.Time) {
	now := a.now().In(a.timezone)
	days := (int(time.Thursday) - int(now.Weekday()) + 7) % 7
	if days == 0 {
		days = 7
	}
	date := now.AddDate(0, 0, days)
	return time.Date(date.Year(), date.Month(), date.Day(), 17, 0, 0, 0, a.timezone),
		time.Date(date.Year(), date.Month(), date.Day(), 20, 0, 0, 0, a.timezone)
}

func latestToolResult(messages []Message, name string) (map[string]any, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != RoleTool || messages[i].Name != name {
			continue
		}
		var object map[string]any
		if json.Unmarshal([]byte(messages[i].Content), &object) != nil {
			return nil, false
		}
		return object, true
	}
	return nil, false
}

func latestAssistantToolArguments(messages []Message, name string) (json.RawMessage, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		for j := len(messages[i].ToolCalls) - 1; j >= 0; j-- {
			if messages[i].ToolCalls[j].Name == name {
				return append(json.RawMessage(nil), messages[i].ToolCalls[j].Arguments...), true
			}
		}
	}
	return nil, false
}

func latestMessageRole(messages []Message) Role {
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Role
}

func latestUserText(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

func isAffirmative(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, phrase := range []string{"yes", "confirm", "confirmed", "go ahead", "book it", "please do", "ja"} {
		if normalized == phrase || strings.HasPrefix(normalized, phrase+" ") ||
			strings.HasPrefix(normalized, phrase+",") || strings.HasPrefix(normalized, phrase+"!") ||
			strings.HasPrefix(normalized, phrase+".") {
			return true
		}
	}
	return false
}

func jsonStringAt(object map[string]any, path ...string) string {
	var value any = object
	for _, part := range path {
		mapping, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value = mapping[part]
	}
	result, _ := value.(string)
	return result
}

func findFirstString(value any, key string) string {
	switch typed := value.(type) {
	case map[string]any:
		if result, ok := typed[key].(string); ok {
			return result
		}
		for _, child := range typed {
			if result := findFirstString(child, key); result != "" {
				return result
			}
		}
	case []any:
		for _, child := range typed {
			if result := findFirstString(child, key); result != "" {
				return result
			}
		}
	}
	return ""
}
