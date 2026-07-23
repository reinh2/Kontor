package demohttp

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// ssePollInterval bounds how quickly a connected widget observes a new
	// committed turn. Events are durable rows, so polling — not push — keeps
	// each stream on the shared pool without a dedicated LISTEN connection.
	ssePollInterval = 1 * time.Second
	// sseHeartbeatInterval keeps intermediaries from timing out a quiet
	// stream; the widget ignores comment frames.
	sseHeartbeatInterval = 15 * time.Second
	// sseMaxStreamAge closes healthy streams so a restarted or drained server
	// sheds connections predictably. Clients resume with Last-Event-ID.
	sseMaxStreamAge = 10 * time.Minute
	// sseWriteTimeout bounds one frame write; the ResponseController deadline
	// overrides the server-wide write timeout that would otherwise kill a
	// long-lived stream.
	sseWriteTimeout = 10 * time.Second
	sseBatchLimit   = 100
)

// streamEvents replays the conversation's durable events after Last-Event-ID
// and then follows the stream until the client disconnects or the stream age
// cap is reached. Events are only ever written by committed transactions, so
// a replayed event can never describe an outcome that later disappears.
func (h *Handler) streamEvents(w http.ResponseWriter, r *http.Request) {
	conversationID := r.PathValue("conversationID")
	if !h.requireConversationCapability(w, r, conversationID) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "streaming unsupported", "The server connection cannot stream events")
		return
	}
	cursor, err := lastEventID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid cursor", "Last-Event-ID must be a non-negative integer")
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	// Tells nginx to pass frames through instead of buffering the response.
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	controller := http.NewResponseController(w)
	writeFrame := func(frame string) error {
		_ = controller.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
		if _, err := fmt.Fprint(w, frame); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	if err := writeFrame(": connected\n\n"); err != nil {
		return
	}

	started := time.Now()
	poll := time.NewTicker(ssePollInterval)
	defer poll.Stop()
	lastActivity := time.Now()
	for {
		events, err := h.app.ConversationEvents(r.Context(), conversationID, cursor, sseBatchLimit)
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			// The stream cannot report a mid-stream error as a status code;
			// close it and let the client reconnect from its cursor.
			h.logger.Error("sse event load failed", "conversation_id", conversationID, "error", err)
			return
		}
		for _, event := range events {
			payload := strings.ReplaceAll(string(event.Payload), "\n", "")
			frame := fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, payload)
			if err := writeFrame(frame); err != nil {
				return
			}
			cursor = event.ID
			lastActivity = time.Now()
		}
		if time.Since(started) > sseMaxStreamAge {
			_ = writeFrame(": stream age limit reached, reconnect to resume\n\n")
			return
		}
		if time.Since(lastActivity) > sseHeartbeatInterval {
			if err := writeFrame(": heartbeat\n\n"); err != nil {
				return
			}
			lastActivity = time.Now()
		}
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
		}
	}
}

// lastEventID reads the SSE resume cursor. The standard Last-Event-ID header
// is preferred; a last_event_id query parameter is accepted for clients that
// cannot set headers. Zero replays the whole stream.
func lastEventID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("last_event_id"))
	}
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0, fmt.Errorf("Last-Event-ID must be a non-negative integer")
	}
	return id, nil
}
