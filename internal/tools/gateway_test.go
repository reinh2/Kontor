package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const (
	testTenant       = "11111111-1111-4111-8111-111111111111"
	testCustomer     = "22222222-2222-4222-8222-222222222222"
	testConversation = "33333333-3333-4333-8333-333333333333"
	testService      = "44444444-4444-4444-8444-444444444444"
	testStaff        = "55555555-5555-4555-8555-555555555555"
	testBooking      = "66666666-6666-4666-8666-666666666666"
)

var testNow = time.Date(2026, time.July, 22, 9, 0, 0, 0, time.UTC)

type fakeBackend struct {
	services    []Service
	staff       []Staff
	slots       []AvailableSlot
	createCalls int
	lastCreate  CreateBookingCommand
	createErr   error
	findQuery   FindSlotsQuery
	findCalls   int
	onFind      func()
	escalations []EscalationCommand
}

func (b *fakeBackend) ListServices(context.Context, string) ([]Service, error) {
	return b.services, nil
}

func (b *fakeBackend) ListStaff(context.Context, string, string) ([]Staff, error) {
	return b.staff, nil
}

func (b *fakeBackend) FindSlots(_ context.Context, query FindSlotsQuery) ([]AvailableSlot, error) {
	b.findCalls++
	b.findQuery = query
	if b.onFind != nil {
		b.onFind()
	}
	return b.slots, nil
}

func (b *fakeBackend) CreateBooking(_ context.Context, command CreateBookingCommand) (CreateBookingOutcome, error) {
	b.createCalls++
	b.lastCreate = command
	if b.createErr != nil {
		return CreateBookingOutcome{}, b.createErr
	}
	return CreateBookingOutcome{
		Booking: Booking{
			ID: testBooking, Status: "confirmed", ServiceID: command.ServiceID,
			ServiceName: "Consultation", StaffID: command.StaffID, StaffName: "Ada",
			StartAt: command.StartAt, EndAt: command.EndAt, Timezone: command.Timezone,
			CustomerDisplayName: command.Customer.DisplayName, Version: 1,
		},
		CalendarSync:        "queued",
		IdempotencyReplayed: b.createCalls > 1,
	}, nil
}

func (b *fakeBackend) Escalate(_ context.Context, command EscalationCommand) (EscalationOutcome, error) {
	b.escalations = append(b.escalations, command)
	return EscalationOutcome{
		ID: "99999999-9999-4999-8999-999999999999", Status: "open",
		Replayed: len(b.escalations) > 1,
	}, nil
}

func testTrusted(messageID string) TrustedContext {
	return TrustedContext{
		TenantID: testTenant, CustomerID: testCustomer,
		CustomerDisplayName: "Persisted Customer", CustomerEmail: "persisted@example.com",
		ConversationID:   testConversation,
		InboundMessageID: messageID,
		Capabilities: map[Capability]bool{
			CapabilityScheduleRead: true, CapabilityBookingCreateSelf: true,
			CapabilityBookingWriteSelf: true, CapabilityCRMContactSelf: true,
			CapabilityCRMDealAfterBook: true, CapabilityConversationEscalate: true,
		},
	}
}

func newTestGateway(t *testing.T, backend *fakeBackend, store ConfirmationStore) *Gateway {
	t.Helper()
	gateway, err := NewGateway(Config{
		Backend: backend, Confirmations: store,
		SlotSigningKey: []byte("0123456789abcdef0123456789abcdef"),
		Now:            func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	return gateway
}

type failingMarkConsumedStore struct {
	*MemoryConfirmationStore
}

func (s failingMarkConsumedStore) MarkConsumed(context.Context, string, ConfirmationBinding, time.Time) error {
	return errors.New("simulated confirmation persistence failure")
}

func rawArguments(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal arguments: %v", err)
	}
	return raw
}

func testSlotToken(t *testing.T, gateway *Gateway, trusted TrustedContext) string {
	t.Helper()
	token, err := gateway.signer.Sign(SlotClaims{
		TenantID: trusted.TenantID, ConversationID: trusted.ConversationID,
		ServiceID: testService, ServiceName: "Consultation",
		StaffID: testStaff, StaffName: "Ada",
		StartAt: testNow.Add(24 * time.Hour), EndAt: testNow.Add(25 * time.Hour),
		Timezone: "Europe/Berlin", ExpiresAt: testNow.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("sign slot: %v", err)
	}
	return token
}

func validCreateArguments(token string) map[string]any {
	return map[string]any{
		"slot_token": token,
		"customer": map[string]any{
			"display_name": "Alex", "contact": map[string]any{"email": "alex@example.com"},
		},
		"idempotency_key": "booking-request-0001",
	}
}

func TestDefinitionsAreExactAllowlist(t *testing.T) {
	definitions := Definitions()
	want := []string{
		ToolListServices, ToolListStaff, ToolFindSlots, ToolCreateBooking,
		ToolReschedule, ToolCancel, ToolUpsertContact, ToolCreateDeal, ToolEscalate,
		ToolRespondToCustomer,
	}
	if len(definitions) != len(want) {
		t.Fatalf("got %d definitions, want %d", len(definitions), len(want))
	}
	for i := range want {
		if definitions[i].Name != want[i] || definitions[i].Version != ContractVersion {
			t.Fatalf("definition %d = %s@%s", i, definitions[i].Name, definitions[i].Version)
		}
		if !json.Valid(definitions[i].Parameters) {
			t.Fatalf("definition %s has invalid JSON Schema", definitions[i].Name)
		}
	}
}

func TestRespondToCustomerContractIsStrict(t *testing.T) {
	t.Parallel()
	valid := json.RawMessage(`{"disposition":"clarification_needed","message":"Which service would you like?"}`)
	arguments, err := ParseRespondToCustomerArguments(valid)
	if err != nil {
		t.Fatal(err)
	}
	if arguments.Disposition != ResponseClarificationNeeded || arguments.Message != "Which service would you like?" {
		t.Fatalf("arguments = %#v", arguments)
	}

	invalid := []json.RawMessage{
		json.RawMessage(`{"disposition":"complete","message":"ok","extra":true}`),
		json.RawMessage(`{"disposition":"complete","disposition":"clarification_needed","message":"question"}`),
		json.RawMessage(`{"disposition":"unknown","message":"question"}`),
		json.RawMessage(`{"disposition":"complete","message":"   "}`),
		json.RawMessage(`{"disposition":"complete","message":"` + strings.Repeat("x", 2001) + `"}`),
	}
	for index, raw := range invalid {
		if _, err := ParseRespondToCustomerArguments(raw); err == nil {
			t.Fatalf("invalid contract case %d was accepted", index)
		}
	}
}

func TestGatewayRejectsUnknownInvalidAndDuplicateArguments(t *testing.T) {
	gateway := newTestGateway(t, &fakeBackend{}, NewMemoryConfirmationStore())
	trusted := testTrusted("message-1")

	tests := []struct {
		name string
		call Call
		code ErrorCode
	}{
		{"unknown", Call{ID: "1", Name: "run_shell", Arguments: json.RawMessage(`{}`)}, CodeToolNotAllowed},
		{"format assertion", Call{ID: "2", Name: ToolListStaff, Arguments: json.RawMessage(`{"service_id":"not-a-uuid"}`)}, CodeInvalidArgument},
		{"extra property", Call{ID: "3", Name: ToolListServices, Arguments: json.RawMessage(`{"foo":true}`)}, CodeInvalidArgument},
		{"duplicate property", Call{ID: "4", Name: ToolListStaff, Arguments: json.RawMessage(fmt.Sprintf(`{"service_id":%q,"service_id":%q}`, testService, testService))}, CodeInvalidArgument},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := gateway.Execute(context.Background(), trusted, test.call)
			if result.Status != StatusError || result.Error == nil || result.Error.Code != test.code {
				t.Fatalf("result = %#v, want error %s", result, test.code)
			}
		})
	}
}

func TestGatewayRejectsInjectedIdentityAtAnyDepth(t *testing.T) {
	backend := &fakeBackend{}
	gateway := newTestGateway(t, backend, NewMemoryConfirmationStore())

	for _, raw := range []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`{"service_id":%q,"tenant_id":%q}`, testService, testTenant)),
		json.RawMessage(`{"slot_token":"slt_v1_aaaaaaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbb","customer":{"display_name":"Alex","contact":{"email":"alex@example.com"},"owner_id":"victim"},"idempotency_key":"booking-request-0001"}`),
	} {
		name := ToolListStaff
		if string(raw[2:12]) == "slot_token" {
			name = ToolCreateBooking
		}
		result := gateway.Execute(context.Background(), testTrusted("message-1"), Call{Name: name, Arguments: raw})
		if result.Error == nil || result.Error.Code != CodeInvalidArgument {
			t.Fatalf("result = %#v, want INVALID_ARGUMENT", result)
		}
	}
	if backend.createCalls != 0 {
		t.Fatalf("backend mutated %d times", backend.createCalls)
	}
}

func TestGatewayEnforcesServerCapabilities(t *testing.T) {
	backend := &fakeBackend{}
	gateway := newTestGateway(t, backend, NewMemoryConfirmationStore())
	trusted := testTrusted("message-1")
	delete(trusted.Capabilities, CapabilityScheduleRead)
	result := gateway.Execute(context.Background(), trusted, Call{
		Name: ToolListServices, Arguments: json.RawMessage(`{}`),
	})
	if result.Error == nil || result.Error.Code != CodePolicyDenied {
		t.Fatalf("result = %#v, want POLICY_DENIED", result)
	}
}

func TestSlotTokenTamperingAndScope(t *testing.T) {
	gateway := newTestGateway(t, &fakeBackend{}, NewMemoryConfirmationStore())
	trusted := testTrusted("message-1")
	token := testSlotToken(t, gateway, trusted)

	replacement := byte('A')
	if token[len(token)-1] == replacement {
		replacement = 'B'
	}
	tampered := token[:len(token)-1] + string(replacement)
	if _, err := gateway.signer.Verify(tampered, trusted, testNow); !errors.Is(err, ErrInvalidSlotToken) {
		t.Fatalf("tampered token error = %v", err)
	}
	otherConversation := trusted
	otherConversation.ConversationID = "77777777-7777-4777-8777-777777777777"
	if _, err := gateway.signer.Verify(token, otherConversation, testNow); !errors.Is(err, ErrSlotTokenScope) {
		t.Fatalf("cross-conversation token error = %v", err)
	}
}

func TestGatewayRejectsCrossConversationSlot(t *testing.T) {
	backend := &fakeBackend{}
	gateway := newTestGateway(t, backend, NewMemoryConfirmationStore())
	owner := testTrusted("message-1")
	arguments := validCreateArguments(testSlotToken(t, gateway, owner))
	attacker := owner
	attacker.ConversationID = "77777777-7777-4777-8777-777777777777"
	result := gateway.Execute(context.Background(), attacker, Call{
		Name: ToolCreateBooking, Arguments: rawArguments(t, arguments),
	})
	if result.Error == nil || result.Error.Code != CodePolicyDenied || backend.createCalls != 0 {
		t.Fatalf("result = %#v, calls = %d", result, backend.createCalls)
	}
}

func TestFindSlotsIssuesScopedToken(t *testing.T) {
	backend := &fakeBackend{slots: []AvailableSlot{{
		ServiceID: testService, ServiceName: "Consultation", StaffID: testStaff,
		StaffName: "Ada", StartAt: testNow.Add(24 * time.Hour),
		EndAt: testNow.Add(25 * time.Hour), Timezone: "Europe/Berlin",
	}}}
	gateway := newTestGateway(t, backend, NewMemoryConfirmationStore())
	arguments := map[string]any{
		"service_id": testService,
		"date_from":  testNow.Add(23 * time.Hour).Format(time.RFC3339),
		"date_to":    testNow.Add(26 * time.Hour).Format(time.RFC3339),
	}
	result := gateway.Execute(context.Background(), testTrusted("message-1"), Call{
		ID: "find-1", Name: ToolFindSlots, Arguments: rawArguments(t, arguments),
	})
	if result.Status != StatusSuccess {
		t.Fatalf("find result = %#v", result)
	}
	data, ok := result.Data.(FindSlotsData)
	if !ok || len(data.Slots) != 1 {
		t.Fatalf("find data = %#v", result.Data)
	}
	claims, err := gateway.signer.Verify(data.Slots[0].SlotToken, testTrusted("message-1"), testNow)
	if err != nil || claims.ServiceID != testService || claims.StaffID != testStaff {
		t.Fatalf("claims = %#v, err = %v", claims, err)
	}
}

func TestGatewayBookingWindowDefaultsAndConfigValidation(t *testing.T) {
	gateway := newTestGateway(t, &fakeBackend{}, NewMemoryConfirmationStore())
	if gateway.minBookingLeadTime != 15*time.Minute || gateway.maxBookingHorizon != 365*24*time.Hour {
		t.Fatalf("default booking window = lead %s horizon %s", gateway.minBookingLeadTime, gateway.maxBookingHorizon)
	}
	_, err := NewGateway(Config{
		Backend: &fakeBackend{}, Confirmations: NewMemoryConfirmationStore(),
		SlotSigningKey:     []byte("0123456789abcdef0123456789abcdef"),
		MinBookingLeadTime: 2 * time.Hour, MaxBookingHorizon: time.Hour,
	})
	if err == nil {
		t.Fatal("expected horizon shorter than lead time to be rejected")
	}
	configured, err := NewGateway(Config{
		Backend: &fakeBackend{}, Confirmations: NewMemoryConfirmationStore(),
		SlotSigningKey:     []byte("0123456789abcdef0123456789abcdef"),
		MinBookingLeadTime: 2 * time.Hour, MaxBookingHorizon: 30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if configured.minBookingLeadTime != 2*time.Hour || configured.maxBookingHorizon != 30*24*time.Hour {
		t.Fatalf("configured booking window = lead %s horizon %s", configured.minBookingLeadTime, configured.maxBookingHorizon)
	}
}

func TestFindSlotsRejectsPastTooSoonAndFarFutureSearches(t *testing.T) {
	tests := []struct {
		name string
		from time.Time
		to   time.Time
	}{
		{name: "past", from: testNow.Add(-time.Hour), to: testNow.Add(time.Hour)},
		{name: "too soon", from: testNow.Add(14 * time.Minute), to: testNow.Add(time.Hour)},
		{name: "far future", from: testNow.Add(364 * 24 * time.Hour), to: testNow.Add(365*24*time.Hour + time.Minute)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeBackend{}
			gateway := newTestGateway(t, backend, NewMemoryConfirmationStore())
			result := gateway.Execute(context.Background(), testTrusted("message-1"), Call{
				Name: ToolFindSlots,
				Arguments: rawArguments(t, map[string]any{
					"service_id": testService,
					"date_from":  test.from.Format(time.RFC3339),
					"date_to":    test.to.Format(time.RFC3339),
				}),
			})
			if result.Error == nil || result.Error.Code != CodeInvalidArgument {
				t.Fatalf("result = %#v, want INVALID_ARGUMENT", result)
			}
			if backend.findCalls != 0 {
				t.Fatalf("unsafe search reached backend %d times", backend.findCalls)
			}
		})
	}
}

func TestFindSlotsDoesNotSignCandidateThatAgesInsideLeadTime(t *testing.T) {
	clock := testNow
	backend := &fakeBackend{slots: []AvailableSlot{{
		ServiceID: testService, ServiceName: "Consultation", StaffID: testStaff,
		StaffName: "Ada", StartAt: testNow.Add(16 * time.Minute),
		EndAt: testNow.Add(46 * time.Minute), Timezone: "Europe/Berlin",
	}}}
	backend.onFind = func() { clock = testNow.Add(2 * time.Minute) }
	gateway, err := NewGateway(Config{
		Backend: backend, Confirmations: NewMemoryConfirmationStore(),
		SlotSigningKey: []byte("0123456789abcdef0123456789abcdef"),
		Now:            func() time.Time { return clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	result := gateway.Execute(context.Background(), testTrusted("message-1"), Call{
		Name: ToolFindSlots,
		Arguments: rawArguments(t, map[string]any{
			"service_id": testService,
			"date_from":  testNow.Add(15 * time.Minute).Format(time.RFC3339),
			"date_to":    testNow.Add(time.Hour).Format(time.RFC3339),
		}),
	})
	data, ok := result.Data.(FindSlotsData)
	if result.Status != StatusSuccess || !ok || len(data.Slots) != 0 {
		t.Fatalf("aged slot result = %#v, want safe empty success", result)
	}
}

func TestFindSlotsCapsTokenExpiryAtLeadTimeBoundary(t *testing.T) {
	start := testNow.Add(18 * time.Minute)
	backend := &fakeBackend{slots: []AvailableSlot{{
		ServiceID: testService, ServiceName: "Consultation", StaffID: testStaff,
		StaffName: "Ada", StartAt: start, EndAt: start.Add(30 * time.Minute),
		Timezone: "Europe/Berlin",
	}}}
	gateway := newTestGateway(t, backend, NewMemoryConfirmationStore())
	result := gateway.Execute(context.Background(), testTrusted("message-1"), Call{
		Name: ToolFindSlots,
		Arguments: rawArguments(t, map[string]any{
			"service_id": testService,
			"date_from":  testNow.Add(15 * time.Minute).Format(time.RFC3339),
			"date_to":    testNow.Add(time.Hour).Format(time.RFC3339),
		}),
	})
	data, ok := result.Data.(FindSlotsData)
	if result.Status != StatusSuccess || !ok || len(data.Slots) != 1 {
		t.Fatalf("result = %#v", result)
	}
	wantExpiry := start.Add(-15 * time.Minute)
	if !data.Slots[0].ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("token expiry = %s, want lead boundary %s", data.Slots[0].ExpiresAt, wantExpiry)
	}
	claims, err := gateway.signer.Verify(data.Slots[0].SlotToken, testTrusted("message-1"), testNow)
	if err != nil || !claims.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("signed claims = %#v, err = %v", claims, err)
	}
}

func TestCreateBookingRejectsPastTooSoonAndFarFutureSlotClaims(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
	}{
		{name: "past", start: testNow.Add(-time.Hour)},
		{name: "too soon", start: testNow.Add(14 * time.Minute)},
		{name: "far future", start: testNow.Add(366 * 24 * time.Hour)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeBackend{}
			store := NewMemoryConfirmationStore()
			gateway := newTestGateway(t, backend, store)
			trusted := testTrusted("message-1")
			token, err := gateway.signer.Sign(SlotClaims{
				TenantID: trusted.TenantID, ConversationID: trusted.ConversationID,
				ServiceID: testService, StaffID: testStaff,
				StartAt: test.start, EndAt: test.start.Add(30 * time.Minute),
				Timezone: "Europe/Berlin", ExpiresAt: testNow.Add(5 * time.Minute),
			})
			if err != nil {
				t.Fatal(err)
			}
			result := gateway.Execute(context.Background(), trusted, Call{
				Name: ToolCreateBooking, Arguments: rawArguments(t, validCreateArguments(token)),
			})
			if result.Error == nil || result.Error.Code != CodeSlotUnavailable {
				t.Fatalf("result = %#v, want SLOT_UNAVAILABLE", result)
			}
			if result.Confirmation != nil || backend.createCalls != 0 {
				t.Fatalf("unsafe claim proposed or executed: result=%#v calls=%d", result, backend.createCalls)
			}
		})
	}
}

func TestCreateBookingRequiresBoundAuthorizationAndUsesTrustedOwner(t *testing.T) {
	backend := &fakeBackend{}
	store := NewMemoryConfirmationStore()
	gateway := newTestGateway(t, backend, store)
	requestContext := testTrusted("message-1")
	arguments := validCreateArguments(testSlotToken(t, gateway, requestContext))

	proposalResult := gateway.Execute(context.Background(), requestContext, Call{
		ID: "create-1", Name: ToolCreateBooking, Arguments: rawArguments(t, arguments),
	})
	if proposalResult.Status != StatusConfirmationRequired || proposalResult.Confirmation == nil {
		t.Fatalf("proposal result = %#v", proposalResult)
	}
	if backend.createCalls != 0 {
		t.Fatal("booking executed before confirmation")
	}
	latest, found, err := store.Latest(context.Background(), testTenant, testCustomer, testConversation, testNow)
	if err != nil || !found || !json.Valid(latest.Binding.ArgumentsJSON) {
		t.Fatalf("latest = %#v, found = %v, err = %v", latest, found, err)
	}
	var frozen map[string]any
	_ = json.Unmarshal(latest.Binding.ArgumentsJSON, &frozen)
	if _, exists := frozen["confirmation_id"]; exists {
		t.Fatal("frozen arguments include confirmation_id")
	}
	frozenCustomer, ok := frozen["customer"].(map[string]any)
	if !ok || frozenCustomer["display_name"] != requestContext.CustomerDisplayName {
		t.Fatalf("frozen customer = %#v, want trusted customer %q", frozen["customer"], requestContext.CustomerDisplayName)
	}
	if got := proposalResult.Confirmation.Facts[len(proposalResult.Confirmation.Facts)-1]; got.Label != "Customer" || got.Value != requestContext.CustomerDisplayName {
		t.Fatalf("confirmation customer fact = %#v, want trusted customer", got)
	}

	arguments["confirmation_id"] = proposalResult.Confirmation.ID
	unconfirmed := gateway.Execute(context.Background(), testTrusted("message-2"), Call{
		ID: "create-2", Name: ToolCreateBooking, Arguments: rawArguments(t, arguments),
	})
	if unconfirmed.Error == nil || unconfirmed.Error.Code != CodeConfirmationInvalid || backend.createCalls != 0 {
		t.Fatalf("unconfirmed result = %#v, calls = %d", unconfirmed, backend.createCalls)
	}
	confirmedContext := testTrusted("message-2")
	if err := store.Authorize(context.Background(), proposalResult.Confirmation.ID, confirmedContext, testNow); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	// Even a different, schema-valid model identity cannot change the frozen
	// action or the backend customer after the customer authorizes it.
	arguments["customer"] = map[string]any{
		"display_name": "Model-Supplied Mallory",
		"contact":      map[string]any{"phone": "+4915222222222"},
	}
	executed := gateway.Execute(context.Background(), confirmedContext, Call{
		ID: "create-3", Name: ToolCreateBooking, Arguments: rawArguments(t, arguments),
	})
	if executed.Status != StatusSuccess || backend.createCalls != 1 {
		t.Fatalf("executed = %#v, calls = %d", executed, backend.createCalls)
	}
	if backend.lastCreate.OwnerCustomerID != testCustomer || backend.lastCreate.TenantID != testTenant ||
		backend.lastCreate.ConversationID != testConversation {
		t.Fatalf("backend received untrusted ownership: %#v", backend.lastCreate)
	}
	if backend.lastCreate.Customer.DisplayName != requestContext.CustomerDisplayName ||
		backend.lastCreate.Customer.Contact.Email != requestContext.CustomerEmail ||
		backend.lastCreate.Customer.Contact.Phone != requestContext.CustomerPhone {
		t.Fatalf("backend received model-supplied customer profile: %#v", backend.lastCreate.Customer)
	}

	// Exact retries are safe: consumed authorization plus the bound idempotency
	// key can only replay the same backend operation.
	replayed := gateway.Execute(context.Background(), confirmedContext, Call{
		ID: "create-4", Name: ToolCreateBooking, Arguments: rawArguments(t, arguments),
	})
	if replayed.Status != StatusSuccess || !replayed.Meta.IdempotencyReplayed || backend.createCalls != 2 {
		t.Fatalf("replayed = %#v, calls = %d", replayed, backend.createCalls)
	}
}

func TestCreateBookingSignalsCommittedSideEffectWhenConfirmationFinalizationFails(t *testing.T) {
	backend := &fakeBackend{}
	memory := NewMemoryConfirmationStore()
	store := failingMarkConsumedStore{MemoryConfirmationStore: memory}
	gateway := newTestGateway(t, backend, store)
	proposalContext := testTrusted("message-1")
	arguments := validCreateArguments(testSlotToken(t, gateway, proposalContext))
	proposal := gateway.Execute(context.Background(), proposalContext, Call{
		ID: "proposal", Name: ToolCreateBooking, Arguments: rawArguments(t, arguments),
	})
	if proposal.Confirmation == nil {
		t.Fatalf("proposal = %#v", proposal)
	}
	confirmedContext := testTrusted("message-2")
	if err := memory.Authorize(context.Background(), proposal.Confirmation.ID, confirmedContext, testNow); err != nil {
		t.Fatal(err)
	}
	arguments["confirmation_id"] = proposal.Confirmation.ID
	result := gateway.Execute(context.Background(), confirmedContext, Call{
		ID: "commit", Name: ToolCreateBooking, Arguments: rawArguments(t, arguments),
	})
	if result.Status != StatusError || result.Error == nil || !result.Error.Retryable ||
		!result.SideEffectCommitted || backend.createCalls != 1 {
		t.Fatalf("result=%#v create calls=%d", result, backend.createCalls)
	}
}

func TestConfirmationRejectsChangedArgumentsAndCrossOwner(t *testing.T) {
	backend := &fakeBackend{}
	store := NewMemoryConfirmationStore()
	gateway := newTestGateway(t, backend, store)
	trusted := testTrusted("message-1")
	arguments := validCreateArguments(testSlotToken(t, gateway, trusted))
	proposal := gateway.Execute(context.Background(), trusted, Call{Name: ToolCreateBooking, Arguments: rawArguments(t, arguments)})
	if proposal.Confirmation == nil {
		t.Fatalf("proposal = %#v", proposal)
	}

	otherOwner := testTrusted("message-2")
	otherOwner.CustomerID = "88888888-8888-4888-8888-888888888888"
	if err := store.Authorize(context.Background(), proposal.Confirmation.ID, otherOwner, testNow); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("cross-owner authorization error = %v", err)
	}
	confirmed := testTrusted("message-2")
	if err := store.Authorize(context.Background(), proposal.Confirmation.ID, confirmed, testNow); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	arguments["notes"] = "changed after customer confirmation"
	arguments["confirmation_id"] = proposal.Confirmation.ID
	result := gateway.Execute(context.Background(), confirmed, Call{Name: ToolCreateBooking, Arguments: rawArguments(t, arguments)})
	if result.Error == nil || result.Error.Code != CodeConfirmationStale || backend.createCalls != 0 {
		t.Fatalf("result = %#v, calls = %d", result, backend.createCalls)
	}
}

func TestKnownLaterToolReturnsNotImplemented(t *testing.T) {
	gateway := newTestGateway(t, &fakeBackend{}, NewMemoryConfirmationStore())
	result := gateway.Execute(context.Background(), testTrusted("message-1"), Call{
		Name: ToolCancel,
		Arguments: json.RawMessage(`{"booking_id":"66666666-6666-4666-8666-666666666666",` +
			`"reason":"Plans changed","idempotency_key":"cancel-request-0001"}`),
	})
	if result.Error == nil || result.Error.Code != CodeNotImplemented {
		t.Fatalf("result = %#v", result)
	}
}

func TestGatewayEscalatesUsingOnlyTrustedConversationIdentity(t *testing.T) {
	backend := &fakeBackend{}
	gateway := newTestGateway(t, backend, NewMemoryConfirmationStore())
	trusted := testTrusted("message-1")
	trusted.AgentRunID = "77777777-7777-4777-8777-777777777777"
	call := Call{
		ID: "escalation-call-1", Name: ToolEscalate,
		Arguments: json.RawMessage(`{"reason":{"code":"customer_request","summary":"Please call me"}}`),
	}
	result := gateway.Execute(context.Background(), trusted, call)
	if result.Status != StatusSuccess || len(backend.escalations) != 1 {
		t.Fatalf("result=%#v escalations=%#v", result, backend.escalations)
	}
	command := backend.escalations[0]
	if command.TenantID != testTenant || command.OwnerCustomerID != testCustomer ||
		command.ConversationID != testConversation || command.AgentRunID != trusted.AgentRunID ||
		command.ToolCallID != call.ID || command.ReasonCode != "customer_request" {
		t.Fatalf("backend received untrusted escalation context: %#v", command)
	}
}
