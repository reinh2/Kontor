# Kontor — Roadmap

Kontor is a self-hosted AI agent that turns a customer conversation into a
confirmed booking, safely. This document is the plan for getting from the
current backend to a launchable product. It is written the same way as
[`README.md`](README.md) and [`docs/ENGINEERING.md`](docs/ENGINEERING.md):
it describes what exists today and what is deliberately not built yet, not a
wish list.

## How to read this roadmap

Four rules keep the plan honest and the stages small:

- **Ship code, not promises.** A stage is done when its behaviour is in the
  repository, tested, and verified end to end — not when it is designed.
- **Grow the schema one stage at a time.** Migrations are forward-only and
  checksum-guarded. Each stage adds only the tables its runtime needs; the
  schema never speculates ahead of the code.
- **The core invariants hold on every surface.** Whatever channel or UI is
  added, nothing may bypass the four guarantees the backend already enforces:
  book only after explicit confirmation, authorize every action with a
  conversation capability, respect the per-conversation token budget, and
  keep booking consistency serialized in PostgreSQL.
- **One stage per capability boundary.** Channels, design, reminders, the
  operator console, multi-tenancy, and hardening are separable concerns and
  ship in that order, because each depends on the one before it.

## Status at a glance

| Stage | Focus | State |
| --- | --- | --- |
| 1 | Conversation-to-booking core | **Shipped** |
| 2 | Channels (widget, SSE, Telegram, edge protection) | **Shipped** |
| 3 | Design implementation (customer widget + operator screens) | **Shipped** |
| 4 | Reminders and CRM hand-off | **Shipped** |
| 5 | Operator console (live data) | **Shipped** |
| 6 | Multi-tenancy and identity | **Shipped** |
| 7 | Production hardening and launch | **In progress** 🚧 |

---

## Shipped

### Stage 1 — Conversation-to-booking core ✅

The agent runtime: a customer message runs one bounded agent turn that can
read the catalogue and availability, propose a booking, and — only after the
customer explicitly confirms — commit it.

- Bounded agent loop over a deterministic demo model or OpenRouter, with a
  per-turn iteration and time budget and a persistent per-conversation token
  budget.
- A tool executor behind a schema, capability, and confirmation gateway;
  retryable tool failures get bounded attempts with backoff.
- Two-phase confirmation: a proposal is stored and nothing is booked until the
  customer confirms; confirmations are bound to the exact proposed facts.
- Booking consistency: availability and double-booking protection are
  serialized in PostgreSQL; idempotency keys make a confirm safe to retry.
- Durable agent traces, escalation to a human, and dead-letter capture of
  provider failures.
- A small JSON demo API with a one-time, conversation-scoped bearer capability
  (only its SHA-256 digest is stored).

### Stage 2 — Channels ✅

Three delivery surfaces in front of the same core, none of which can bypass
its guarantees.

- **Embeddable widget** served from the API binary as one `<script>` tag,
  rendered in a closed shadow root, holding the capability in `sessionStorage`
  and streaming replies over fetch-based SSE.
- **Durable SSE stream:** each committed turn writes a `conversation_events`
  row in the same transaction as its reply, so replay from `Last-Event-ID`
  never exposes an outcome that later disappears.
- **Telegram webhook** behind a constant-time secret check, with per-`update_id`
  idempotency, one-conversation-per-chat binding, and a bounded retrying sender.
- **Edge protection:** a per-client-IP token-bucket rate limiter in front of
  the turn-admission queue and a configurable CORS policy; health and readiness
  probes bypass the limiter.

### Stage 3 — Design implementation ✅

The static designs become a real, browsable front-end served from the API
binary.

- **Redesigned customer widget** (`web/widget/kontor.js`) matching the
  `Kontor Customer Chat` design screen: dark surface with Iris accent, Geist
  font stack, hairline borders, skeleton shimmer loading, confirmation cards
  with a facts table, escalation notice, and full ARIA accessibility
  (`role=dialog`, `role=log`, `aria-live=polite`, `aria-label` on all
  controls, `focus-visible` rings). Shadow root preserved.
- **Operator console SPA** (`web/operator/`) built on the vendored DS bundle
  (`_ds_bundle.js`) against fixture data with hash-based routing: Dashboard
  (6 KPI cards with sparklines, bar and donut charts, recent-runs table,
  attention panel), Agent Trace (two-pane conversation + timeline with retry
  nesting, summary header, key-value block), Runs List (full DataTable with
  15 rows), and Week Calendar (staff columns, booking blocks, now-line).
- **Go embed wiring:** three new routes (`/operator`, `/operator/ds/styles.css`,
  `/operator/ds/bundle.js`) serve the operator SPA and inlined DS tokens
  (14 KB combined CSS) from the binary.
- **Geist font placeholder** (`design/fonts/README.md`) with instructions for
  adding the variable woff2 builds; the system fallback works until then.
- **Responsive:** operator screens stack below 1100 px; sidebar hides below
  760 px; KPI grid collapses from 6 → 3 → 2 columns.
- No backend, migration, or test changes.

### Stage 4 — Reminders and CRM hand-off ✅

The worker binary becomes a real job runner, closing the
book → CRM → reminder loop.

- **Durable job queue** (`internal/jobqueue`): claim with `FOR UPDATE SKIP
  LOCKED`, exponential backoff retry (30 s base, 2^n, capped at 1 h),
  dead-letter after max attempts, and idempotent enqueue via
  `ON CONFLICT DO NOTHING`.
- **Migration `000003`** adds `jobs` and `dead_letter_jobs` tables with
  FK to bookings, partial indexes for the worker poll, and idempotency
  deduplication.
- **Real worker** (`cmd/worker`): poll loop with configurable concurrency
  (default 4), per-job timeout, graceful shutdown, and dispatch by job type.
- **Notification adapter** (`internal/notifications`): `Notifier` interface
  with a `LogNotifier` demo driver for email/SMS reminders.
- **CRM adapter** (`internal/crm`): `CRM` interface with a `LogCRM` demo
  driver and a `HubSpotCRM` real driver (contacts + deals API, behind a
  feature flag).
- **`reschedule_booking` and `cancel_booking` implemented:** both pass through
  the two-phase confirmation gateway (same pattern as `create_booking`),
  use serializable PostgreSQL transactions with schedule locks, record
  booking events, and support idempotency replay.
- **Transactional outbox:** a confirmed booking atomically enqueues
  `send_reminder` and `crm_upsert_contact` jobs in the same transaction,
  guaranteeing delivery if and only if the booking commits.

---

## In progress and planned

Each stage below lists its **scope**, concrete **deliverables**, and the
**exit criteria** that mark it done (mirroring the existing gates: `go test
-race`, the PostgreSQL integration suite, and the authenticated Compose smoke
in CI).

### Stage 5 — Operator console (live data) 🚧

**Why now.** Stage 3 delivers the operator screens against fixture data;
Stage 4 adds the backend data the operator needs (reminders, CRM, full booking
lifecycle). This stage wires the two together: the fixture mocks are replaced
with real API calls, so the operator sees live runs, traces, bookings, and
escalations.

**Implemented first slice.** The console now has tenant-scoped live read APIs
for dashboard aggregates, an allowlisted/filterable keyset-paginated run feed,
full run detail (bounded messages, bookings, escalations, iterations, tools,
and nested attempts), and tenant-timezone calendar reads. They are disabled
unless an explicit 32+ byte `OPERATOR_ADMIN_TOKEN` is configured, execute
outside the widget CORS branch, and return `no-store` responses. The SPA uses
tab-scoped token storage, live polling, real run routes, and loading/error/empty
states; React is pinned and embedded rather than executed from a CDN.

Calendar create/reschedule/cancel now ship end to end. A dedicated admin path
in the scheduling repository records `admin` actors in `booking_events`,
enforces an expected `schedule_version` (optimistic concurrency) on reschedule
and cancel, and transactionally moves or cancels the booking's reminder job —
deliberately not reusing the customer-oriented `tools.Gateway`, which would
produce incorrect audit and stale writes. Migration `000005` adds a
`cancelled` job state for retired reminders. Three admin-token-guarded POST
endpoints (`/api/v1/operator/bookings`, `.../{id}/reschedule`,
`.../{id}/cancel`) expose them, plus a `GET /operator/customers` search
endpoint for the create-booking picker; all covered by unit and PostgreSQL
integration tests. The SPA calendar is wired to all three commands: an "Add
appointment" button and a per-slot `+` affordance open a create form with a
debounced customer search, clicking a booking opens a detail drawer with
Reschedule/Cancel actions, and a 409 schedule-version conflict surfaces an
explicit "reload and try again" affordance instead of a generic error. Times
are converted between the operator's wall-clock input and UTC using the
tenant's IANA timezone (verified against a DST transition).

**Scope.**
- A browser application for operators: live dashboard (KPIs + status), a runs
  list, a click-through agent-trace viewer, and a week calendar with
  create/reschedule/cancel. **Implemented.**
- Read APIs to back it: runs listing with filters, dashboard aggregation
  queries, trace enrichment, calendar reads, and a customer search, plus the
  admin-safe booking commands. **Implemented.**
- Build on the imported design system (`_ds_bundle.js`: `Timeline`,
  `DataTable`, `WeekCalendar`, `Drawer`, `Chart`, etc.) so the console matches
  the designed screens; the booking modal and drawer fall back to plain markup
  when a bundle component (`Modal`/`Drawer`) is unavailable.

**Deliverables.**
- `internal/channels/operatorhttp` read and command endpoints are present and
  guarded by an opt-in single admin token; the create/reschedule/cancel commands
  carry admin-aware audit, optimistic version checks, and transactional reminder
  updates. Stage 6 replaces the token with operator identity.
- A front-end app wired to the design system and its tokens, with the Geist
  fonts added (see the design-system note below).
- Dashboard aggregation queries replacing the fixture metrics. **Implemented.**

**Exit criteria.**
- An operator can list runs, open a run and read its full nested trace, watch
  live status, and manage bookings on the calendar against real data.
  **Met.**
- The console is accessibility-audited (the design system ships a11y
  affordances; the app must preserve them). The new controls carry
  `aria-label`/`role` and `focus-visible` styling consistent with the rest of
  the SPA, but a full assistive-technology audit (as opposed to following the
  existing patterns) has not been performed and remains open.

### Stage 6 — Multi-tenancy and identity ✅

**Shipped.** Kontor now resolves public widget/customer traffic by tenant host,
operator traffic from a server-validated session, and Telegram traffic from a
tenant webhook path. It provisions tenant-local catalogues, staff, schedules,
widget origins, and encrypted Telegram configuration without environment-file
edits.

**Delivered scope.**
- Opaque PostgreSQL-backed operator sessions with login, logout, owner/staff
  RBAC, server-side revocation, and no Stage 5 shared-admin-token fallback.
- Runtime tenant resolution and repository routing across customer, operator,
  onboarding, and Telegram surfaces, with no caller-controlled tenant selector.
- Owner-only onboarding/configuration APIs, canonical per-tenant widget CORS,
  and tenant-isolated channel secrets.
- A fail-closed migration bootstrap for pristine legacy tenants, requiring an
  explicit owner identity and exact idempotent replay.

**Delivered.**
- Identity, operator-session, and tenant-channel schema/middleware.
- Tenant provisioning and tenant-local operator/catalogue/channel storage.
- Unit, race, and PostgreSQL integration coverage for session middleware,
  tenant origin, onboarding, legacy bootstrap, tenant isolation, and Telegram
  retry behaviour.

**Exit criteria — met.**
- Two tenants run side by side without data or configuration bleed, each with
  isolated operators, catalogues, widget origins, and channels; an operator
  only accesses the tenant encoded by their validated session.

### Stage 7 — Production hardening and launch

**Why now.** With features complete, launch is about operability, safety, and
scale. Many items here are called out individually in *Release readiness*
below; this stage is where they are closed.

**Implemented first slice.** Observability and supply-chain hardening have
started. The API now serves an opt-in Prometheus `/metrics` endpoint
(`internal/platform/metrics`, standard-library only, no new runtime dependency)
instrumented with request totals by method and status code, a request-latency
histogram, an in-flight gauge, an edge rate-limiter rejection counter, and
build/start-time info. Long-lived SSE streams are counted but excluded from the
latency histogram so they cannot pin every observation in the `+Inf` bucket; the
endpoint sits outside the rate limiter and the widget CORS edge, returns
`no-store`, and requires a bearer token when `METRICS_TOKEN` is set (it is not
mounted at all unless `METRICS_ENABLED=true`). Startup now **fails closed** when
the public demo `SLOT_TOKEN_SECRET` or tenant channel encryption key is used
with `DEMO_MODE=false`, so the compose defaults can no longer reach production
silently. CI gained a security job running `gofmt`, `govulncheck` (dependency
and vulnerability scanning), and `gosec` (SAST with reviewed rule exclusions);
the first `govulncheck` run surfaced `GO-2026-5970` in `golang.org/x/text`,
fixed by upgrading to `v0.39.0`. All changes are covered by unit tests and pass
`go test -race`. Distributed tracing, error tracking, dashboards, alerting,
shared-state horizontal scaling, secrets management, abuse protection, the
PII lifecycle, rollback/backup procedures, and a real production deploy remain
open below.

**Scope.**
- Observability: metrics, distributed tracing, error tracking, dashboards, and
  alerting.
- Horizontal scale: move the in-memory rate limiter and any in-process state to
  a shared store so more than one API instance is safe; zero-downtime deploys
  and migration ordering.
- Security review and secrets management; production CORS locked per tenant;
  dependency and static-analysis scanning in CI.
- Privacy and compliance: data retention, export, and erasure for customer PII;
  privacy notice and widget consent.
- Backup/restore runbook and an operator playbook.

**Exit criteria.**
- A documented, repeatable production deploy with TLS, secrets, backups, and
  rollback; load-tested to a target concurrency; a security and privacy review
  signed off; on-call alerting live.

---

## Release readiness — what's missing to launch

A gap analysis of the current codebase against a production launch, grouped by
concern. Items are roughly ordered by how much they block a real customer.
Boxes are unchecked because they are open.

### Security and authentication
- [x] **Operator authentication and authorization.** Stage 6 provides
  tenant-local operator identities, opaque database-backed sessions, owner/staff
  RBAC, and server-side revocation; operator authority is never derived from a
  client-provided tenant value.
- [ ] **Demo secrets by default.** `SLOT_TOKEN_SECRET` ships as
  `demo-only-change-me-…`. Stage 7 makes startup **fail closed** when that value
  or the demo tenant channel key is used with `DEMO_MODE=false`, so the public
  defaults can no longer reach production silently; a real secrets-management
  story (injection, rotation) is still open.
- [x] **Widget CORS is tenant-scoped.** Public customer and widget routes
  resolve the tenant from the host and require that tenant's canonical widget
  origin; operator and provisioning routes are outside that CORS edge.
- [ ] **No abuse protection** on conversation creation beyond the in-memory IP
  limiter (no bot/captcha, no per-tenant quotas).
- [x] **Dependency/vulnerability scanning and SAST in CI.** (Stage 7) —
  **Done.** CI runs `govulncheck` (gating) and `gosec` (SAST, with reviewed
  false-positive exclusions) beside a `gofmt` gate. The first `govulncheck` run
  found and fixed `GO-2026-5970` in `golang.org/x/text` (upgraded to `v0.39.0`).

### Multi-tenancy and identity
- [x] **Runtime tenant resolution and onboarding.** Stage 6 resolves customer
  traffic by host, operator traffic by validated session, and Telegram traffic
  by tenant webhook path, with tenant-local catalogue, schedule, and channel
  configuration.
- [x] **Operator identity model.** Tenant-local owner and staff identities are
  persisted separately from catalogue staff rows and authorized by live
  sessions.

### Real integrations (currently stubbed)
- [ ] **LLM is the `fake` deterministic adapter by default.** OpenRouter is
  wired but needs a key, a chosen model, cost controls, and prompt/version
  pinning validated against a real model.
- [ ] **The deterministic demo adapter is not a valid conversational booking
  simulation.** It ignores the requested service, date, and time and always
  drives the fixed Haircut/Alex/next-Thursday scenario. It is useful for a
  repeatable safety smoke test, but misleading in the public widget and unable
  to demonstrate natural-language booking. **Fix:** label it as a scripted
  smoke scenario, and make the default demo either collect structured inputs
  or use a provider-backed fixture/evaluation that honours the request.
- [x] **The real-model booking protocol is directed for safe recovery.** The
  provider-neutral prompt and tool contracts now require service → staff → slot
  discovery, require a concise email/E.164-phone clarification before booking,
  and require a clarification (not speculative retry) after `fix_arguments`.
  Contacts are extracted only from the authenticated customer's saved message;
  the gateway still derives booking identity from trusted context. Regressions
  cover no-contact requests, schema-invalid calls, and a later valid contact.
  **Remaining:** a versioned multilingual provider/model eval suite and its
  release gate (valid booking, unavailable slot, malformed arguments, retry,
  and confirmation).
- [x] **A stale confirmation card no longer survives a newer or failed turn.**
  A non-consent turn invalidates live proposals server-side, and every durable
  turn event includes the explicit `pending_confirmation_active` snapshot. The
  widget removes an old card on a false snapshot; clarification and
  superseding-intent regressions cover the database state and SSE payload.
- [ ] **No end-to-end provider contract test runs in CI.** Unit tests validate
  the normalized OpenRouter request/response adapter against a local HTTP
  fixture, but CI never exercises a model with the actual tool definitions and
  a seeded catalogue. **Fix:** run a budget-capped, opt-in nightly evaluation
  against the selected provider/model, record only redacted traces and model
  metadata, and keep deterministic protocol tests mandatory on every PR.
- [ ] **Calendar sync is a `noop`.** No external calendar (Google/Microsoft)
  read/write; double-booking protection is DB-only.
- [x] **CRM tools return `NOT_IMPLEMENTED`.** No HubSpot/CSV adapter. (Stage 4) — **Done.** `internal/crm` with LogCRM + HubSpotCRM drivers.
- [x] **No email/SMS provider** for customer reminders/notifications. (Stage 4) — **Done.** `internal/notifications` with LogNotifier driver.
- [x] **Reschedule and cancel return `NOT_IMPLEMENTED`.** (Stage 4) — **Done.** Both wired through two-phase confirmation.

### Reliability and the worker
- [x] **The worker is a no-op.** `cmd/worker` only applies migrations and idles;
  there is no outbox/job processing, retry loop, or dead-letter replay. (Stage 4) — **Done.** Real job runner with claim/retry/dead-letter.
- [ ] **Effectively single-instance.** The rate limiter and other state live in
  process memory; horizontal scaling needs a shared limiter and stateless SSE
  fan-out. (Stage 7)
- [ ] **No load/soak testing** under concurrent turns beyond unit/integration
  coverage.

### Observability and operations
- [ ] **Metrics only; no tracing, error tracking, dashboards, or alerting.**
  Stage 7 adds an opt-in Prometheus `/metrics` endpoint (HTTP request totals,
  latency histogram, in-flight gauge, rate-limit rejections, build info) on top
  of the structured logs and `/healthz`/`/readyz`. Distributed tracing, error
  tracking, dashboards, and alerting remain. (Stage 7)
- [ ] **No proactive operator alerting** on escalations or failures. The live
  console now surfaces them, but there are no push notifications or on-call
  alerts. (Stage 7)

### Operator experience
- [x] **Operator live reads.** Dashboard, runs, nested traces, and calendar use
  a session-authenticated, tenant-local API. (Stage 5, secured by Stage 6)
- [x] **Dashboard metrics are aggregated from PostgreSQL**; token usage is
  reported as tokens because provider pricing is not persisted. (Stage 5)
- [x] **Operator calendar commands.** Owner-authorized
  create/reschedule/cancel endpoints retain audit actors, optimistic
  `schedule_version` checks, and transactional reminder cancel/update, wired
  into the SPA calendar (create modal, detail drawer, conflict handling).
  (Stage 5, authorized by Stage 6 sessions) — **Done.**
- [x] **Operator navigation and design-package integration.** The sidebar and
  responsive mobile navigation now include Inbox, Analytics, and Settings.
  **Overview** is the canonical label while `#/` and `#/dashboard` remain
  compatible Dashboard aliases; Runs and Calendar routes are unchanged. Inbox
  uses tenant-scoped escalated runs, Analytics uses the authoritative dashboard
  aggregate, and Settings reads the authenticated operator/tenant session with
  explicit owner/staff boundaries. Each has loading, retryable error, and
  applicable empty states. The full `Kontor agent trace screen` package was
  inventoried and mapped into the embedded widget/operator UI; provenance,
  asset review, and per-specimen adoption are recorded in
  [`docs/design-integration.md`](docs/design-integration.md). No new frontend
  build pipeline or dependency was introduced; embedded-asset regression tests
  cover the new routes and safe widget components. (2026-07-23)

### Data, migrations, and privacy
- [ ] **No rollback path.** Migrations are forward-only and checksum-guarded;
  there are no down migrations or a documented restore procedure.
- [ ] **No backup/restore runbook**, point-in-time recovery, or retention policy.
- [ ] **PII without a lifecycle.** Customer name, email, and message content are
  stored with no retention, export, or erasure policy; no privacy notice or
  widget consent. GDPR/CCPA obligations are unmet. (Stage 7)

### Deployment and scaling
- [ ] **Local/demo deploy only.** Docker + Compose + nginx exist; there is no
  production deploy (hardened single-host or k8s/Helm), TLS/cert management,
  secret injection, resource limits, or a zero-downtime upgrade + migration
  story for multiple instances. (Stage 7)

### Testing and QA
- [ ] **No browser/E2E tests** for the widget, **no load tests**, and **no
  accessibility audit** of the vanilla `kontor.js` widget.
- [ ] **No browser regression coverage for pending-confirmation state across
  provider failures or clarification replies.** A user can see an old booking
  card beside a newer model response; this needs an end-to-end test using the
  widget's persisted conversation state.
- [ ] **No Telegram contract test** against a fake Bot API in CI (the handler is
  unit-tested; the wiring is not exercised in CI).

### Documentation
- [ ] Missing for launch: a **deployment/ops runbook**, an **OpenAPI/API
  reference**, a **customer widget-embedding guide**, and a **Telegram setup
  guide**. (`README.md` and `docs/ENGINEERING.md` cover the engine well.)

---

## Design system

The Kontor & DocMind design system is now vendored at
[`design/design-system/`](design/design-system/) so the Stage 3 design
implementation can build a polished widget and operator screens against the
same visual language as the authored designs.

- `_ds_bundle.js` — the compiled React component library (global namespace
  `KontorKanonDesignSystem_452420`): `Timeline`, `DataTable`, `WeekCalendar`,
  `Drawer`, `Modal`, `Toast`, `Chart` (Sparkline/Bar/Donut), `CodeBlock`,
  `KeyValue`, `Sidebar`, `Tabs`, `Badge`, `Button`, forms, and feedback states.
- `_ds_manifest.json` — machine-readable index of components, tokens, and
  screens. `readme.md` — the design language (dark-default, single Iris accent,
  Geist type, Lucide icons, hairline-not-shadow depth, accessibility notes).
- `styles.css` + `tokens/` — the token entry point and the seven token files
  (identical to the pre-existing [`design/tokens/`](design/tokens/), kept here
  so the bundle is self-contained).
- `support.js` — the `dc-runtime` that renders the `.dc.html` specimens in
  [`design/screens/`](design/screens/) by mounting them with React/ReactDOM.

**Font status (post Stage 3):** the Geist and Geist Mono variable `woff2` files are not
included (only `@font-face` rules with a system fallback, so nothing breaks).
Add them to `design/fonts/` from Vercel's Geist repository for pixel-accurate
rendering. The component sources are compiled into the bundle; if the console
needs to fork a component, the manifest maps each one to its source path.

---

## Non-goals (for now)

To keep the stages small, these are explicitly out of scope until a stage calls
for them: billing/subscriptions and usage metering, a marketing site, mobile
apps, analytics beyond the operator dashboard, and any channel beyond the
widget and Telegram (email-in, WhatsApp, etc.). They are deferred, not
rejected.

---

## Code review — full audit (2026-07-23)

A whole-repository review (Go, 98 source files, ~15.6k non-test lines).
`gofmt -l`, `go vet ./...`, `go test ./...`, and `go test -race -count=1 ./...`
all pass cleanly on Go 1.26.5 — so the items below are **latent logic and
reliability defects, not build breaks**. Each names the file/symbol, the
concrete failure, and a fix direction.

> Note: this contradicts the stale `docs/current-status.md` /
> `docs/architecture.md` claim of a failing `tenanthttp` test and unformatted
> Stage-6 files — both are resolved (see D-3).

### Verified correct

The safety core holds up under review: DB-level double-booking protection
(`bookings_no_staff_overlap` exclusion constraint + serializable transactions +
`schedule_locks`), two-phase confirmation bound to
tenant/customer/conversation/tool/arguments-hash with an assistant-message-seen
`EXISTS` check, tenant isolation from server-built principals
(`operatorhttp.MultiTenantPostgreSQL.scoped`, `ToolBackend.authorizeTenant`),
identity (PBKDF2-SHA256 600k, constant-time compare, digest-only sessions,
AES-GCM with per-tenant AAD), the save-first / SSE-after-commit ordering in
`internal/app`, and the OpenRouter adapter's bounded retry/backoff/size limits.

### Defects found

- [x] **D-1 · Medium · correctness — customer reschedule/cancel don't map DB
  errors or retry serialization conflicts.** — **Fixed 2026-07-23.**
  In [`internal/scheduling/repository.go`](internal/scheduling/repository.go:396)
  the customer `RescheduleBooking` and
  [`CancelBooking`](internal/scheduling/repository.go:546) run their transaction
  **once** and return raw wrapped errors, unlike `CreateBooking` and all three
  `Admin*` paths, which loop on `isTransactionRetry` and funnel every failure
  through `mapDatabaseError`. Consequences: (a) a reschedule onto a now-occupied
  slot raises the exclusion-constraint violation `23P01`, which is returned as a
  raw `"update booking: %w"` — so
  [`mapToolBackendError`](internal/scheduling/tool_backend.go:333) misses
  `ErrSlotUnavailable` and falls to `ErrDependencyUnavailable` (**retryable**).
  The gateway then reports `DEPENDENCY_UNAVAILABLE` with `retryable=true`, so
  [`agenttools.Executor`](internal/agenttools/executor.go:89) retries an
  operation that can never succeed up to `AGENT_TOOL_MAX_ATTEMPTS` times, instead
  of the clean `SLOT_UNAVAILABLE` / `find_another_slot` the create path returns;
  (b) a transient `40001` serialization failure on these two paths is not retried
  at all. The double-booking invariant itself is **not** violated — the
  constraint still blocks the overlap — but the customer-facing error and retry
  behaviour are wrong and untested (the only `ErrSlotUnavailable`-on-reschedule
  test covers the *admin* path). **Fix:** wrap both customer paths in the same
  retry loop + `mapDatabaseError` used by `CreateBooking`.

- [x] **D-2 · Medium · reliability — job queue has no stale-claim recovery
  (silent reminder/CRM loss).** — **Fixed 2026-07-23.**
  [`Queue.ClaimBatch`](internal/jobqueue/postgres.go:51) flips rows to
  `status='claimed'` and increments `attempts`, but the queue only ever selects
  `status='pending'` and nothing resets a stranded `claimed` row. If the worker
  dies between claim and terminal write — or, at shutdown,
  [`processOne`](cmd/worker/main.go:166) calls `Fail`/`Complete` with the
  already-cancelled **parent** `ctx` (not `jobCtx`) so those writes fail — the
  job stays `claimed` forever and its `send_reminder` / `crm_upsert_contact`
  side effect is silently dropped. This weakens ADR-007's "guaranteed if and only
  if the booking commits": the enqueue is transactional, but delivery is not
  recoverable. **Fix:** add a reaper/visibility-timeout that returns
  `claimed` rows older than a lease back to `pending` (respecting `attempts`),
  and use a shutdown-safe context for terminal `Fail`/`Complete`.

- [x] **D-3 · Low · documentation — status docs describe problems that no longer
  exist.** — **Fixed 2026-07-23.** [`docs/current-status.md`](docs/current-status.md) and the "known
  technical debt" table in [`docs/architecture.md`](docs/architecture.md:227)
  still report a failing `tenanthttp` test
  (`TestPublicTenantScopesEachHostToItsOwnTenant`) and unformatted Stage-6 files;
  the working tree is clean on all checks. Separately, `internal/platform/metrics/`
  exists in the code but is absent from the architecture repository map. **Fix:**
  refresh `docs/current-status.md`, drop the stale debt row, and add the metrics
  package to the architecture map.

- [x] **D-4 · Low · info-hygiene — the demo HTTP handler echoed raw `err.Error()`
  in responses.** — **Fixed 2026-07-23.** Six sites in
  `internal/channels/demohttp/` wrote raw service/decoder/DB error text into the
  problem `detail`, contrary to the project's own coding standard and unlike
  `operatorhttp`/`onboardinghttp`. Most notably [`getRun`](internal/channels/demohttp/handler.go:142)
  performed the trace lookup *before* the capability check, leaking internal
  error text and a run-existence oracle (404 vs 401) to unauthenticated callers.
  Practical severity was low (run IDs are unguessable UUIDs; the strings were
  generic error text, not secrets), but it violated the standard.

### Resolution (2026-07-23)

All three findings are fixed and verified.

- **D-1** — `internal/scheduling/repository.go`: the customer `RescheduleBooking`
  and `CancelBooking` bodies were extracted into `rescheduleBookingOnce` /
  `cancelBookingOnce` and are now driven by the same 3-attempt
  `isTransactionRetry` loop and `mapDatabaseError` mapping as `CreateBooking` and
  the Admin paths. An overlapping-slot reschedule now returns `ErrSlotUnavailable`
  (→ `SLOT_UNAVAILABLE` / `find_another_slot`), and `40001` conflicts are retried.
  Regression: `internal/scheduling/reschedule_conflict_integration_test.go`.
- **D-2** — `internal/jobqueue/postgres.go`: added `Queue.RequeueStaleClaims`,
  which returns claims older than a lease to `pending` (dead-lettering exhausted
  ones via an atomic `UPDATE … RETURNING` move). `cmd/worker/main.go` runs it on
  a timer (`reapInterval`) and records terminal `Complete`/`Fail` under a
  shutdown-safe `context.WithoutCancel` so a finished job is never stranded.
  Regression: `internal/scheduling/stale_claim_integration_test.go`.
- **D-3** — refreshed `docs/current-status.md`, added `internal/platform/metrics`
  to the architecture map, and removed the stale "1 failing test" debt row.
- **D-4** — `internal/channels/demohttp/`: added an `internalError` helper (log +
  generic 500), moved the `getRun` bearer-token check ahead of the trace lookup,
  added the `app.ErrInvalidMessage` sentinel (→ 400), and replaced every raw
  `err.Error()` in a response with a controlled message. Regression tests:
  `TestGetRunWithoutTokenReturnsUnauthorizedBeforeLookup`,
  `TestGetRunInternalErrorIsGeneric`,
  `TestSendMessageInvalidMessageReturnsBadRequestWithoutLeak`.

Verification: `gofmt -l` (clean), `go vet ./...` (clean), `go test ./...` and
`go test -race -count=1 ./...` (all green). The two new tests are integration
tests that require `TEST_DATABASE_URL`; they compile and skip locally (no
PostgreSQL/Docker was available here) and run in CI.
