-- Kontor Stage 1 schema.
--
-- The application intentionally runs with one fixed tenant in Stages 1-3.  We
-- nevertheless carry tenant_id through every business key and foreign key so
-- that a later multi-tenant build cannot accidentally inherit unscoped data.
-- There is deliberately no tenant-management or onboarding schema here.

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;

CREATE TABLE tenants (
    id          uuid PRIMARY KEY,
    slug        text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    timezone    text NOT NULL DEFAULT 'Europe/Berlin',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE tenants IS
    'Schema boundary retained for future isolation; Stages 1-3 use the single fixed Kontor tenant and expose no tenant management.';

INSERT INTO tenants (id, slug, name, timezone)
VALUES ('00000000-0000-4000-8000-000000000001', 'salon-nord', 'Salon Nord', 'Europe/Berlin')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE services (
    tenant_id              uuid NOT NULL,
    id                     uuid NOT NULL DEFAULT gen_random_uuid(),
    slug                   text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    name                   text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    description            text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    duration_minutes       integer NOT NULL CHECK (duration_minutes BETWEEN 5 AND 1440),
    buffer_before_minutes  integer NOT NULL DEFAULT 0 CHECK (buffer_before_minutes BETWEEN 0 AND 240),
    buffer_after_minutes   integer NOT NULL DEFAULT 0 CHECK (buffer_after_minutes BETWEEN 0 AND 240),
    price_minor            bigint NOT NULL DEFAULT 0 CHECK (price_minor >= 0),
    currency               char(3) NOT NULL DEFAULT 'EUR' CHECK (currency = upper(currency)),
    active                 boolean NOT NULL DEFAULT true,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, slug),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id)
);

CREATE TABLE staff (
    tenant_id   uuid NOT NULL,
    id          uuid NOT NULL DEFAULT gen_random_uuid(),
    slug        text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 200),
    timezone    text NOT NULL DEFAULT 'Europe/Berlin',
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, slug),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id)
);

CREATE TABLE staff_services (
    tenant_id  uuid NOT NULL,
    staff_id   uuid NOT NULL,
    service_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, staff_id, service_id),
    FOREIGN KEY (tenant_id, staff_id) REFERENCES staff (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id) ON DELETE CASCADE
);

-- Rules use local wall-clock values in the staff member's IANA timezone.
-- day_of_week follows Go time.Weekday/PostgreSQL EXTRACT(DOW): Sunday = 0.
CREATE TABLE availability_rules (
    tenant_id    uuid NOT NULL,
    id           uuid NOT NULL DEFAULT gen_random_uuid(),
    staff_id     uuid NOT NULL,
    rule_type    text NOT NULL CHECK (rule_type IN ('working', 'break')),
    day_of_week  smallint NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    local_start  time NOT NULL,
    local_end    time NOT NULL,
    valid_from   date,
    valid_until  date,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, staff_id) REFERENCES staff (tenant_id, id) ON DELETE CASCADE,
    CHECK (local_start < local_end),
    CHECK (valid_until IS NULL OR valid_from IS NULL OR valid_until >= valid_from)
);

CREATE INDEX availability_rules_staff_day_idx
    ON availability_rules (tenant_id, staff_id, day_of_week);

-- Manual closures and, from Stage 3, provider busy intervals.  These are
-- availability inputs, not authoritative Kontor appointments.
CREATE TABLE schedule_blocks (
    tenant_id       uuid NOT NULL,
    id              uuid NOT NULL DEFAULT gen_random_uuid(),
    staff_id        uuid NOT NULL,
    starts_at       timestamptz NOT NULL,
    ends_at         timestamptz NOT NULL,
    kind            text NOT NULL DEFAULT 'manual' CHECK (kind IN ('manual', 'time_off', 'external_busy')),
    external_ref    text,
    note            text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, staff_id) REFERENCES staff (tenant_id, id) ON DELETE CASCADE,
    CHECK (starts_at < ends_at)
);

CREATE INDEX schedule_blocks_lookup_idx
    ON schedule_blocks USING gist (tenant_id, staff_id, tstzrange(starts_at, ends_at, '[)'));
CREATE UNIQUE INDEX schedule_blocks_external_ref_uq
    ON schedule_blocks (tenant_id, staff_id, kind, external_ref)
    WHERE external_ref IS NOT NULL;

CREATE TABLE customers (
    tenant_id    uuid NOT NULL,
    id           uuid NOT NULL DEFAULT gen_random_uuid(),
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 200),
    email        text CHECK (email IS NULL OR length(email) <= 254),
    phone        text CHECK (phone IS NULL OR phone ~ '^\+[1-9][0-9]{7,14}$'),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CHECK (email IS NOT NULL OR phone IS NOT NULL)
);

CREATE TABLE conversations (
    tenant_id       uuid NOT NULL,
    id              uuid NOT NULL DEFAULT gen_random_uuid(),
    customer_id     uuid,
    channel         text NOT NULL CHECK (channel IN ('web', 'telegram', 'demo')),
    channel_ref     text,
    status          text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'waiting_confirmation', 'escalated', 'closed')),
    -- Server-owned count of consecutive structured clarification outcomes.
    -- The model never supplies an attempt number; reaching three is an
    -- unconditional hand-off enforced while the reply is persisted.
    consecutive_clarification_failures smallint NOT NULL DEFAULT 0
        CHECK (consecutive_clarification_failures BETWEEN 0 AND 3),
    -- Reservations allow concurrent turns to atomically claim capacity.  The
    -- cross-column check is the database-enforced per-conversation hard cap.
    token_budget    integer NOT NULL DEFAULT 50000 CHECK (token_budget BETWEEN 1 AND 100000),
    tokens_used     integer NOT NULL DEFAULT 0 CHECK (tokens_used >= 0),
    tokens_reserved integer NOT NULL DEFAULT 0 CHECK (tokens_reserved >= 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
    CHECK (tokens_used + tokens_reserved <= token_budget),
    CHECK (consecutive_clarification_failures < 3 OR status = 'escalated'),
    UNIQUE (tenant_id, id, customer_id)
);

COMMENT ON COLUMN conversations.channel_ref IS
    'For the Stage 1 demo channel, stores only the hex SHA-256 digest of the one-time conversation capability token; never the raw token.';

CREATE UNIQUE INDEX conversations_channel_ref_uq
    ON conversations (tenant_id, channel, channel_ref)
    WHERE channel_ref IS NOT NULL;

CREATE TABLE messages (
    tenant_id       uuid NOT NULL,
    id              uuid NOT NULL DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL,
    role            text NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
    content         text NOT NULL CHECK (octet_length(content) <= 65536),
    token_count     integer NOT NULL DEFAULT 0 CHECK (token_count >= 0),
    external_ref    text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, id, conversation_id)
);

CREATE INDEX messages_conversation_created_idx
    ON messages (tenant_id, conversation_id, created_at, id);
CREATE UNIQUE INDEX messages_external_ref_uq
    ON messages (tenant_id, conversation_id, external_ref)
    WHERE external_ref IS NOT NULL;

CREATE TABLE bookings (
    tenant_id              uuid NOT NULL,
    id                     uuid NOT NULL DEFAULT gen_random_uuid(),
    customer_id            uuid NOT NULL,
    conversation_id        uuid,
    service_id             uuid NOT NULL,
    staff_id               uuid NOT NULL,
    status                 text NOT NULL DEFAULT 'confirmed'
                                  CHECK (status IN ('confirmed', 'checked_in', 'completed', 'no_show', 'cancelled')),
    starts_at              timestamptz NOT NULL,
    ends_at                timestamptz NOT NULL,
    buffer_before_minutes  integer NOT NULL CHECK (buffer_before_minutes BETWEEN 0 AND 240),
    buffer_after_minutes   integer NOT NULL CHECK (buffer_after_minutes BETWEEN 0 AND 240),
    -- Maintained by a trigger below. PostgreSQL classifies timestamptz +/-
    -- interval as STABLE (timezone-aware), so that expression is not legal in
    -- a generated column even though minute-only buffers are deterministic.
    occupied_range         tstzrange NOT NULL,
    schedule_version       integer NOT NULL DEFAULT 1 CHECK (schedule_version > 0),
    notes                  text NOT NULL DEFAULT '' CHECK (length(notes) <= 500),
    cancellation_reason    text CHECK (cancellation_reason IS NULL OR length(cancellation_reason) <= 500),
    cancelled_at           timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
    FOREIGN KEY (tenant_id, conversation_id, customer_id)
        REFERENCES conversations (tenant_id, id, customer_id),
    FOREIGN KEY (tenant_id, service_id) REFERENCES services (tenant_id, id),
    FOREIGN KEY (tenant_id, staff_id) REFERENCES staff (tenant_id, id),
    CHECK (starts_at < ends_at),
    CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL))
);

CREATE FUNCTION kontor_set_booking_occupied_range()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.occupied_range := tstzrange(
        NEW.starts_at - make_interval(mins => NEW.buffer_before_minutes),
        NEW.ends_at + make_interval(mins => NEW.buffer_after_minutes),
        '[)'
    );
    RETURN NEW;
END;
$$;

CREATE TRIGGER bookings_set_occupied_range
BEFORE INSERT OR UPDATE OF starts_at, ends_at, buffer_before_minutes, buffer_after_minutes
ON bookings
FOR EACH ROW EXECUTE FUNCTION kontor_set_booking_occupied_range();

ALTER TABLE bookings ADD CONSTRAINT bookings_no_staff_overlap
    EXCLUDE USING gist (
        tenant_id WITH =,
        staff_id WITH =,
        occupied_range WITH &&
    ) WHERE (status IN ('confirmed', 'checked_in', 'completed', 'no_show'));

CREATE INDEX bookings_customer_idx
    ON bookings (tenant_id, customer_id, starts_at DESC);
CREATE INDEX bookings_staff_time_idx
    ON bookings (tenant_id, staff_id, starts_at);

CREATE TABLE booking_events (
    tenant_id   uuid NOT NULL,
    id          uuid NOT NULL DEFAULT gen_random_uuid(),
    booking_id  uuid NOT NULL,
    event_type  text NOT NULL CHECK (event_type IN ('created', 'rescheduled', 'cancelled', 'status_changed')),
    actor_type  text NOT NULL CHECK (actor_type IN ('customer', 'agent', 'admin', 'system')),
    actor_ref   text,
    from_state  jsonb,
    to_state    jsonb NOT NULL CHECK (octet_length(to_state::text) <= 65536),
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, booking_id) REFERENCES bookings (tenant_id, id) ON DELETE CASCADE
);

CREATE INDEX booking_events_booking_idx
    ON booking_events (tenant_id, booking_id, created_at, id);

-- A row is materialised for each staff/local-date pair before a booking write.
-- Writers lock it with SELECT ... FOR UPDATE and recheck availability.
CREATE TABLE schedule_locks (
    tenant_id   uuid NOT NULL,
    staff_id    uuid NOT NULL,
    local_date  date NOT NULL,
    touched_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, staff_id, local_date),
    FOREIGN KEY (tenant_id, staff_id) REFERENCES staff (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE idempotency_records (
    tenant_id      uuid NOT NULL,
    scope          text NOT NULL CHECK (length(scope) BETWEEN 1 AND 100),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 128),
    request_hash   text NOT NULL CHECK (length(request_hash) BETWEEN 32 AND 128),
    status         text NOT NULL CHECK (status IN ('in_progress', 'completed')),
    resource_id    uuid,
    response_json  jsonb,
    created_at     timestamptz NOT NULL DEFAULT now(),
    completed_at   timestamptz,
    PRIMARY KEY (tenant_id, scope, idempotency_key),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CHECK ((status = 'completed') = (completed_at IS NOT NULL))
);

CREATE TABLE agent_runs (
    tenant_id          uuid NOT NULL,
    id                 uuid NOT NULL DEFAULT gen_random_uuid(),
    conversation_id    uuid NOT NULL,
    trigger_message_id uuid,
    status             text NOT NULL CHECK (status IN ('running', 'completed', 'failed', 'escalated', 'budget_exhausted')),
    provider           text NOT NULL,
    model              text NOT NULL,
    prompt_tokens      integer NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens  integer NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    duration_ms        integer CHECK (duration_ms IS NULL OR duration_ms >= 0),
    error_code         text,
    error_message      text CHECK (error_message IS NULL OR length(error_message) <= 2000),
    started_at         timestamptz NOT NULL DEFAULT now(),
    finished_at        timestamptz,
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, conversation_id) REFERENCES conversations (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, trigger_message_id, conversation_id)
        REFERENCES messages (tenant_id, id, conversation_id),
    UNIQUE (tenant_id, id, conversation_id)
);

CREATE INDEX agent_runs_conversation_idx
    ON agent_runs (tenant_id, conversation_id, started_at DESC);

CREATE TABLE agent_iterations (
    tenant_id        uuid NOT NULL,
    id               uuid NOT NULL DEFAULT gen_random_uuid(),
    agent_run_id     uuid NOT NULL,
    iteration_no     integer NOT NULL CHECK (iteration_no BETWEEN 1 AND 64),
    status           text NOT NULL CHECK (status IN ('succeeded', 'failed')),
    finish_reason    text,
    prompt_tokens    integer NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens integer NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    duration_ms      integer NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    model_response   jsonb CHECK (model_response IS NULL OR octet_length(model_response::text) <= 262144),
    error_message    text CHECK (error_message IS NULL OR length(error_message) <= 2000),
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, agent_run_id) REFERENCES agent_runs (tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, agent_run_id, iteration_no),
    UNIQUE (tenant_id, id, agent_run_id)
);

-- One row per model-emitted tool call.  Retry attempts are deliberately child
-- rows in tool_execution_attempts so the Stage 4 trace can render an expandable
-- parent call with attempt 1..N beneath it.
CREATE TABLE tool_executions (
    tenant_id        uuid NOT NULL,
    id               uuid NOT NULL DEFAULT gen_random_uuid(),
    agent_run_id     uuid NOT NULL,
    agent_iteration_id uuid NOT NULL,
    tool_call_id     text NOT NULL CHECK (length(tool_call_id) BETWEEN 1 AND 200),
    call_index       integer NOT NULL CHECK (call_index BETWEEN 1 AND 64),
    call_count       integer NOT NULL CHECK (call_count BETWEEN 1 AND 64),
    tool_name        text NOT NULL CHECK (length(tool_name) BETWEEN 1 AND 100),
    contract_version text NOT NULL CHECK (length(contract_version) BETWEEN 1 AND 32),
    arguments_json   jsonb NOT NULL CHECK (octet_length(arguments_json::text) <= 65536),
    result_json      jsonb CHECK (result_json IS NULL OR octet_length(result_json::text) <= 262144),
    status           text NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'refused', 'confirmation_required')),
    duration_ms      integer CHECK (duration_ms IS NULL OR duration_ms >= 0),
    started_at       timestamptz,
    finished_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, agent_run_id) REFERENCES agent_runs (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, agent_iteration_id, agent_run_id)
        REFERENCES agent_iterations (tenant_id, id, agent_run_id) ON DELETE CASCADE,
    UNIQUE (tenant_id, agent_run_id, tool_call_id),
    UNIQUE (tenant_id, agent_iteration_id, call_index),
    CHECK (call_index <= call_count)
);

CREATE TABLE tool_execution_attempts (
    tenant_id          uuid NOT NULL,
    id                 uuid NOT NULL DEFAULT gen_random_uuid(),
    tool_execution_id  uuid NOT NULL,
    attempt_no         integer NOT NULL CHECK (attempt_no BETWEEN 1 AND 16),
    status             text NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    result_json        jsonb CHECK (result_json IS NULL OR octet_length(result_json::text) <= 262144),
    error_code         text,
    error_message      text CHECK (error_message IS NULL OR length(error_message) <= 2000),
    retryable          boolean NOT NULL DEFAULT false,
    provider_status    integer,
    duration_ms        integer CHECK (duration_ms IS NULL OR duration_ms >= 0),
    started_at         timestamptz NOT NULL DEFAULT now(),
    finished_at        timestamptz,
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, tool_execution_id) REFERENCES tool_executions (tenant_id, id) ON DELETE CASCADE,
    UNIQUE (tenant_id, tool_execution_id, attempt_no)
);

CREATE INDEX tool_executions_run_idx
    ON tool_executions (tenant_id, agent_run_id, created_at, id);
CREATE INDEX tool_execution_attempts_parent_idx
    ON tool_execution_attempts (tenant_id, tool_execution_id, attempt_no);

CREATE TABLE action_proposals (
    tenant_id             uuid NOT NULL,
    id                    uuid NOT NULL DEFAULT gen_random_uuid(),
    conversation_id       uuid NOT NULL,
    customer_id           uuid NOT NULL,
    proposed_message_id   uuid NOT NULL,
    tool_name             text NOT NULL CHECK (tool_name IN ('create_booking', 'reschedule_booking', 'cancel_booking')),
    arguments_json        jsonb NOT NULL CHECK (octet_length(arguments_json::text) <= 65536),
    arguments_hash        text NOT NULL CHECK (length(arguments_hash) BETWEEN 32 AND 128),
    summary               text NOT NULL CHECK (length(summary) BETWEEN 1 AND 2000),
    resource_id           uuid,
    resource_version      integer,
    status                text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'confirmed', 'consumed', 'expired', 'rejected')),
    expires_at            timestamptz NOT NULL,
    confirmed_message_id  uuid,
    confirmed_at          timestamptz,
    consumed_at           timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
    FOREIGN KEY (tenant_id, conversation_id, customer_id)
        REFERENCES conversations (tenant_id, id, customer_id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, proposed_message_id, conversation_id)
        REFERENCES messages (tenant_id, id, conversation_id),
    FOREIGN KEY (tenant_id, confirmed_message_id, conversation_id)
        REFERENCES messages (tenant_id, id, conversation_id),
    CHECK (resource_version IS NULL OR resource_version > 0),
    CHECK (expires_at > created_at)
);

CREATE INDEX action_proposals_pending_idx
    ON action_proposals (tenant_id, conversation_id, customer_id, expires_at)
    WHERE status = 'pending';
CREATE UNIQUE INDEX action_proposals_one_active_uq
    ON action_proposals (tenant_id, conversation_id, customer_id)
    WHERE status IN ('pending', 'confirmed');

CREATE TABLE escalations (
    tenant_id       uuid NOT NULL,
    id              uuid NOT NULL DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL,
    customer_id     uuid,
    agent_run_id    uuid,
    source_tool_call_id text CHECK (source_tool_call_id IS NULL OR length(source_tool_call_id) BETWEEN 1 AND 200),
    reason_code     text NOT NULL CHECK (length(reason_code) BETWEEN 1 AND 100),
    summary         text NOT NULL CHECK (length(summary) BETWEEN 1 AND 2000),
    status          text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'claimed', 'resolved')),
    claimed_by      text,
    claimed_at      timestamptz,
    resolved_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
    FOREIGN KEY (tenant_id, conversation_id, customer_id)
        REFERENCES conversations (tenant_id, id, customer_id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, agent_run_id, conversation_id)
        REFERENCES agent_runs (tenant_id, id, conversation_id)
);

CREATE INDEX escalations_open_idx
    ON escalations (tenant_id, created_at)
    WHERE status IN ('open', 'claimed');
CREATE UNIQUE INDEX escalations_tool_call_uq
    ON escalations (tenant_id, agent_run_id, source_tool_call_id)
    WHERE source_tool_call_id IS NOT NULL;

-- Save-first agent failures are retained for operator replay/inspection rather
-- than disappearing after the customer-facing fallback is written. Stage 2's
-- bounded channel queue can use the same dead-letter shape.
CREATE TABLE dead_letter_events (
    tenant_id         uuid NOT NULL,
    id                uuid NOT NULL DEFAULT gen_random_uuid(),
    conversation_id   uuid NOT NULL,
    customer_id       uuid NOT NULL,
    agent_run_id      uuid NOT NULL,
    trigger_message_id uuid NOT NULL,
    event_type        text NOT NULL CHECK (event_type IN ('agent_turn_failed')),
    reason_code       text NOT NULL CHECK (length(reason_code) BETWEEN 1 AND 100),
    payload_json      jsonb NOT NULL CHECK (octet_length(payload_json::text) <= 65536),
    status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'resolved')),
    replay_attempts   integer NOT NULL DEFAULT 0 CHECK (replay_attempts BETWEEN 0 AND 100),
    last_error        text CHECK (last_error IS NULL OR length(last_error) <= 2000),
    created_at        timestamptz NOT NULL DEFAULT now(),
    resolved_at       timestamptz,
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id, conversation_id, customer_id)
        REFERENCES conversations (tenant_id, id, customer_id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, agent_run_id, conversation_id)
        REFERENCES agent_runs (tenant_id, id, conversation_id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, trigger_message_id, conversation_id)
        REFERENCES messages (tenant_id, id, conversation_id),
    CHECK ((status = 'resolved') = (resolved_at IS NOT NULL))
);

CREATE INDEX dead_letter_events_pending_idx
    ON dead_letter_events (tenant_id, created_at, id)
    WHERE status = 'pending';
