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
- **One stage per capability boundary.** Channels, reminders, the operator
  console, multi-tenancy, and hardening are separable concerns and ship in
  that order, because each depends on the one before it.

## Status at a glance

| Stage | Focus | State |
| --- | --- | --- |
| 1 | Conversation-to-booking core | **Shipped** |
| 2 | Channels (widget, SSE, Telegram, edge protection) | **Shipped** |
| 3 | Reminders and CRM hand-off | Next |
| 4 | Operator console | Planned |
| 5 | Multi-tenancy and identity | Planned |
| 6 | Production hardening and launch | Planned |

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

---

## Planned

Each stage below lists its **scope**, concrete **deliverables**, and the
**exit criteria** that mark it done (mirroring the existing gates: `go test
-race`, the PostgreSQL integration suite, and the authenticated Compose smoke
in CI).

### Stage 3 — Reminders and CRM hand-off (next)

**Why now.** The product story in the README ends the booking flow with two
hand-offs — "update the customer in the CRM and send a reminder" — that are
not built. The worker binary is a placeholder that says so directly: it logs
`durable reminder jobs arrive in Stage 3` and otherwise only applies
migrations and idles. Stage 3 makes that worker real and closes the
book → CRM → reminder loop.

**Scope.**
- A durable outbox/jobs table written transactionally alongside the booking
  it belongs to (the same pattern Stage 2 used for `conversation_events`).
- The `cmd/worker` process becomes a real job runner: claim, execute, retry
  with backoff, and dead-letter, with at-least-once delivery and idempotent
  side effects.
- A notification adapter for customer reminders (email and/or SMS) behind an
  interface, with a no-op/log driver for the demo.
- A CRM adapter so `upsert_crm_contact` and `create_deal` stop returning
  `NOT_IMPLEMENTED`; start with a CSV/log driver and one real provider
  (HubSpot) behind the same `tools.Backend` interface (which today implements
  only `list_services`, `list_staff`, `find_slots`, `create_booking`, and
  `escalate_to_human`).
- Implement `reschedule_booking` and `cancel_booking`, which currently return
  `NOT_IMPLEMENTED`, so the booking lifecycle is complete.

**Deliverables.**
- Migration `000003` adding the outbox/jobs (and any reminder-schedule) tables.
- Worker job loop with claim/lease, retry policy, and dead-letter replay.
- `internal/notifications` and `internal/crm` packages with interface + demo
  driver + one real driver each.
- Reschedule/cancel wired through the confirmation gateway (both are
  destructive and must be confirmed).

**Exit criteria.**
- A confirmed booking durably enqueues a reminder and a CRM upsert in the same
  transaction; the worker delivers each exactly once under retries, and a
  permanently failing job lands in the dead-letter table.
- Reschedule and cancel run through two-phase confirmation and keep booking
  consistency.
- `go test -race` (incl. new integration tests) and the Compose smoke assert
  the enqueue-and-deliver path.

### Stage 4 — Operator console

**Why now.** Everything the operator needs already exists in the database
(runs, traces, bookings, escalations) but there is no way to see it. The
design system for this UI is now in the repo at
[`design/design-system/`](design/design-system/), and the screens are already
designed in [`design/screens/`](design/screens/).

**Scope.**
- A browser application for operators: live dashboard (KPIs + status), a runs
  list, a click-through agent-trace viewer, and a week calendar with
  create/reschedule/cancel.
- Read APIs to back it: runs listing with filters, dashboard aggregation
  queries (today the dashboard numbers are fixture values — a real aggregation
  query does not exist yet), and calendar reads.
- Build on the imported design system (`_ds_bundle.js`: `Timeline`,
  `DataTable`, `WeekCalendar`, `Drawer`, `Chart`, etc.) so the console matches
  the designed screens.

**Deliverables.**
- `internal/channels/operatorhttp` (or equivalent) read/command endpoints,
  authorized by the Stage 5 operator identity (until then, guarded behind a
  single admin token and not exposed publicly).
- A front-end app wired to the design system and its tokens, with the Geist
  fonts added (see the design-system note below).
- Dashboard aggregation queries replacing the fixture metrics.

**Exit criteria.**
- An operator can list runs, open a run and read its full nested trace, watch
  live status, and manage bookings on the calendar against real data.
- The console is accessibility-audited (the design system ships a11y
  affordances; the app must preserve them).

### Stage 5 — Multi-tenancy and identity

**Why now.** The runtime is a single fixed tenant (`Salon Nord`) with no user
identity beyond catalogue staff, and the operator console from Stage 4 needs
real logins. This stage turns the demo into something a second business could
use.

**Scope.**
- Operator authentication (login, sessions) and authorization (roles:
  owner/staff), replacing the single admin token.
- Runtime tenant resolution: derive the tenant from the request/host/webhook
  instead of the fixed env tenant, with strict per-tenant data isolation
  (the schema already carries `tenant_id` everywhere).
- Tenant onboarding: create a tenant, its catalogue, staff, schedule, and
  channel configuration (widget origin, Telegram bot) without editing env.
- Per-tenant secrets and configuration.

**Deliverables.**
- Identity + session + RBAC tables and middleware.
- Tenant provisioning flow and per-tenant channel/config storage.
- Authorization tests proving cross-tenant isolation on every surface.

**Exit criteria.**
- Two tenants run side by side with no data or configuration bleed, each with
  its own operators, catalogue, and channels; an operator only ever sees their
  tenant.

### Stage 6 — Production hardening and launch

**Why now.** With features complete, launch is about operability, safety, and
scale. Many items here are called out individually in *Release readiness*
below; this stage is where they are closed.

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
- [ ] **No operator authentication or authorization.** The only auth today is
  the per-conversation customer bearer capability. There is no operator login,
  session, or role model. (Stage 5)
- [ ] **Demo secrets by default.** `SLOT_TOKEN_SECRET` ships as
  `demo-only-change-me-…`; production needs real secrets and a real
  secrets-management story, not compose env defaults.
- [ ] **CORS defaults to `*`.** Fine for the zero-key demo; must be locked to
  each tenant's widget origin before launch.
- [ ] **No abuse protection** on conversation creation beyond the in-memory IP
  limiter (no bot/captcha, no per-tenant quotas).
- [ ] **No dependency/vulnerability scanning or SAST** in CI.

### Multi-tenancy and identity
- [ ] **Single fixed tenant.** No runtime tenant resolution, onboarding, or
  per-tenant branding/config. (Stage 5)
- [ ] **No user identity model** beyond catalogue staff rows.

### Real integrations (currently stubbed)
- [ ] **LLM is the `fake` deterministic adapter by default.** OpenRouter is
  wired but needs a key, a chosen model, cost controls, and prompt/version
  pinning validated against a real model.
- [ ] **Calendar sync is a `noop`.** No external calendar (Google/Microsoft)
  read/write; double-booking protection is DB-only.
- [ ] **CRM tools return `NOT_IMPLEMENTED`.** No HubSpot/CSV adapter. (Stage 3)
- [ ] **No email/SMS provider** for customer reminders/notifications. (Stage 3)
- [ ] **Reschedule and cancel return `NOT_IMPLEMENTED`.** (Stage 3)

### Reliability and the worker
- [ ] **The worker is a no-op.** `cmd/worker` only applies migrations and idles;
  there is no outbox/job processing, retry loop, or dead-letter replay. (Stage 3)
- [ ] **Effectively single-instance.** The rate limiter and other state live in
  process memory; horizontal scaling needs a shared limiter and stateless SSE
  fan-out. (Stage 6)
- [ ] **No load/soak testing** under concurrent turns beyond unit/integration
  coverage.

### Observability and operations
- [ ] **No metrics, tracing, error tracking, dashboards, or alerting.**
  Structured logs plus `/healthz` and `/readyz` exist; nothing else. (Stage 6)
- [ ] **No operator-facing audit/alerting** on escalations or failures (the data
  is in the DB; nothing surfaces it).

### Operator experience
- [ ] **No operator UI.** Dashboard, runs list, trace viewer, and calendar exist
  only as static designs. The design system to build them is now imported. (Stage 4)
- [ ] **Dashboard metrics are fixtures** — no aggregation query exists yet. (Stage 4)

### Data, migrations, and privacy
- [ ] **No rollback path.** Migrations are forward-only and checksum-guarded;
  there are no down migrations or a documented restore procedure.
- [ ] **No backup/restore runbook**, point-in-time recovery, or retention policy.
- [ ] **PII without a lifecycle.** Customer name, email, and message content are
  stored with no retention, export, or erasure policy; no privacy notice or
  widget consent. GDPR/CCPA obligations are unmet. (Stage 6)

### Deployment and scaling
- [ ] **Local/demo deploy only.** Docker + Compose + nginx exist; there is no
  production deploy (hardened single-host or k8s/Helm), TLS/cert management,
  secret injection, resource limits, or a zero-downtime upgrade + migration
  story for multiple instances. (Stage 6)

### Testing and QA
- [ ] **No browser/E2E tests** for the widget, **no load tests**, and **no
  accessibility audit** of the vanilla `kontor.js` widget.
- [ ] **No Telegram contract test** against a fake Bot API in CI (the handler is
  unit-tested; the wiring is not exercised in CI).

### Documentation
- [ ] Missing for launch: a **deployment/ops runbook**, an **OpenAPI/API
  reference**, a **customer widget-embedding guide**, and a **Telegram setup
  guide**. (`README.md` and `docs/ENGINEERING.md` cover the engine well.)

---

## Design system

The Kontor & DocMind design system is now vendored at
[`design/design-system/`](design/design-system/) so the Stage 4 operator
console can be built against the same visual language as the designed screens.

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

**Before Stage 4:** the Geist and Geist Mono variable `woff2` files are not
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
