# Architecture Decision Log

## Decision index

| ID | Date | Status | Decision | Scope |
|---|---|---|---|---|
| ADR-001 | — | accepted | Two-phase confirmation for all booking mutations | `internal/tools/`, `internal/confirmations/` |
| ADR-002 | — | accepted | LLM as untrusted planner; server-side authority | `internal/agent/`, `internal/tools/` |
| ADR-003 | — | accepted | PostgreSQL as sole data store (no cache, no external queue) | `compose.yaml`, `internal/platform/database/` |
| ADR-004 | — | accepted | Forward-only migrations, applied at startup | `db/migrations/`, `internal/platform/database/` |
| ADR-005 | — | accepted | Capability token with SHA-256 digest storage | `internal/channels/demohttp/`, `internal/conversations/` |
| ADR-006 | — | accepted | Bounded agent loop with escalation on exhaustion | `internal/agent/` |
| ADR-007 | — | accepted | Transactional outbox for post-booking side effects | `internal/jobqueue/`, `internal/scheduling/` |
| ADR-008 | — | accepted | Single Go binary per role; no frontend build step | `Dockerfile`, `web/` |
| ADR-009 | — | accepted | Durable SSE via transactional event rows | `internal/channels/demohttp/` |
| ADR-010 | — | accepted | Operator session tokens with digest-only storage | `internal/identity/` |

---

## ADR-001: Two-phase confirmation for all booking mutations

- **Status:** accepted
- **Supersedes:** none

### Context

An LLM can hallucinate parameters or misinterpret customer intent. Allowing a model to directly create, reschedule, or cancel a booking risks unauthorized schedule changes.

### Decision

Every booking mutation follows a propose → confirm cycle. The tool gateway stores a frozen proposal with exact facts. Only an explicit subsequent customer message matching the proposal authorizes the mutation. The confirmation is bound to the proposal's immutable arguments.

### Consequences

- **Positive:** No silent calendar mutation; customer always sees what will happen before it happens.
- **Negative:** Two round-trips minimum for any booking change.
- **Evidence:** `internal/tools/gateway.go`, `internal/confirmations/`, CI smoke test validates the two-step flow.

---

## ADR-002: LLM as untrusted planner

- **Status:** accepted
- **Supersedes:** none

### Context

Relying on prompt instructions alone for safety is fragile. Model behavior can change with versions, temperature, or adversarial input.

### Decision

The LLM proposes actions; server-side code validates, authorizes, and executes them. Model-supplied identity data never controls a booking. Authorization is derived from the conversation's capability token, not from model output.

### Consequences

- **Positive:** Safety does not depend on prompt engineering quality.
- **Negative:** More application code to validate and enforce invariants.
- **Evidence:** `internal/tools/` gateway validates JSON Schema on every call; `internal/scheduling/` enforces DB-level consistency.

---

## ADR-003: PostgreSQL as sole data store

- **Status:** accepted
- **Supersedes:** none

### Context

The system needs transactional consistency for bookings, durable events for SSE replay, and a job queue. Adding Redis or a message broker increases operational complexity for a single-instance demo.

### Decision

Use PostgreSQL for all state: domain data, SSE events, job queue (via `FOR UPDATE SKIP LOCKED`), traces, and budgets. No external cache or queue.

### Consequences

- **Positive:** Single dependency; strong consistency without distributed coordination.
- **Negative:** Horizontal scaling requires rethinking the job queue and rate limiter (currently in-memory).
- **Evidence:** `compose.yaml`, all repositories use pgx pool directly.

---

## ADR-004: Forward-only migrations at startup

- **Status:** accepted
- **Supersedes:** none

### Context

The project grows schema one stage at a time. Migrations must be safe to apply automatically for the single-instance demo deployment.

### Decision

Migrations are embedded SQL files applied in order at process startup. No down-migrations exist. Checksums prevent re-running modified migrations.

### Consequences

- **Positive:** Zero-step deployment; schema is always current when the binary starts.
- **Negative:** No automated rollback path. Production will need a separate strategy.
- **Evidence:** `db/migrations/embed.go`, `internal/platform/database/` applies them via `database.ApplyMigrations`.

---

## ADR-005: Opaque capability token with digest storage

- **Status:** accepted
- **Supersedes:** none

### Context

Customer conversations need authentication without user accounts. The token must not be recoverable from the database.

### Decision

A conversation creation returns a one-time opaque bearer token. Only its SHA-256 digest is stored. Subsequent requests authenticate by presenting the raw token; the server hashes it and matches.

### Consequences

- **Positive:** Database breach does not expose valid tokens.
- **Negative:** Lost token means lost conversation access (no recovery mechanism).
- **Evidence:** `internal/channels/demohttp/`, `internal/conversations/`.

---

## ADR-006: Bounded agent loop with escalation

- **Status:** accepted
- **Supersedes:** none

### Context

An unconstrained LLM loop can consume unbounded resources, loop indefinitely on ambiguous input, or produce incoherent multi-turn chains.

### Decision

The agent runner enforces: max iterations per turn, wall-clock timeout per turn, max retry attempts per tool call, and a persistent per-conversation token budget. Exhaustion of any limit produces a safe escalation rather than continued execution.

### Consequences

- **Positive:** Predictable resource usage; graceful degradation.
- **Negative:** Complex conversations may hit limits before resolution.
- **Evidence:** `internal/agent/runner.go`, `internal/agent/budget.go`, config env vars `AGENT_MAX_ITERATIONS`, `AGENT_TURN_TIMEOUT`, etc.

---

## ADR-007: Transactional outbox for side effects

- **Status:** accepted
- **Supersedes:** none

### Context

A confirmed booking must reliably trigger reminders and CRM updates. Sending notifications inline risks partial failure (booking committed, notification lost).

### Decision

Booking confirmation atomically enqueues `send_reminder` and `crm_upsert_contact` jobs in the same PostgreSQL transaction. The worker process polls and executes them with bounded retries.

### Consequences

- **Positive:** Side effects are guaranteed if and only if the booking commits.
- **Negative:** Delivery is eventually-consistent (poll interval delay).
- **Evidence:** `internal/jobqueue/`, `internal/scheduling/` booking creation, `cmd/worker/`.

---

## ADR-008: Single binary per role; embedded frontend

- **Status:** accepted
- **Supersedes:** none

### Context

The project aims for minimal operational overhead. A separate frontend build pipeline and static file server add complexity.

### Decision

The API binary embeds the widget JS and operator SPA HTML/CSS/JS via `go:embed`. No Node.js, npm, or webpack in the build chain. The operator console uses a vendored React DS bundle.

### Consequences

- **Positive:** `go build` produces a self-contained binary; no frontend CI step.
- **Negative:** Updating frontend assets requires rebuilding the Go binary. No hot-reload for frontend development.
- **Evidence:** `web/widget/embed.go`, `web/operator/embed.go`, `Dockerfile`.

---

## ADR-009: Durable SSE via transactional event rows

- **Status:** accepted
- **Supersedes:** none

### Context

The widget uses Server-Sent Events for streaming replies. Clients may disconnect and reconnect. Events must not be lost or duplicated.

### Decision

Each committed turn writes a `conversation_events` row in the same transaction as the reply. The SSE endpoint replays from `Last-Event-ID`. An event is never visible before its transaction commits.

### Consequences

- **Positive:** Reconnecting clients see exactly the events that were committed; no phantom outcomes.
- **Negative:** Slight delivery latency (transaction commit before SSE push).
- **Evidence:** `internal/channels/demohttp/` SSE handler, migration `000002`.

---

## ADR-010: Operator session tokens with digest-only storage

- **Status:** accepted
- **Supersedes:** none

### Context

Operators need authenticated sessions. Storing raw session tokens in the database would allow a database breach to hijack sessions.

### Decision

Same pattern as customer capabilities: login returns an opaque token once; only SHA-256 digest is persisted. Validation hashes the presented token and matches.

### Consequences

- **Positive:** Leaked database cannot replay operator sessions.
- **Negative:** Token rotation requires new login.
- **Evidence:** `internal/identity/store.go`, migration `000006`.
