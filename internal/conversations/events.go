package conversations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Event is one durable entry of a conversation's SSE stream. ID is the SSE
// event id clients replay from; writes happen inside the application-service
// transactions that persist the outcome the event describes.
type Event struct {
	ID        int64           `json:"id"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// EventsAfter returns up to limit events for one conversation with an id
// strictly greater than afterID, in id order. afterID zero replays from the
// beginning of the stream.
func (s *Store) EventsAfter(ctx context.Context, tenantID, conversationID string, afterID int64, limit int) ([]Event, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, event_type, payload_json, created_at
		FROM conversation_events
		WHERE tenant_id=$1 AND conversation_id=$2 AND id>$3
		ORDER BY id
		LIMIT $4`, tenantID, conversationID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("load conversation events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Type, &event.Payload, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation events: %w", err)
	}
	return events, nil
}
