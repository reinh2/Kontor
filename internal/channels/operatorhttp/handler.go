package operatorhttp

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Config struct {
	AdminToken string
	Session    Session
	Now        func() time.Time
}

type Handler struct {
	backend     Backend
	adminDigest [sha256.Size]byte
	session     Session
	location    *time.Location
	now         func() time.Time
	logger      *slog.Logger
}

func New(config Config, backend Backend, logger *slog.Logger) (http.Handler, error) {
	if backend == nil {
		return nil, errors.New("operator HTTP: nil backend")
	}
	if len(config.AdminToken) < 32 || len(config.AdminToken) > 512 {
		return nil, errors.New("operator HTTP: admin token must contain between 32 and 512 bytes")
	}
	location, err := time.LoadLocation(config.Session.Timezone)
	if err != nil {
		return nil, errors.New("operator HTTP: invalid tenant timezone")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	h := &Handler{
		backend: backend, adminDigest: sha256.Sum256([]byte(config.AdminToken)),
		session: config.Session, location: location, now: config.Now, logger: logger,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/operator/session", h.getSession)
	mux.HandleFunc("GET /api/v1/operator/dashboard", h.getDashboard)
	mux.HandleFunc("GET /api/v1/operator/runs", h.listRuns)
	mux.HandleFunc("GET /api/v1/operator/runs/{runID}", h.getRun)
	mux.HandleFunc("GET /api/v1/operator/calendar", h.getCalendar)
	return h.recover(h.noStore(h.authenticate(mux))), nil
}

func (h *Handler) getSession(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.session)
}

func (h *Handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	days := 30
	switch value := r.URL.Query().Get("range"); value {
	case "", "30d":
	case "7d":
		days = 7
	case "90d":
		days = 90
	default:
		writeProblem(w, http.StatusBadRequest, "invalid range", "range must be one of 7d, 30d, or 90d")
		return
	}
	result, err := h.backend.Dashboard(r.Context(), DashboardRequest{Days: days})
	if err != nil {
		h.internalError(w, r, "dashboard query failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	request := ListRunsRequest{
		Cursor:  strings.TrimSpace(query.Get("cursor")),
		Status:  strings.TrimSpace(query.Get("status")),
		Channel: strings.TrimSpace(query.Get("channel")),
		Query:   strings.TrimSpace(query.Get("q")),
		Limit:   50,
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			writeProblem(w, http.StatusBadRequest, "invalid limit", "limit must be an integer between 1 and 100")
			return
		}
		request.Limit = limit
	}
	if request.Status != "" && !allowedRunStatus(request.Status) {
		writeProblem(w, http.StatusBadRequest, "invalid status", "status is not a supported agent run status")
		return
	}
	if request.Channel != "" && !allowedChannel(request.Channel) {
		writeProblem(w, http.StatusBadRequest, "invalid channel", "channel must be web, telegram, or demo")
		return
	}
	if len(request.Query) > 200 {
		writeProblem(w, http.StatusBadRequest, "invalid query", "q must not exceed 200 characters")
		return
	}
	if raw := strings.TrimSpace(query.Get("from")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid time range", "from must be an RFC3339 timestamp")
			return
		}
		request.From = &value
	}
	if raw := strings.TrimSpace(query.Get("to")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid time range", "to must be an RFC3339 timestamp")
			return
		}
		request.To = &value
	}
	if request.From != nil && request.To != nil && !request.From.Before(*request.To) {
		writeProblem(w, http.StatusBadRequest, "invalid time range", "from must be earlier than to")
		return
	}
	if request.Cursor != "" {
		if _, err := decodeRunCursor(request.Cursor); err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid cursor", "cursor is malformed or expired")
			return
		}
	}
	result, err := h.backend.ListRuns(r.Context(), request)
	if err != nil {
		h.internalError(w, r, "runs query failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("runID"))
	if !validUUID(runID) {
		writeProblem(w, http.StatusBadRequest, "invalid run id", "run id must be a UUID")
		return
	}
	result, err := h.backend.GetRun(r.Context(), runID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "run not found", "The requested run does not exist")
		return
	}
	if err != nil {
		h.internalError(w, r, "run trace query failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getCalendar(w http.ResponseWriter, r *http.Request) {
	fromValue := strings.TrimSpace(r.URL.Query().Get("from"))
	toValue := strings.TrimSpace(r.URL.Query().Get("to"))
	var fromLocal, toLocal time.Time
	var err error
	if fromValue == "" && toValue == "" {
		now := h.now().In(h.location)
		mondayOffset := (int(now.Weekday()) + 6) % 7
		fromLocal = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.location).
			AddDate(0, 0, -mondayOffset)
		toLocal = fromLocal.AddDate(0, 0, 7)
	} else {
		if fromValue == "" || toValue == "" {
			writeProblem(w, http.StatusBadRequest, "invalid calendar range", "from and to must be supplied together")
			return
		}
		fromLocal, err = time.ParseInLocation("2006-01-02", fromValue, h.location)
		if err == nil {
			toLocal, err = time.ParseInLocation("2006-01-02", toValue, h.location)
		}
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "invalid calendar range", "from and to must use YYYY-MM-DD")
			return
		}
	}
	if !fromLocal.Before(toLocal) || toLocal.After(fromLocal.AddDate(0, 0, 31)) {
		writeProblem(w, http.StatusBadRequest, "invalid calendar range", "calendar range must be positive and no longer than 31 days")
		return
	}
	result, err := h.backend.Calendar(r.Context(), CalendarRequest{From: fromLocal, To: toLocal})
	if err != nil {
		h.internalError(w, r, "calendar query failed", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		presented := sha256.Sum256([]byte(token))
		if !ok || subtle.ConstantTimeCompare(presented[:], h.adminDigest[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kontor-operator"`)
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "A valid operator Bearer token is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				h.logger.Error("operator HTTP panic", "method", r.Method, "path", r.URL.Path, "error", recovered)
				writeProblem(w, http.StatusInternalServerError, "internal error", "The operator request could not be completed")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	h.logger.Error(operation, "method", r.Method, "path", r.URL.Path, "error", err)
	writeProblem(w, http.StatusInternalServerError, "operator query failed", "The requested operator data could not be loaded")
}

func bearerToken(header string) (string, bool) {
	if len(header) > 1024 {
		return "", false
	}
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || fields[1] == "" {
		return "", false
	}
	return fields[1], true
}

func allowedRunStatus(status string) bool {
	switch status {
	case "running", "completed", "failed", "escalated", "budget_exhausted":
		return true
	default:
		return false
	}
}

func allowedChannel(channel string) bool {
	switch channel {
	case "web", "telegram", "demo":
		return true
	default:
		return false
	}
}

func validUUID(value string) bool {
	var parsed pgtype.UUID
	return parsed.Scan(value) == nil && parsed.Valid
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "about:blank", "title": title, "status": status, "detail": detail,
	})
}
