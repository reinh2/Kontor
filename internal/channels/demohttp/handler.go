// Package demohttp exposes the Stage 1 conversation-to-booking path. The
// embeddable widget and SSE transport arrive in Stage 2.
package demohttp

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/kontor/internal/agenttrace"
	"github.com/reinhlord/kontor/internal/app"
	"github.com/reinhlord/kontor/internal/conversations"
	"github.com/reinhlord/kontor/internal/platform/ids"
)

type Handler struct {
	app    *app.Service
	trace  *agenttrace.Store
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func New(application *app.Service, trace *agenttrace.Store, pool *pgxpool.Pool, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{app: application, trace: trace, pool: pool, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /readyz", h.ready)
	mux.HandleFunc("POST /api/v1/demo/conversations", h.createConversation)
	mux.HandleFunc("POST /api/v1/demo/conversations/{conversationID}/messages", h.sendMessage)
	mux.HandleFunc("GET /api/v1/demo/runs/{runID}", h.getRun)
	return h.recover(mux)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	if err := h.pool.Ping(r.Context()); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "not ready", "PostgreSQL is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) createConversation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
		Phone       string `json:"phone"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid request", err.Error())
		return
	}
	created, err := h.app.CreateConversation(r.Context(), conversations.Profile{
		DisplayName: input.DisplayName, Email: input.Email, Phone: input.Phone,
	})
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "conversation rejected", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"conversation_id": created.ID,
		"customer_id":     created.CustomerID,
		"token_budget":    created.TokenBudget,
		"tenant_scope":    "fixed",
	})
}

func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("conversationID")
	var input struct {
		ClientMessageID string `json:"client_message_id"`
		Text            string `json:"text"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid request", err.Error())
		return
	}
	if input.ClientMessageID == "" {
		input.ClientMessageID = ids.New()
	}
	result, err := h.app.SendMessage(r.Context(), conversationID, input.Text, input.ClientMessageID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, conversations.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeProblem(w, status, "turn failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.trace.GetRun(r.Context(), r.PathValue("runID"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "run not found", "The requested run does not exist")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "trace failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				h.logger.Error("http panic", "method", r.Method, "path", r.URL.Path, "error", recovered)
				writeProblem(w, http.StatusInternalServerError, "internal error", "The request could not be completed")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(contentType, "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	if len(detail) > 1000 {
		detail = detail[:1000]
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "about:blank", "title": title, "status": status, "detail": detail,
	})
}
