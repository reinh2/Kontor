package llm

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestDemoAdapterDrivesDeterministicBookingAndMultiToolDiscovery(t *testing.T) {
	t.Parallel()
	adapter, err := NewDemoAdapter(DemoConfig{Now: func() time.Time {
		return time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	}})
	if err != nil {
		t.Fatal(err)
	}
	messages := []Message{{Role: RoleUser, Content: "I need a haircut Thursday evening"}}

	discovery, err := adapter.Complete(context.Background(), Request{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovery.Message.ToolCalls) != 2 || discovery.Message.ToolCalls[0].Name != "list_services" || discovery.Message.ToolCalls[1].Name != "list_staff" {
		t.Fatalf("discovery calls = %#v", discovery.Message.ToolCalls)
	}
	messages = append(messages, discovery.Message,
		Message{Role: RoleTool, Name: "list_services", ToolCallID: discovery.Message.ToolCalls[0].ID, Content: `{"status":"success","data":{"services":[]}}`},
		Message{Role: RoleTool, Name: "list_staff", ToolCallID: discovery.Message.ToolCalls[1].ID, Content: `{"status":"success","data":{"staff":[]}}`},
	)

	find, err := adapter.Complete(context.Background(), Request{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if len(find.Message.ToolCalls) != 1 || find.Message.ToolCalls[0].Name != "find_slots" {
		t.Fatalf("find response = %#v", find)
	}
	var findArguments map[string]any
	if err := json.Unmarshal(find.Message.ToolCalls[0].Arguments, &findArguments); err != nil {
		t.Fatal(err)
	}
	if findArguments["date_from"] != "2026-07-23T17:00:00+02:00" {
		t.Fatalf("date_from = %v", findArguments["date_from"])
	}
	messages = append(messages, find.Message, Message{
		Role: RoleTool, Name: "find_slots", ToolCallID: find.Message.ToolCalls[0].ID,
		Content: `{"status":"success","data":{"slots":[{"slot_token":"slt_v1_demo.signature"}]}}`,
	})

	proposal, err := adapter.Complete(context.Background(), Request{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Message.ToolCalls) != 1 || proposal.Message.ToolCalls[0].Name != "create_booking" {
		t.Fatalf("proposal response = %#v", proposal)
	}
	originalArguments := append(json.RawMessage(nil), proposal.Message.ToolCalls[0].Arguments...)
	messages = append(messages, proposal.Message, Message{
		Role: RoleTool, Name: "create_booking", ToolCallID: proposal.Message.ToolCalls[0].ID,
		Content: `{"status":"confirmation_required","confirmation":{"id":"30000000-0000-4000-8000-000000000001"}}`,
	})

	requestConfirmation, err := adapter.Complete(context.Background(), Request{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if requestConfirmation.FinishReason != "tool_calls" || len(requestConfirmation.Message.ToolCalls) != 1 ||
		requestConfirmation.Message.ToolCalls[0].Name != "respond_to_customer" {
		t.Fatalf("confirmation response = %#v", requestConfirmation)
	}
	var confirmationTerminal map[string]string
	if err := json.Unmarshal(requestConfirmation.Message.ToolCalls[0].Arguments, &confirmationTerminal); err != nil {
		t.Fatal(err)
	}
	if confirmationTerminal["disposition"] != "complete" || confirmationTerminal["message"] == "" {
		t.Fatalf("confirmation terminal arguments = %#v", confirmationTerminal)
	}
	messages = append(messages, requestConfirmation.Message, Message{Role: RoleUser, Content: "Yes, confirm"})
	confirmed, err := adapter.Complete(context.Background(), Request{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if len(confirmed.Message.ToolCalls) != 1 || confirmed.Message.ToolCalls[0].Name != "create_booking" {
		t.Fatalf("confirmed response = %#v", confirmed)
	}
	var original, confirmedObject map[string]any
	if json.Unmarshal(originalArguments, &original) != nil || json.Unmarshal(confirmed.Message.ToolCalls[0].Arguments, &confirmedObject) != nil {
		t.Fatal("decode create arguments")
	}
	if original["idempotency_key"] != confirmedObject["idempotency_key"] || confirmedObject["confirmation_id"] == "" {
		t.Fatalf("confirmed arguments = %#v, original = %#v", confirmedObject, original)
	}

	messages = append(messages, confirmed.Message, Message{
		Role: RoleTool, Name: "create_booking", ToolCallID: confirmed.Message.ToolCalls[0].ID,
		Content: `{"status":"success","data":{"booking":{"id":"40000000-0000-4000-8000-000000000001"}}}`,
	})
	booked, err := adapter.Complete(context.Background(), Request{Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if booked.FinishReason != "tool_calls" || len(booked.Message.ToolCalls) != 1 ||
		booked.Message.ToolCalls[0].Name != "respond_to_customer" {
		t.Fatalf("booked terminal response = %#v", booked)
	}
	var bookedTerminal map[string]string
	if err := json.Unmarshal(booked.Message.ToolCalls[0].Arguments, &bookedTerminal); err != nil {
		t.Fatal(err)
	}
	if bookedTerminal["disposition"] != "complete" || bookedTerminal["message"] == "" {
		t.Fatalf("booked terminal arguments = %#v", bookedTerminal)
	}
}

func TestDemoAdapterExecutesServerAuthorizedActionMarkerWithoutPriorToolHistory(t *testing.T) {
	t.Parallel()
	adapter, err := NewDemoAdapter(DemoConfig{})
	if err != nil {
		t.Fatal(err)
	}
	action := `{"tool":"create_booking","arguments":{"slot_token":"slt_v1_demo.signature","confirmation_id":"30000000-0000-4000-8000-000000000001","idempotency_key":"demo-booking-0001","customer":{"display_name":"Demo Customer","contact":{"email":"demo@example.com"}}}}`
	response, err := adapter.Complete(context.Background(), Request{Messages: []Message{
		{Role: RoleUser, Content: "Yes, confirm"},
		{Role: RoleSystem, Content: AuthorizedActionPrefix + action},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].Name != "create_booking" {
		t.Fatalf("response = %#v", response)
	}
	var arguments map[string]any
	if err := json.Unmarshal(response.Message.ToolCalls[0].Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments["confirmation_id"] != "30000000-0000-4000-8000-000000000001" {
		t.Fatalf("arguments = %#v", arguments)
	}
}
