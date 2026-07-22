-- Kontor Stage 4: durable job queue.
--
-- jobs is the outbox/retry table for background work spawned by booking
-- lifecycle events — reminders, CRM upserts, and CRM deal creation.  Workers
-- poll for claimable rows using the partial index on (status, next_retry_at)
-- and advance the state machine atomically.
--
-- dead_letter_jobs preserves jobs that exhausted their retry budget for
-- operator inspection and manual replay.

CREATE TABLE jobs (
    tenant_id       uuid NOT NULL,
    id              uuid NOT NULL DEFAULT gen_random_uuid(),
    booking_id      uuid NOT NULL,
    job_type        text NOT NULL CHECK (job_type IN ('send_reminder', 'crm_upsert_contact', 'crm_create_deal')),
    status          text NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending', 'claimed', 'completed', 'dead')),
    payload_json    jsonb NOT NULL CHECK (octet_length(payload_json::text) <= 65536),
    attempts        integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts    integer NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 20),
    next_retry_at   timestamptz NOT NULL DEFAULT now(),
    claimed_at      timestamptz,
    completed_at    timestamptz,
    dead_at         timestamptz,
    last_error      text CHECK (last_error IS NULL OR length(last_error) <= 2000),
    idempotency_key text CHECK (idempotency_key IS NULL OR length(idempotency_key) BETWEEN 16 AND 128),
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    FOREIGN KEY (tenant_id, booking_id) REFERENCES bookings (tenant_id, id),
    CHECK ((status = 'completed') = (completed_at IS NOT NULL)),
    CHECK ((status = 'dead') = (dead_at IS NOT NULL))
);

COMMENT ON TABLE jobs IS
    'Durable job queue for booking-driven background work (reminders, CRM sync). Workers poll the claimable partial index and advance the state machine within a single UPDATE.';

CREATE UNIQUE INDEX jobs_idempotency_uq
    ON jobs (tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX jobs_claimable_idx
    ON jobs (tenant_id, status, next_retry_at)
    WHERE status = 'pending';

CREATE INDEX jobs_booking_idx
    ON jobs (tenant_id, booking_id);

CREATE TABLE dead_letter_jobs (
    tenant_id       uuid NOT NULL,
    id              uuid NOT NULL DEFAULT gen_random_uuid(),
    original_job_id uuid NOT NULL,
    booking_id      uuid NOT NULL,
    job_type        text NOT NULL CHECK (job_type IN ('send_reminder', 'crm_upsert_contact', 'crm_create_deal')),
    status          text NOT NULL DEFAULT 'dead'
                          CHECK (status IN ('pending', 'claimed', 'completed', 'dead')),
    payload_json    jsonb NOT NULL CHECK (octet_length(payload_json::text) <= 65536),
    attempts        integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts    integer NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 20),
    completed_at    timestamptz,
    dead_at         timestamptz,
    last_error      text CHECK (last_error IS NULL OR length(last_error) <= 2000),
    idempotency_key text CHECK (idempotency_key IS NULL OR length(idempotency_key) BETWEEN 16 AND 128),
    failed_at       timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, id),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id)
);

COMMENT ON TABLE dead_letter_jobs IS
    'Permanently failed jobs preserved for operator inspection and manual replay; moved here when attempts exhaust max_attempts.';
