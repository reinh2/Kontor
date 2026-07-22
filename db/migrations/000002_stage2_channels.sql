-- Kontor Stage 2: channel delivery.
--
-- conversation_events is the durable per-conversation event stream behind the
-- widget's SSE transport.  Events are written in the same transaction as the
-- customer-visible outcome they describe, so a reconnecting client replaying
-- from Last-Event-ID can never observe an outcome that later disappears, and
-- a restart never loses an event for a committed turn.
--
-- telegram_updates records processed Telegram update identifiers so webhook
-- retries acknowledge without re-running an agent turn.

CREATE TABLE conversation_events (
    tenant_id       uuid NOT NULL,
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id uuid NOT NULL,
    event_type      text NOT NULL CHECK (event_type IN ('turn_completed')),
    payload_json    jsonb NOT NULL CHECK (pg_column_size(payload_json) <= 65536),
    created_at      timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id)
);

COMMENT ON TABLE conversation_events IS
    'Durable SSE stream. The identity column is the SSE event id; per-conversation ordering follows from the per-conversation turn serialization lock.';

CREATE INDEX conversation_events_stream_idx
    ON conversation_events (tenant_id, conversation_id, id);

CREATE TABLE telegram_updates (
    tenant_id   uuid NOT NULL,
    update_id   bigint NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, update_id),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id)
);

COMMENT ON TABLE telegram_updates IS
    'Webhook idempotency: an update_id that inserts with a conflict was already processed and is acknowledged without another agent turn.';
