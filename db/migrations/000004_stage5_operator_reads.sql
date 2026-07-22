-- Kontor Stage 5 operator read models.
--
-- The console reads the existing source-of-truth tables. These indexes make
-- the tenant-wide run feed, dashboard booking buckets, and calendar overlap
-- queries predictable without adding a second projection or cache.

CREATE INDEX agent_runs_operator_feed_idx
    ON agent_runs (tenant_id, started_at DESC, id DESC);

CREATE INDEX bookings_operator_created_idx
    ON bookings (tenant_id, created_at DESC, id DESC);

CREATE INDEX bookings_operator_calendar_idx
    ON bookings (tenant_id, starts_at, id)
    WHERE status <> 'cancelled';
