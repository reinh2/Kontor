package jobqueue

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Job represents a claimed job ready for execution.
type Job struct {
	ID          string
	TenantID    string
	BookingID   string
	JobType     string
	PayloadJSON []byte
	Attempts    int
	MaxAttempts int
}

// Handler processes a single job. Return nil for success, error for retry.
type Handler func(ctx context.Context, job Job) error

// EnqueueParams holds the parameters for inserting a new job.
type EnqueueParams struct {
	TenantID       string
	BookingID      string
	JobType        string
	PayloadJSON    []byte
	MaxAttempts    int
	IdempotencyKey string
}

// Queue manages job claiming, completion, retry, and dead-lettering.
type Queue struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// New creates a new Queue backed by the given connection pool.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Queue {
	return &Queue{pool: pool, logger: logger}
}

// ClaimBatch claims up to limit pending jobs that are ready for processing.
// It uses FOR UPDATE SKIP LOCKED to allow safe concurrent polling.
func (q *Queue) ClaimBatch(ctx context.Context, limit int) ([]Job, error) {
	query := `
		UPDATE jobs SET status = 'claimed', claimed_at = now(), attempts = attempts + 1
		WHERE (tenant_id, id) IN (
			SELECT tenant_id, id FROM jobs
			WHERE status = 'pending' AND next_retry_at <= now()
			ORDER BY next_retry_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id::text, tenant_id::text, booking_id::text, job_type, payload_json, attempts, max_attempts
	`

	rows, err := q.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("jobqueue: claim batch query: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.TenantID, &j.BookingID, &j.JobType, &j.PayloadJSON, &j.Attempts, &j.MaxAttempts); err != nil {
			return nil, fmt.Errorf("jobqueue: claim batch scan: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("jobqueue: claim batch rows: %w", err)
	}

	if len(jobs) > 0 {
		q.logger.LogAttrs(ctx, slog.LevelInfo, "claimed jobs",
			slog.Int("count", len(jobs)),
		)
	}

	return jobs, nil
}

// Complete marks a claimed job as successfully completed.
func (q *Queue) Complete(ctx context.Context, jobID string) error {
	query := `UPDATE jobs SET status = 'completed', completed_at = now() WHERE id = $1 AND status = 'claimed'`

	tag, err := q.pool.Exec(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("jobqueue: complete job %s: %w", jobID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("jobqueue: complete job %s: no claimed job found", jobID)
	}

	q.logger.LogAttrs(ctx, slog.LevelInfo, "job completed",
		slog.String("job_id", jobID),
	)
	return nil
}

// Fail handles a job execution failure. If the job has remaining attempts it is
// returned to pending with exponential backoff. Otherwise it is dead-lettered.
func (q *Queue) Fail(ctx context.Context, jobID string, jobErr error) error {
	// Fetch current state of the job.
	var attempts, maxAttempts int
	var tenantID string
	err := q.pool.QueryRow(ctx,
		`SELECT tenant_id::text, attempts, max_attempts FROM jobs WHERE id = $1 AND status = 'claimed'`,
		jobID,
	).Scan(&tenantID, &attempts, &maxAttempts)
	if err != nil {
		return fmt.Errorf("jobqueue: fail lookup job %s: %w", jobID, err)
	}

	if attempts < maxAttempts {
		return q.retryJob(ctx, jobID, attempts, jobErr)
	}
	return q.deadLetter(ctx, jobID, tenantID, jobErr)
}

// retryJob returns a job to pending status with exponential backoff.
func (q *Queue) retryJob(ctx context.Context, jobID string, attempts int, jobErr error) error {
	delay := retryDelay(attempts)
	query := `
		UPDATE jobs
		SET status = 'pending',
		    next_retry_at = now() + make_interval(secs => $1),
		    last_error = $2,
		    claimed_at = NULL
		WHERE id = $3 AND status = 'claimed'
	`

	_, err := q.pool.Exec(ctx, query, delay.Seconds(), jobErr.Error(), jobID)
	if err != nil {
		return fmt.Errorf("jobqueue: retry job %s: %w", jobID, err)
	}

	q.logger.LogAttrs(ctx, slog.LevelWarn, "job scheduled for retry",
		slog.String("job_id", jobID),
		slog.Int("attempts", attempts),
		slog.Duration("delay", delay),
		slog.String("error", jobErr.Error()),
	)
	return nil
}

// deadLetter moves a job to the dead letter state after exhausting retries.
func (q *Queue) deadLetter(ctx context.Context, jobID, tenantID string, jobErr error) error {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("jobqueue: dead letter begin tx for job %s: %w", jobID, err)
	}
	defer tx.Rollback(ctx)

	// Copy to dead_letter_jobs from the current job row.
	_, err = tx.Exec(ctx, `
		INSERT INTO dead_letter_jobs
		    (tenant_id, original_job_id, booking_id, job_type, payload_json,
		     attempts, max_attempts, last_error, idempotency_key, dead_at, created_at)
		SELECT tenant_id, id, booking_id, job_type, payload_json,
		       attempts, max_attempts, $2, idempotency_key, now(), created_at
		FROM jobs WHERE id = $1 AND status = 'claimed'
	`, jobID, jobErr.Error())
	if err != nil {
		return fmt.Errorf("jobqueue: dead letter insert for job %s: %w", jobID, err)
	}

	// Mark the job as dead.
	_, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'dead', dead_at = now(), last_error = $1, claimed_at = NULL
		WHERE id = $2 AND status = 'claimed'
	`, jobErr.Error(), jobID)
	if err != nil {
		return fmt.Errorf("jobqueue: dead letter update for job %s: %w", jobID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("jobqueue: dead letter commit for job %s: %w", jobID, err)
	}

	q.logger.LogAttrs(ctx, slog.LevelError, "job dead-lettered",
		slog.String("job_id", jobID),
		slog.String("tenant_id", tenantID),
		slog.String("error", jobErr.Error()),
	)
	return nil
}

// Enqueue inserts a new job inside an existing transaction. It uses an
// idempotency key to avoid duplicate jobs via ON CONFLICT DO NOTHING.
func (q *Queue) Enqueue(ctx context.Context, tx pgx.Tx, params EnqueueParams) error {
	query := `
		INSERT INTO jobs (tenant_id, booking_id, job_type, payload_json, max_attempts, idempotency_key, status, next_retry_at, attempts, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending', now(), 0, now())
		ON CONFLICT (tenant_id, idempotency_key)
		WHERE idempotency_key IS NOT NULL DO NOTHING
	`

	_, err := tx.Exec(ctx, query,
		params.TenantID,
		params.BookingID,
		params.JobType,
		params.PayloadJSON,
		params.MaxAttempts,
		params.IdempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("jobqueue: enqueue job type %s for tenant %s: %w", params.JobType, params.TenantID, err)
	}

	q.logger.LogAttrs(ctx, slog.LevelInfo, "job enqueued",
		slog.String("tenant_id", params.TenantID),
		slog.String("booking_id", params.BookingID),
		slog.String("job_type", params.JobType),
		slog.String("idempotency_key", params.IdempotencyKey),
	)
	return nil
}

// ReplayDeadLetter moves a dead-lettered job back to pending with reset attempts.
func (q *Queue) ReplayDeadLetter(ctx context.Context, jobID string) error {
	query := `
		UPDATE jobs
		SET status = 'pending',
		    attempts = 0,
		    next_retry_at = now(),
		    dead_at = NULL,
		    last_error = NULL,
		    claimed_at = NULL,
		    completed_at = NULL
		WHERE id = $1 AND status = 'dead'
	`

	tag, err := q.pool.Exec(ctx, query, jobID)
	if err != nil {
		return fmt.Errorf("jobqueue: replay dead letter job %s: %w", jobID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("jobqueue: replay dead letter job %s: no dead job found", jobID)
	}

	q.logger.LogAttrs(ctx, slog.LevelInfo, "dead-lettered job replayed",
		slog.String("job_id", jobID),
	)
	return nil
}

// retryDelay calculates exponential backoff delay for a given attempt count.
func retryDelay(attempts int) time.Duration {
	const baseDelay = 30 * time.Second
	const maxDelay = time.Hour
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempts-1)))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}
