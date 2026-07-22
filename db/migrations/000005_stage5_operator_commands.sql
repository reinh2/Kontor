-- Kontor Stage 5: operator calendar commands.
--
-- Operator create/reschedule/cancel need a way to stop or move a pending
-- reminder in the same transaction that mutates the booking. Neither existing
-- terminal state fits: 'completed' asserts a reminder was actually delivered
-- and 'dead' asserts the retry budget was exhausted. A dedicated 'cancelled'
-- state records that the job was intentionally retired because its booking was
-- cancelled or rescheduled, keeping the operator audit and the queue honest.

ALTER TABLE jobs ADD COLUMN cancelled_at timestamptz;

-- The status value CHECK is an inline single-column constraint, so PostgreSQL
-- named it jobs_status_check. Replace it to admit the new terminal state; the
-- separate completed/dead cross-column CHECKs already hold for 'cancelled'
-- (both completed_at and dead_at remain NULL) and are left untouched.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_status_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_status_check
    CHECK (status IN ('pending', 'claimed', 'completed', 'dead', 'cancelled'));

ALTER TABLE jobs ADD CONSTRAINT jobs_cancelled_at_check
    CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL));

COMMENT ON COLUMN jobs.cancelled_at IS
    'Set when an operator booking cancel/reschedule intentionally retires a still-pending job (e.g. a reminder that must no longer fire).';
