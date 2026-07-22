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

type fakeOperatorBackend struct {
	dashboardRequests []DashboardRequest
	runRequests       []ListRunsRequest
	runIDs            []string
	calendarRequests  []CalendarRequest

	dashboardResult Dashboard
	runsResult      RunPage
	runResult       RunDetail
	calendarResult  Calendar

	dashboardErr error
	runsErr      error
	runErr       error
	calendarErr  error
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

func (f *fakeOperatorBackend) callCount() int {
	return len(f.dashboardRequests) + len(f.runRequests) + len(f.runIDs) + len(f.calendarRequests)
}

var _ Backend = (*fakeOperatorBackend)(nil)
