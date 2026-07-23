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
| ADR-011 | — | accepted | Customer messages own contact capture and proposal supersession | `internal/conversations/`, `internal/app/`, `internal/tools/` |
| ADR-012 | 2026-07-23 | accepted | Conservative token estimation uses bytes÷3, not 1:1 | `internal/agent/budget.go` |
| ADR-013 | 2026-07-24 | accepted | respond_to_customer is the only channel to the customer | `internal/agent/runner.go` |
| ADR-014 | 2026-07-24 | accepted | An accounted model call is charged at its reported usage | `internal/agent/runner.go`, `internal/llm/` |
| ADR-015 | 2026-07-24 | accepted | A confirmed call executes the server's frozen action | `internal/tools/gateway.go` |

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

---

## ADR-011: Customer messages own contact capture and proposal supersession

- **Status:** accepted
- **Supersedes:** none

### Context

The model may need contact data to progress a booking, but it is not an
authority for customer identity. A visible pending proposal must also never be
confirmable after the customer changes the request or a turn fails.

### Decision

Only a literal email or E.164 phone in the authenticated customer's persisted
message can fill a missing profile contact. The tool gateway continues to
derive booking customer data from that trusted profile. Any non-consent turn,
clarification, or failed turn rejects live proposals; every durable turn event
includes an explicit `pending_confirmation_active` snapshot for embedded UI.

### Consequences

- **Positive:** the model cannot fabricate contact data, and an old card cannot
  silently authorize a superseded action.
- **Negative:** a customer changing their mind must receive a new proposal and
  confirm it again.
- **Evidence:** `internal/conversations/store.go`, `internal/app/service.go`,
  `internal/tools/confirmations.go`, `web/widget/kontor.js`.


---

## ADR-012: Conservative token estimation uses bytes÷3, not 1:1

- **Status:** accepted
- **Supersedes:** none (refines ADR-006's budget implementation)

### Context

The `ConservativeTokenEstimator` converted raw byte counts of message content
and tool parameter schemas directly into token reservations (1 byte = 1 token).
Real BPE tokenization averages 3.5–4 bytes per token for English text and JSON.
The 1:1 ratio inflated typical tool-heavy requests (~10 schemas totaling ~5 KB)
to ~5000-token reservations when actual provider usage was ~1500. With a 50 000
conversation budget, a single turn could require 33 000+ tokens, exhausting the
budget in one or two turns and triggering escalation before meaningful work.

### Decision

Replace the 1:1 byte-to-token ratio with ceiling division by a configurable
`BytesPerToken` field (default 3). The conservative ratio of 3 safely
overestimates real tokenization (~15% above average) while reducing reservations
by ~3× compared to the former 1:1.

The hard-budget safety invariant is preserved:
- Provider failures and ambiguous usage still charge the full reservation.
- Actual provider-reported usage above the reservation still triggers
  `ErrUsageExceedsReservation` (full reservation is kept).
- The persistent PostgreSQL cap (`tokens_used + tokens_reserved <= token_budget`)
  is unchanged.

### Consequences

- **Positive:** A 50 000-token conversation now supports 5–8 normal turns before
  budget pressure, matching real-model cost expectations.
- **Negative:** If a provider tokenizer is far less efficient than 3 bytes/token
  (unlikely for modern models with English/JSON), occasional
  `ErrUsageExceedsReservation` events would retain the full reservation. This is
  a safe failure mode (overcharges, never undercharges).
- **Evidence:** `internal/agent/budget.go`, `internal/agent/budget_test.go`
  (`TestConservativeTokenEstimatorRealisticToolSchemaNotInflated`).

---

## ADR-013: respond_to_customer is the only channel to the customer

- **Status:** accepted
- **Supersedes:** none (tightens ADR-002 and ADR-006)

### Context

The runner accepted an assistant message with no tool calls as a finished turn
and delivered its text to the customer. With a real provider this became a
hallucination channel: a model that skipped `list_services` answered a booking
request with `I am sorry, "colour" is not a valid service` — for a service the
tenant actually offers — followed by an invented UUID. No tool ran, so no
server-side check could contradict it.

The same gap let a turn keep calling tools after the gateway had already frozen
a confirmation proposal, spending the iteration and token budget on work that
only the customer's answer could unblock.

### Decision

Free assistant prose is never delivered. A response with no tool call is
discarded, the runner appends a server-authored protocol correction and
re-prompts within the same bounded turn, and after `maxProtocolCorrections`
consecutive violations the turn fails closed into the existing escalation path.

Once any tool returns `confirmation_required`, the remaining calls in that batch
are skipped and every later iteration is answered with the same correction: only
`respond_to_customer` may follow a live proposal.

The correction is sent with the user role, not the system role: providers that
flatten a conversation into a native format (Gemini through OpenRouter) return
an empty response when a system message trails an assistant turn. The correction
grants no authority, so a customer imitating it changes nothing.

### Consequences

- **Positive:** Every customer-facing sentence passes the validated terminal
  control call, and the disposition/clarification policy sees every reply.
- **Negative:** A model that cannot follow the protocol costs up to two extra
  iterations per turn before escalating.
- **Evidence:** `internal/agent/runner.go`, `internal/agent/runner_test.go`
  (`TestRunnerDiscardsUnstructuredTextAndCorrectsProtocol`,
  `TestRunnerFailsClosedOnRepeatedUnstructuredText`,
  `TestRunnerStopsActingAfterAConfirmationProposal`).

---

## ADR-014: An accounted model call is charged at its reported usage

- **Status:** accepted
- **Supersedes:** none (refines ADR-006 and ADR-012)

### Context

`Response.UsageIncomplete` is sticky across the provider adapter's retry loop:
one failed attempt sets it for the whole call. The runner charged the full
worst-case reservation whenever it was set — even when a later attempt succeeded
and reported its own usage. A booking conversation was billed ~82 000 tokens
against a 100 000 budget for 39 329 real tokens and escalated as
`budget_exhausted` before the customer could confirm.

### Decision

`chargeForModelCall` prices one call:

- provider error, or no reported usage at all → the full reservation (unchanged);
- reported usage with no incomplete attempts → the reported usage (unchanged);
- reported usage with N unaccounted attempts → reported usage plus N × the
  reported input tokens, capped at the reservation.

Each unaccounted attempt sent the same prompt and returned no completion, so the
input tokens bound its cost. `Response.UnknownUsageAttempts` carries N from the
adapter; `Usage` keeps only provider-reported numbers.

The hard cap is untouched: a charge never exceeds the reservation the budget
already holds, and the PostgreSQL constraint
(`tokens_used + tokens_reserved <= token_budget`) is unchanged.

### Consequences

- **Positive:** A conversation whose provider retried once is no longer billed
  several times its real spend, so a booking can finish inside its budget.
- **Negative:** If a failed attempt somehow cost more than the prompt it sent,
  that excess is not charged. It remains bounded by the reservation.
- **Evidence:** `internal/agent/runner.go` (`chargeForModelCall`),
  `internal/agent/runner_test.go` (`TestChargeForModelCall`),
  `internal/llm/openrouter.go`, `internal/llm/openrouter_test.go`
  (`TestOpenRouterAdapterCountsAttemptsThatReportedNoUsage`).

---

## ADR-015: A confirmed call executes the server's frozen action

- **Status:** accepted
- **Supersedes:** none (changes how ADR-001 is enforced)

### Context

ADR-001 binds a mutation to the exact proposed facts. Enforcement compared the
model's re-sent arguments against the frozen ones and refused the call on any
difference. That made the model responsible for reproducing a ~600-character
signed `slot_token` verbatim on the confirming turn. A small model corrupts it,
the gateway reported `slot token is invalid or has been tampered with`, and a
booking the customer had already approved died.

### Decision

When a call carries a `confirmation_id` that matches the conversation's own live
proposal for that tool, the gateway replaces the call's arguments with the exact
`ArgumentsJSON` frozen when the proposal was shown, then dispatches. An unknown
or mismatched id changes nothing and the existing confirmation checks still
decide whether the call may execute.

### Consequences

- **Positive:** Tampering after consent is impossible by construction rather
  than detected after the fact, and the model never has to echo an opaque token
  to complete a booking the customer approved.
- **Negative:** A model that changes an argument after confirmation gets no
  distinct error; its edit is silently discarded. The executed action is still
  exactly the one the customer saw.
- **Evidence:** `internal/tools/gateway.go` (`restoreFrozenAction`),
  `internal/tools/gateway_test.go`
  (`TestConfirmationRejectsChangedArgumentsAndCrossOwner`,
  `TestConfirmationRejectsAnUnknownConfirmationID`).
