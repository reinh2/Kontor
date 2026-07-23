# Current Project Status

- **Last updated:** 2026-07-23
- **Branch / worktree:** `main`
- **Current goal:** Stage 7 — production hardening and launch

## Stable working state

- Stages 1–6 are complete: conversation-to-booking core, channels (widget, SSE, Telegram), design implementation, reminders/CRM, operator console with live data and calendar commands, and multi-tenancy + operator identity.
- All local checks are green: `gofmt -l` reports no unformatted files, `go vet ./...` passes, `go test ./...` passes, and `go test -race -count=1 ./...` passes.
- Both binaries (`cmd/api`, `cmd/worker`) build and run successfully.

## Recently completed

- **Direct OpenAI provider.** Added `LLM_PROVIDER=openai` with `OPENAI_API_KEY`, `OPENAI_MODEL`, and optional `OPENAI_BASE_URL`. It uses the existing bounded Chat Completions tool-calling path without OpenRouter attribution headers; fake and OpenRouter modes remain supported. Unit coverage verifies direct authentication and headers, and Compose now passes the settings to both API and worker.
- **Code-review audit fixes (2026-07-23).** Resolved the defects recorded in [`ROADMAP.md`](../ROADMAP.md) "Code review — full audit":
  - **D-1 (correctness):** the customer `reschedule_booking` / `cancel_booking` repository paths now run through the same serializable-retry loop and `mapDatabaseError` wrapper as `CreateBooking` and the Admin paths, so an overlapping-slot reschedule surfaces as `ErrSlotUnavailable` (not a misleadingly retryable dependency error) and transient serialization conflicts are retried.
  - **D-2 (reliability):** the job queue now recovers stranded work. `jobqueue.Queue.RequeueStaleClaims` returns jobs stuck in `claimed` by a crashed/shut-down worker to `pending` (dead-lettering exhausted ones), the worker runs it on a timer, and terminal `Complete`/`Fail` writes use a shutdown-safe context so a finished job is never stranded.
  - **D-3 (docs):** this status doc and the architecture map were refreshed.
  - **D-4 (info-hygiene):** the demo HTTP handler no longer echoes raw `err.Error()` in responses. It gained an `internalError` helper (log + generic 500), the `getRun` bearer check now precedes the trace lookup (closing an error/existence leak to unauthenticated callers), and message-size rejections map to a 400 via the new `app.ErrInvalidMessage` sentinel.
- Stage 6 multi-tenancy and identity: operator authentication (PBKDF2-SHA256), opaque database-backed sessions, owner/staff RBAC, host-based tenant resolution, tenant-scoped widget CORS, encrypted (AES-GCM, per-tenant AAD) channel secrets, and tenant onboarding routes.
- Stage 7 first slice: opt-in Prometheus `/metrics` endpoint (`internal/platform/metrics`), secrets hardening (fail-closed on demo defaults outside demo mode), and CI vulnerability/SAST scanning.

## In progress

- **Stage 7: production hardening and launch** — see [`ROADMAP.md`](../ROADMAP.md) Stage 7 and the "Release readiness" section for the remaining scope (shared-store rate limiting for horizontal scale, external calendar sync, real LLM/CRM/notification providers, tracing/alerting, backups/retention).

## Next actions

1. Add regression coverage for the audit fixes to the CI integration run (the new `internal/scheduling` reschedule-conflict and stale-claim tests require `TEST_DATABASE_URL`).
2. Continue Stage 7: shared-store rate limiter, observability (tracing/alerting), and a documented rollback/restore procedure.
3. Lock `HTTP_ALLOWED_ORIGIN` / per-tenant origins for any non-demo deployment.

## Blockers and open questions

- Stage 7 scope decision: minimum production observability (metrics only vs. metrics + tracing + alerting) for the first launch.
- External calendar sync strategy (Google Calendar / Microsoft Graph) is still open.

## Verification status

| Check | Result | Command / evidence | Last run |
|---|---|---|---|
| Format | pass (no unformatted files) | `gofmt -l` | 2026-07-23 |
| Static analysis | pass | `go vet ./...` | 2026-07-23 |
| Tests | pass | `go test ./...` | 2026-07-23 |
| Tests (race) | pass | `go test -race -count=1 ./...` | 2026-07-23 |
| Build (API) | pass | `go build ./cmd/api` | 2026-07-23 |
| Build (Worker) | pass | `go build ./cmd/worker` | 2026-07-23 |
| Integration | not run locally | needs `TEST_DATABASE_URL`; runs in CI | — |
| E2E | not run locally | CI Compose smoke (runs on push) | — |

## Changed areas requiring attention

- `internal/scheduling/repository.go` — customer reschedule/cancel now use the retry + `mapDatabaseError` wrapper (D-1).
- `internal/jobqueue/postgres.go` and `cmd/worker/main.go` — stale-claim reaper and shutdown-safe terminal writes (D-2).
- New integration tests under `internal/scheduling/` (`reschedule_conflict_integration_test.go`, `stale_claim_integration_test.go`) that require `TEST_DATABASE_URL`.

## Handoff notes

- The audit fixes are code-complete and pass local `vet`/`test`/`-race`; the two new integration tests could not be executed locally (no PostgreSQL/Docker available) and are expected to run in CI.
