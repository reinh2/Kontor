package operatorhttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const testOperatorAdminToken = "stage5-operator-admin-token-0123456789abcdef"

var (
	testOperatorSession = Session{
		TenantID:   "00000000-0000-4000-8000-000000000001",
		TenantName: "Salon Nord",
		Timezone:   "Europe/Berlin",
		Currency:   "EUR",
	}
	testOperatorNow = time.Date(2026, time.July, 22, 18, 30, 0, 0, time.UTC)
)

func TestAuthenticationRejectsBeforeCallingBackend(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "wrong", authorization: "Bearer another-operator-token-that-is-long-enough"},
		{name: "malformed scheme", authorization: "Basic " + testOperatorAdminToken},
		{name: "malformed fields", authorization: "Bearer one two"},
		{name: "conversation capability", authorization: "Bearer tPZUHj2sg4Sk7dVvGQ3B_cLkVHwKBt0SgK6VdA1QeYo"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeOperatorBackend{}
			handler := newOperatorTestHandler(t, backend)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/operator/dashboard", nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("WWW-Authenticate"); got != `Bearer realm="kontor-operator"` {
				t.Fatalf("WWW-Authenticate=%q", got)
			}
			if calls := backend.callCount(); calls != 0 {
				t.Fatalf("unauthorized request made %d backend calls", calls)
			}
		})
	}
}

func TestValidTokenReturnsSessionWithPrivateCacheHeaders(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/session")

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control=%q, want no-store", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options=%q, want nosniff", got)
	}
	if got := response.Header().Get("Cross-Origin-Resource-Policy"); got != "same-origin" {
		t.Fatalf("Cross-Origin-Resource-Policy=%q, want same-origin", got)
	}
	var session Session
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if session != testOperatorSession {
		t.Fatalf("session=%#v, want %#v", session, testOperatorSession)
	}
	if calls := backend.callCount(); calls != 0 {
		t.Fatalf("session request made %d backend calls", calls)
	}
}

func TestDashboardRangeSelection(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		days int
	}{
		{name: "default", days: 30},
		{name: "seven days", raw: "7d", days: 7},
		{name: "thirty days", raw: "30d", days: 30},
		{name: "ninety days", raw: "90d", days: 90},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeOperatorBackend{dashboardResult: Dashboard{PeriodDays: test.days}}
			handler := newOperatorTestHandler(t, backend)
			target := "/api/v1/operator/dashboard"
			if test.raw != "" {
				target += "?range=" + test.raw
			}

			response := serveOperatorRequest(handler, http.MethodGet, target)

			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if len(backend.dashboardRequests) != 1 || backend.dashboardRequests[0].Days != test.days {
				t.Fatalf("dashboard requests=%#v, want days=%d", backend.dashboardRequests, test.days)
			}
		})
	}
}

func TestDashboardRejectsUnsupportedRange(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/dashboard?range=all")

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if calls := backend.callCount(); calls != 0 {
		t.Fatalf("invalid range made %d backend calls", calls)
	}
}

func TestListRunsForwardsFiltersLimitAndCursor(t *testing.T) {
	cursor := encodeRunCursor(runCursor{
		StartedAt: time.Date(2026, time.July, 22, 14, 30, 0, 0, time.UTC),
		ID:        "11111111-1111-4111-8111-111111111111",
	})
	backend := &fakeOperatorBackend{runsResult: RunPage{Items: []RunSummary{}}}
	handler := newOperatorTestHandler(t, backend)
	target := "/api/v1/operator/runs?status=failed&channel=telegram&q=Ada+Lovelace&limit=25&cursor=" + cursor

	response := serveOperatorRequest(handler, http.MethodGet, target)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	want := ListRunsRequest{
		Limit: 25, Cursor: cursor, Status: "failed", Channel: "telegram", Query: "Ada Lovelace",
	}
	if len(backend.runRequests) != 1 || backend.runRequests[0] != want {
		t.Fatalf("run requests=%#v, want %#v", backend.runRequests, want)
	}
}

func TestListRunsDefaultsLimit(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/runs")

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.runRequests) != 1 || backend.runRequests[0].Limit != 50 {
		t.Fatalf("run requests=%#v, want default limit 50", backend.runRequests)
	}
}

func TestListRunsParsesRFC3339Bounds(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet,
		"/api/v1/operator/runs?from=2026-07-20T10:00:00Z&to=2026-07-22T10:00:00Z")

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.runRequests) != 1 || backend.runRequests[0].From == nil || backend.runRequests[0].To == nil {
		t.Fatalf("run requests=%#v", backend.runRequests)
	}
	if got := backend.runRequests[0].From.Format(time.RFC3339); got != "2026-07-20T10:00:00Z" {
		t.Fatalf("from=%s", got)
	}
	if got := backend.runRequests[0].To.Format(time.RFC3339); got != "2026-07-22T10:00:00Z" {
		t.Fatalf("to=%s", got)
	}
}

func TestListRunsRejectsInvalidFilters(t *testing.T) {
	invalidUUIDCursor := base64.RawURLEncoding.EncodeToString([]byte(
		`{"started_at":"2026-07-22T14:30:00Z","id":"not-a-uuid"}`,
	))
	trailingCursor := base64.RawURLEncoding.EncodeToString([]byte(
		`{"started_at":"2026-07-22T14:30:00Z","id":"11111111-1111-4111-8111-111111111111"}{}`,
	))
	tests := []struct {
		name  string
		query string
	}{
		{name: "non numeric limit", query: "limit=many"},
		{name: "zero limit", query: "limit=0"},
		{name: "excessive limit", query: "limit=101"},
		{name: "unknown status", query: "status=success"},
		{name: "unknown channel", query: "channel=email"},
		{name: "long search", query: "q=" + strings.Repeat("x", 201)},
		{name: "malformed cursor", query: "cursor=not-a-cursor"},
		{name: "cursor with invalid UUID", query: "cursor=" + invalidUUIDCursor},
		{name: "cursor with trailing JSON", query: "cursor=" + trailingCursor},
		{name: "malformed from", query: "from=yesterday"},
		{name: "malformed to", query: "to=tomorrow"},
		{name: "reversed time range", query: "from=2026-07-22T10:00:00Z&to=2026-07-20T10:00:00Z"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeOperatorBackend{}
			handler := newOperatorTestHandler(t, backend)
			response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/runs?"+test.query)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if calls := backend.callCount(); calls != 0 {
				t.Fatalf("invalid filters made %d backend calls", calls)
			}
		})
	}
}

func TestGetRunRejectsInvalidUUIDBeforeCallingBackend(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/runs/not-a-uuid")

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if calls := backend.callCount(); calls != 0 {
		t.Fatalf("invalid run ID made %d backend calls", calls)
	}
}

func TestGetRunMapsMissingRowToNotFound(t *testing.T) {
	backend := &fakeOperatorBackend{runErr: pgx.ErrNoRows}
	handler := newOperatorTestHandler(t, backend)
	const runID = "22222222-2222-4222-8222-222222222222"
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/runs/"+runID)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.runIDs) != 1 || backend.runIDs[0] != runID {
		t.Fatalf("run IDs=%#v, want %s", backend.runIDs, runID)
	}
}

func TestBackendFailureReturnsGenericProblemWithoutLeakingDetails(t *testing.T) {
	const sensitive = "postgres://operator:hunter2@database/kontor"
	backend := &fakeOperatorBackend{dashboardErr: errors.New(sensitive)}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/dashboard")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), sensitive) || strings.Contains(response.Body.String(), "hunter2") {
		t.Fatalf("response leaked backend error: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"title":"operator query failed"`) {
		t.Fatalf("response omitted generic problem title: %s", response.Body.String())
	}
}

func TestCalendarDefaultsToTenantLocalMonday(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/calendar")

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.calendarRequests) != 1 {
		t.Fatalf("calendar requests=%#v", backend.calendarRequests)
	}
	request := backend.calendarRequests[0]
	if got := request.From.Format("2006-01-02 15:04 MST"); got != "2026-07-20 00:00 CEST" {
		t.Fatalf("calendar from=%s", got)
	}
	if got := request.To.Format("2006-01-02 15:04 MST"); got != "2026-07-27 00:00 CEST" {
		t.Fatalf("calendar to=%s", got)
	}
}

func TestCalendarRejectsInvalidRanges(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "missing to", query: "from=2026-07-01"},
		{name: "missing from", query: "to=2026-07-08"},
		{name: "invalid date", query: "from=2026-07-01&to=not-a-date"},
		{name: "empty range", query: "from=2026-07-01&to=2026-07-01"},
		{name: "reversed range", query: "from=2026-07-08&to=2026-07-01"},
		{name: "longer than maximum", query: "from=2026-07-01&to=2026-08-02"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeOperatorBackend{}
			handler := newOperatorTestHandler(t, backend)
			response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/calendar?"+test.query)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if calls := backend.callCount(); calls != 0 {
				t.Fatalf("invalid range made %d backend calls", calls)
			}
		})
	}
}

func TestCalendarAcceptsMaximumRange(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet,
		"/api/v1/operator/calendar?from=2026-07-01&to=2026-08-01")

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.calendarRequests) != 1 {
		t.Fatalf("calendar requests=%#v", backend.calendarRequests)
	}
	request := backend.calendarRequests[0]
	if got := request.From.Format("2006-01-02"); got != "2026-07-01" {
		t.Fatalf("calendar from=%s", got)
	}
	if got := request.To.Format("2006-01-02"); got != "2026-08-01" {
		t.Fatalf("calendar to=%s", got)
	}
}

func TestNonMatchingRouteReturnsNotFoundWithoutBackendCall(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/not-a-route")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if calls := backend.callCount(); calls != 0 {
		t.Fatalf("non-matching route made %d backend calls", calls)
	}
}

func newOperatorTestHandler(t *testing.T, backend Backend) http.Handler {
	t.Helper()
	handler, err := New(Config{
		AdminToken: testOperatorAdminToken,
		Session:    testOperatorSession,
		Now:        func() time.Time { return testOperatorNow },
	}, backend, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func serveOperatorRequest(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("Authorization", "Bearer "+testOperatorAdminToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func serveOperatorJSON(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testOperatorAdminToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestListCustomersForwardsQueryAndLimit(t *testing.T) {
	backend := &fakeOperatorBackend{customersResult: CustomerList{Items: []Customer{}}}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/customers?q=Ada&limit=5")

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	want := CustomerListRequest{Query: "Ada", Limit: 5}
	if len(backend.customerRequests) != 1 || backend.customerRequests[0] != want {
		t.Fatalf("customer requests=%#v, want %#v", backend.customerRequests, want)
	}
}

func TestListCustomersDefaultsLimitAndRejectsInvalid(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/customers")

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.customerRequests) != 1 || backend.customerRequests[0].Limit != 20 {
		t.Fatalf("customer requests=%#v, want default limit 20", backend.customerRequests)
	}

	tests := []string{"limit=0", "limit=51", "limit=many", "q=" + strings.Repeat("x", 201)}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			backend := &fakeOperatorBackend{}
			handler := newOperatorTestHandler(t, backend)
			response := serveOperatorRequest(handler, http.MethodGet, "/api/v1/operator/customers?"+query)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if calls := backend.callCount(); calls != 0 {
				t.Fatalf("invalid query made %d backend calls", calls)
			}
		})
	}
}

const (
	testCommandCustomer = "51000000-0000-4000-8000-0000000000aa"
	testCommandService  = "81000000-0000-4000-8000-0000000000aa"
	testCommandStaff    = "82000000-0000-4000-8000-0000000000aa"
	testCommandBooking  = "91000000-0000-4000-8000-0000000000aa"
)

func TestCreateBookingForwardsParsedCommand(t *testing.T) {
	backend := &fakeOperatorBackend{createResult: Booking{ID: testCommandBooking, ScheduleVersion: 1}}
	handler := newOperatorTestHandler(t, backend)
	body := `{"customer_id":"` + testCommandCustomer + `","service_id":"` + testCommandService +
		`","staff_id":"` + testCommandStaff + `","starts_at":"2026-07-23T10:00:00Z","notes":"walk-in"}`

	response := serveOperatorJSON(handler, http.MethodPost, "/api/v1/operator/bookings", body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.createRequests) != 1 {
		t.Fatalf("create requests=%#v", backend.createRequests)
	}
	got := backend.createRequests[0]
	if got.CustomerID != testCommandCustomer || got.ServiceID != testCommandService ||
		got.StaffID != testCommandStaff || got.Notes != "walk-in" {
		t.Fatalf("create command=%#v", got)
	}
	if got.StartsAt.Format(time.RFC3339) != "2026-07-23T10:00:00Z" {
		t.Fatalf("starts_at=%s", got.StartsAt.Format(time.RFC3339))
	}
	var booking Booking
	if err := json.Unmarshal(response.Body.Bytes(), &booking); err != nil {
		t.Fatal(err)
	}
	if booking.ID != testCommandBooking {
		t.Fatalf("response booking=%#v", booking)
	}
}

func TestCreateBookingRejectsInvalidBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "not json", body: "{"},
		{name: "unknown field", body: `{"customer_id":"` + testCommandCustomer + `","service_id":"` + testCommandService + `","staff_id":"` + testCommandStaff + `","starts_at":"2026-07-23T10:00:00Z","bogus":true}`},
		{name: "bad customer uuid", body: `{"customer_id":"nope","service_id":"` + testCommandService + `","staff_id":"` + testCommandStaff + `","starts_at":"2026-07-23T10:00:00Z"}`},
		{name: "missing starts_at", body: `{"customer_id":"` + testCommandCustomer + `","service_id":"` + testCommandService + `","staff_id":"` + testCommandStaff + `"}`},
		{name: "bad starts_at", body: `{"customer_id":"` + testCommandCustomer + `","service_id":"` + testCommandService + `","staff_id":"` + testCommandStaff + `","starts_at":"soon"}`},
		{name: "short idempotency key", body: `{"customer_id":"` + testCommandCustomer + `","service_id":"` + testCommandService + `","staff_id":"` + testCommandStaff + `","starts_at":"2026-07-23T10:00:00Z","idempotency_key":"short"}`},
		{name: "trailing json", body: `{"customer_id":"` + testCommandCustomer + `","service_id":"` + testCommandService + `","staff_id":"` + testCommandStaff + `","starts_at":"2026-07-23T10:00:00Z"}{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeOperatorBackend{}
			handler := newOperatorTestHandler(t, backend)
			response := serveOperatorJSON(handler, http.MethodPost, "/api/v1/operator/bookings", test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if calls := backend.callCount(); calls != 0 {
				t.Fatalf("invalid body made %d backend calls", calls)
			}
		})
	}
}

func TestRescheduleBookingForwardsCommand(t *testing.T) {
	backend := &fakeOperatorBackend{rescheduleResult: Booking{ID: testCommandBooking, ScheduleVersion: 3}}
	handler := newOperatorTestHandler(t, backend)
	body := `{"expected_version":2,"starts_at":"2026-07-24T09:30:00Z"}`

	response := serveOperatorJSON(handler, http.MethodPost, "/api/v1/operator/bookings/"+testCommandBooking+"/reschedule", body)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.rescheduleRequests) != 1 {
		t.Fatalf("reschedule requests=%#v", backend.rescheduleRequests)
	}
	got := backend.rescheduleRequests[0]
	if got.BookingID != testCommandBooking || got.ExpectedVersion != 2 ||
		got.StartsAt.Format(time.RFC3339) != "2026-07-24T09:30:00Z" {
		t.Fatalf("reschedule command=%#v", got)
	}
}

func TestRescheduleBookingRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		target string
		body   string
	}{
		{name: "bad booking id", target: "/api/v1/operator/bookings/not-a-uuid/reschedule", body: `{"expected_version":1,"starts_at":"2026-07-24T09:30:00Z"}`},
		{name: "missing version", target: "/api/v1/operator/bookings/" + testCommandBooking + "/reschedule", body: `{"starts_at":"2026-07-24T09:30:00Z"}`},
		{name: "zero version", target: "/api/v1/operator/bookings/" + testCommandBooking + "/reschedule", body: `{"expected_version":0,"starts_at":"2026-07-24T09:30:00Z"}`},
		{name: "bad time", target: "/api/v1/operator/bookings/" + testCommandBooking + "/reschedule", body: `{"expected_version":1,"starts_at":"later"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeOperatorBackend{}
			handler := newOperatorTestHandler(t, backend)
			response := serveOperatorJSON(handler, http.MethodPost, test.target, test.body)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if calls := backend.callCount(); calls != 0 {
				t.Fatalf("invalid reschedule made %d backend calls", calls)
			}
		})
	}
}

func TestCancelBookingForwardsCommand(t *testing.T) {
	backend := &fakeOperatorBackend{cancelResult: Booking{ID: testCommandBooking, Status: "cancelled", ScheduleVersion: 3}}
	handler := newOperatorTestHandler(t, backend)
	body := `{"expected_version":2,"reason":"customer no longer needs it"}`

	response := serveOperatorJSON(handler, http.MethodPost, "/api/v1/operator/bookings/"+testCommandBooking+"/cancel", body)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(backend.cancelRequests) != 1 {
		t.Fatalf("cancel requests=%#v", backend.cancelRequests)
	}
	got := backend.cancelRequests[0]
	if got.BookingID != testCommandBooking || got.ExpectedVersion != 2 || got.Reason != "customer no longer needs it" {
		t.Fatalf("cancel command=%#v", got)
	}
}

func TestCancelBookingRejectsMissingReason(t *testing.T) {
	backend := &fakeOperatorBackend{}
	handler := newOperatorTestHandler(t, backend)
	response := serveOperatorJSON(handler, http.MethodPost, "/api/v1/operator/bookings/"+testCommandBooking+"/cancel", `{"expected_version":1,"reason":"  "}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if calls := backend.callCount(); calls != 0 {
		t.Fatalf("invalid cancel made %d backend calls", calls)
	}
}

func TestBookingCommandErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "version conflict", err: ErrVersionConflict, status: http.StatusConflict},
		{name: "slot unavailable", err: ErrSlotUnavailable, status: http.StatusConflict},
		{name: "state conflict", err: ErrBookingStateConflict, status: http.StatusConflict},
		{name: "not found", err: ErrBookingNotFound, status: http.StatusNotFound},
		{name: "invalid", err: ErrInvalidCommand, status: http.StatusBadRequest},
		{name: "internal", err: errors.New("boom"), status: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &fakeOperatorBackend{cancelErr: test.err}
			handler := newOperatorTestHandler(t, backend)
			response := serveOperatorJSON(handler, http.MethodPost, "/api/v1/operator/bookings/"+testCommandBooking+"/cancel", `{"expected_version":1,"reason":"done"}`)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.status == http.StatusInternalServerError && strings.Contains(response.Body.String(), "boom") {
				t.Fatalf("internal error leaked detail: %s", response.Body.String())
			}
		})
	}
}

type fakeOperatorBackend struct {
	dashboardRequests  []DashboardRequest
	runRequests        []ListRunsRequest
	runIDs             []string
	calendarRequests   []CalendarRequest
	customerRequests   []CustomerListRequest
	createRequests     []CreateBookingCommand
	rescheduleRequests []RescheduleBookingCommand
	cancelRequests     []CancelBookingCommand

	dashboardResult  Dashboard
	runsResult       RunPage
	runResult        RunDetail
	calendarResult   Calendar
	customersResult  CustomerList
	createResult     Booking
	rescheduleResult Booking
	cancelResult     Booking

	dashboardErr  error
	runsErr       error
	runErr        error
	calendarErr   error
	customersErr  error
	createErr     error
	rescheduleErr error
	cancelErr     error
}

func (f *fakeOperatorBackend) Dashboard(_ context.Context, request DashboardRequest) (Dashboard, error) {
	f.dashboardRequests = append(f.dashboardRequests, request)
	return f.dashboardResult, f.dashboardErr
}

func (f *fakeOperatorBackend) ListRuns(_ context.Context, request ListRunsRequest) (RunPage, error) {
	f.runRequests = append(f.runRequests, request)
	return f.runsResult, f.runsErr
}

func (f *fakeOperatorBackend) GetRun(_ context.Context, runID string) (RunDetail, error) {
	f.runIDs = append(f.runIDs, runID)
	return f.runResult, f.runErr
}

func (f *fakeOperatorBackend) Calendar(_ context.Context, request CalendarRequest) (Calendar, error) {
	f.calendarRequests = append(f.calendarRequests, request)
	return f.calendarResult, f.calendarErr
}

func (f *fakeOperatorBackend) ListCustomers(_ context.Context, request CustomerListRequest) (CustomerList, error) {
	f.customerRequests = append(f.customerRequests, request)
	return f.customersResult, f.customersErr
}

func (f *fakeOperatorBackend) CreateBooking(_ context.Context, command CreateBookingCommand) (Booking, error) {
	f.createRequests = append(f.createRequests, command)
	return f.createResult, f.createErr
}

func (f *fakeOperatorBackend) RescheduleBooking(_ context.Context, command RescheduleBookingCommand) (Booking, error) {
	f.rescheduleRequests = append(f.rescheduleRequests, command)
	return f.rescheduleResult, f.rescheduleErr
}

func (f *fakeOperatorBackend) CancelBooking(_ context.Context, command CancelBookingCommand) (Booking, error) {
	f.cancelRequests = append(f.cancelRequests, command)
	return f.cancelResult, f.cancelErr
}

func (f *fakeOperatorBackend) callCount() int {
	return len(f.dashboardRequests) + len(f.runRequests) + len(f.runIDs) + len(f.calendarRequests) +
		len(f.customerRequests) + len(f.createRequests) + len(f.rescheduleRequests) + len(f.cancelRequests)
}

var _ Backend = (*fakeOperatorBackend)(nil)
