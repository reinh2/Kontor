package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/reinhlord/kontor/db/migrations"
	"github.com/reinhlord/kontor/internal/crm"
	"github.com/reinhlord/kontor/internal/demo"
	"github.com/reinhlord/kontor/internal/jobqueue"
	"github.com/reinhlord/kontor/internal/notifications"
	"github.com/reinhlord/kontor/internal/platform/config"
	"github.com/reinhlord/kontor/internal/platform/database"
	"github.com/reinhlord/kontor/internal/platform/logging"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultBatchSize    = 10
	defaultConcurrency  = 4
	defaultJobTimeout   = 30 * time.Second
	// defaultStaleClaimLease must exceed defaultJobTimeout so a job still being
	// worked is never reclaimed from under a live worker; defaultReapInterval is
	// how often the loop scans for claims stranded by a crashed worker.
	defaultStaleClaimLease = 5 * time.Minute
	defaultReapInterval    = 1 * time.Minute
	// terminalWriteTimeout bounds the shutdown-safe write that records a job's
	// terminal state even when the worker context is already cancelled.
	terminalWriteTimeout = 5 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.Environment)
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.ApplyMigrations(ctx, pool, migrations.Files, "."); err != nil {
		return err
	}
	if cfg.DemoMode {
		if err := demo.EnsureFixedTenant(ctx, pool, demo.Tenant{
			ID: cfg.Tenant.ID, Slug: cfg.Tenant.Slug, Name: cfg.Tenant.Name,
			Timezone: cfg.Tenant.Timezone, Currency: cfg.Tenant.Currency,
		}); err != nil {
			return err
		}
		if err := demo.SeedCatalog(ctx, pool, cfg.Tenant.ID, cfg.Tenant.Currency); err != nil {
			return err
		}
	}

	queue := jobqueue.New(pool, logger)
	notifier := notifications.NewLogNotifier(logger)
	crmDriver := crm.NewLogCRM(logger)

	worker := &Worker{
		queue:           queue,
		notifier:        notifier,
		crm:             crmDriver,
		logger:          logger,
		pollInterval:    defaultPollInterval,
		batchSize:       defaultBatchSize,
		concurrency:     defaultConcurrency,
		jobTimeout:      defaultJobTimeout,
		staleClaimLease: defaultStaleClaimLease,
		reapInterval:    defaultReapInterval,
	}

	logger.Info("worker ready", "stage", 4, "concurrency", worker.concurrency, "poll_interval", worker.pollInterval)
	return worker.Run(ctx)
}

// Worker is the job processing loop.
type Worker struct {
	queue           *jobqueue.Queue
	notifier        notifications.Notifier
	crm             crm.CRM
	logger          *slog.Logger
	pollInterval    time.Duration
	batchSize       int
	concurrency     int
	jobTimeout      time.Duration
	staleClaimLease time.Duration
	reapInterval    time.Duration
	lastReap        time.Time
}

// Run polls for jobs until the context is cancelled, processing them with
// bounded concurrency.
func (w *Worker) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker shutting down")
			return nil
		default:
		}

		w.maybeReapStaleClaims(ctx)

		jobs, err := w.queue.ClaimBatch(ctx, w.batchSize)
		if err != nil {
			if ctx.Err() != nil {
				// A claim that failed because the worker is shutting down is a
				// clean stop, not a processing failure.
				return nil //nolint:nilerr // graceful shutdown, not an error path
			}
			w.logger.Error("claim batch failed", "error", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(w.pollInterval):
			}
			continue
		}

		if len(jobs) == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(w.pollInterval):
			}
			continue
		}

		w.processBatch(ctx, jobs)
	}
}

func (w *Worker) processBatch(ctx context.Context, jobs []jobqueue.Job) {
	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup

	for _, job := range jobs {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(j jobqueue.Job) {
			defer wg.Done()
			defer func() { <-sem }()
			w.processOne(ctx, j)
		}(job)
	}
	wg.Wait()
}

// maybeReapStaleClaims periodically returns jobs stranded in 'claimed' by a
// crashed or shut-down worker to 'pending' (or dead-letters exhausted ones). It
// is throttled by reapInterval and tolerates errors: the next tick retries.
func (w *Worker) maybeReapStaleClaims(ctx context.Context) {
	if w.reapInterval <= 0 {
		return
	}
	if !w.lastReap.IsZero() && time.Since(w.lastReap) < w.reapInterval {
		return
	}
	w.lastReap = time.Now()
	if _, _, err := w.queue.RequeueStaleClaims(ctx, w.staleClaimLease); err != nil {
		if ctx.Err() != nil {
			return
		}
		w.logger.Error("requeue stale claims failed", "error", err)
	}
}

func (w *Worker) processOne(ctx context.Context, job jobqueue.Job) {
	jobCtx, cancel := context.WithTimeout(ctx, w.jobTimeout)
	defer cancel()

	err := w.dispatch(jobCtx, job)

	// The job's work has finished; its terminal state must be recorded even if
	// the worker is shutting down. A detached, bounded context keeps a completed
	// or failed job from being stranded in 'claimed' by a cancelled ctx (the
	// stale-claim reaper is the backstop, not the primary path).
	termCtx, termCancel := context.WithTimeout(context.WithoutCancel(ctx), terminalWriteTimeout)
	defer termCancel()

	if err != nil {
		w.logger.Warn("job failed",
			"job_id", job.ID, "job_type", job.JobType,
			"attempts", job.Attempts, "error", err,
		)
		if failErr := w.queue.Fail(termCtx, job.ID, err); failErr != nil {
			w.logger.Error("fail job error", "job_id", job.ID, "error", failErr)
		}
		return
	}

	if completeErr := w.queue.Complete(termCtx, job.ID); completeErr != nil {
		w.logger.Error("complete job error", "job_id", job.ID, "error", completeErr)
	}
}

func (w *Worker) dispatch(ctx context.Context, job jobqueue.Job) error {
	switch job.JobType {
	case "send_reminder":
		return w.handleReminder(ctx, job)
	case "crm_upsert_contact":
		return w.handleCRMUpsert(ctx, job)
	case "crm_create_deal":
		return w.handleCRMDeal(ctx, job)
	default:
		return fmt.Errorf("unknown job type: %s", job.JobType)
	}
}

// reminderPayload is the JSON envelope for send_reminder jobs.
type reminderPayload struct {
	CustomerName  string `json:"customer_name"`
	CustomerEmail string `json:"customer_email"`
	CustomerPhone string `json:"customer_phone"`
	ServiceName   string `json:"service_name"`
	StaffName     string `json:"staff_name"`
	StartsAt      string `json:"starts_at"`
	Timezone      string `json:"timezone"`
}

func (w *Worker) handleReminder(ctx context.Context, job jobqueue.Job) error {
	var payload reminderPayload
	if err := json.Unmarshal(job.PayloadJSON, &payload); err != nil {
		return fmt.Errorf("decode reminder payload: %w", err)
	}
	startsAt, err := time.Parse(time.RFC3339, payload.StartsAt)
	if err != nil {
		return fmt.Errorf("parse starts_at: %w", err)
	}
	_, err = w.notifier.SendReminder(ctx, notifications.Reminder{
		TenantID:      job.TenantID,
		BookingID:     job.BookingID,
		CustomerName:  payload.CustomerName,
		CustomerEmail: payload.CustomerEmail,
		CustomerPhone: payload.CustomerPhone,
		ServiceName:   payload.ServiceName,
		StaffName:     payload.StaffName,
		StartsAt:      startsAt,
		Timezone:      payload.Timezone,
	})
	return err
}

// crmUpsertPayload is the JSON envelope for crm_upsert_contact jobs.
type crmUpsertPayload struct {
	CustomerID  string `json:"customer_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Company     string `json:"company"`
	Locale      string `json:"locale"`
}

func (w *Worker) handleCRMUpsert(ctx context.Context, job jobqueue.Job) error {
	var payload crmUpsertPayload
	if err := json.Unmarshal(job.PayloadJSON, &payload); err != nil {
		return fmt.Errorf("decode crm upsert payload: %w", err)
	}
	_, err := w.crm.UpsertContact(ctx, crm.Contact{
		TenantID:    job.TenantID,
		CustomerID:  payload.CustomerID,
		DisplayName: payload.DisplayName,
		Email:       payload.Email,
		Phone:       payload.Phone,
		Company:     payload.Company,
		Locale:      payload.Locale,
	})
	return err
}

// crmDealPayload is the JSON envelope for crm_create_deal jobs.
type crmDealPayload struct {
	ContactRef  string `json:"contact_ref"`
	ServiceName string `json:"service_name"`
	StaffName   string `json:"staff_name"`
	StartsAt    string `json:"starts_at"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
}

func (w *Worker) handleCRMDeal(ctx context.Context, job jobqueue.Job) error {
	var payload crmDealPayload
	if err := json.Unmarshal(job.PayloadJSON, &payload); err != nil {
		return fmt.Errorf("decode crm deal payload: %w", err)
	}
	startsAt, err := time.Parse(time.RFC3339, payload.StartsAt)
	if err != nil {
		return fmt.Errorf("parse starts_at in crm deal payload: %w", err)
	}
	_, err = w.crm.CreateDeal(ctx, crm.Deal{
		TenantID:    job.TenantID,
		BookingID:   job.BookingID,
		ContactRef:  payload.ContactRef,
		ServiceName: payload.ServiceName,
		StaffName:   payload.StaffName,
		StartsAt:    startsAt,
		Amount:      payload.Amount,
		Currency:    payload.Currency,
	})
	return err
}
