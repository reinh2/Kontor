# Product

## Summary

- **Project name:** Kontor
- **One-sentence description:** A self-hosted AI front desk that turns customer messages into safely confirmed appointments for service businesses.
- **Current stage:** Demonstration / portfolio project (not production-ready)
- **Primary owner:** reinhlord

## Problem

Customers of appointment-based businesses must wait for opening hours, switch channels, or repeat their details just to book, move, or cancel an appointment. Staff spend time on scheduling instead of service delivery. Existing AI chat solutions lack server-side safety guarantees: they let the model mutate the calendar without explicit customer consent or database-enforced consistency.

## Target users

| User | Need | Success looks like |
|---|---|---|
| End-customer | Book, reschedule, or cancel an appointment via chat at any hour | A confirmed booking created only after explicit consent, with no double-booking |
| Business operator | Monitor conversations, inspect AI decisions, and manage the calendar | Live dashboard with runs, traces, bookings, and calendar commands |
| Business owner | Deploy a 24/7 front desk without trusting the AI with unchecked authority | All mutations require customer confirmation; audit trail is complete |

## Core user journeys

1. **Book an appointment** — Customer sends a natural-language request; the agent finds real availability, proposes a slot, waits for unambiguous confirmation, then commits the booking and enqueues follow-up jobs (reminder, CRM).
2. **Reschedule or cancel** — Same two-phase confirmation flow; existing booking is moved or cancelled with optimistic version checks and transactional consistency.
3. **Operator oversight** — Operator opens the console, reviews live runs and nested agent traces, sees dashboard KPIs, and manages bookings via the weekly calendar (create, reschedule, cancel with conflict handling).
4. **Escalation** — When the agent cannot resolve the request or the customer asks for a person, the conversation is escalated and persisted for human follow-up.

## Current scope

### In scope

- End-to-end appointment creation, rescheduling, and cancellation with two-phase confirmation.
- Embeddable browser widget with durable SSE, Telegram webhook, and a demo JSON API.
- Operator console with live dashboard, run/trace viewer, and calendar commands.
- Durable job queue with transactional outbox for reminders and CRM updates.
- Bounded agent runtime with iteration/time/token budgets, retrying tool executor, and escalation.
- Multi-tenant data model (tenant_id on all rows); runtime multi-tenancy and identity partially implemented (Stage 6 WIP).

### Non-goals

- Billing, subscriptions, or usage metering.
- Marketing site or mobile apps.
- Analytics beyond the operator dashboard.
- Channels beyond widget and Telegram (email-in, WhatsApp, etc.).
- Production deployment, TLS, or multi-instance operation (Stage 7).
- External calendar synchronization (Google/Microsoft).

## Product requirements

### Functional

- A model cannot create, move, or cancel a booking without prior explicit customer confirmation bound to exact proposed facts.
- Every conversation is scoped by an opaque capability token; only its SHA-256 digest is stored.
- Booking consistency is enforced by PostgreSQL serializable transactions, schedule locks, idempotency keys, and an exclusion constraint.
- Agent runtime has strict per-turn iteration, time, retry, and per-conversation token budgets.
- Committed customer turns are persisted before SSE delivery; replay from Last-Event-ID has no gaps.

### Quality attributes

- **Reliability:** Durable delivery via transactional event storage; bounded agent loop with escalation on failure.
- **Performance:** Not formally budgeted; designed for single-instance demo load.
- **Security:** Untrusted-LLM architecture; server-side authorization; SHA-256 capability digests; operator sessions with digest-only token storage.
- **Accessibility:** Widget and operator console follow ARIA patterns (role, aria-live, aria-label, focus-visible). Full assistive-technology audit not yet performed.
- **Compatibility:** Modern browsers (widget uses shadow DOM, SSE, fetch). Backend is a single Go binary + PostgreSQL 15.

## Constraints

- Single-binary Go deployment; no frontend build step (JS is embedded via `go:embed`).
- PostgreSQL 15 is the only external runtime dependency.
- No paid LLM key required for demo mode (deterministic fake adapter).
- Forward-only migrations; no down-migration path.
- Unlicensed repository; normal copyright restrictions apply.

## Success criteria

- A customer can complete a booking end-to-end via the widget without a single unauthorized calendar mutation.
- An operator can inspect every agent decision and its tool calls in the trace viewer.
- CI passes: `go vet`, `go test -race`, Docker image builds, and the authenticated Compose smoke test.

## Open product questions

- Production model selection, prompt versioning, and cost controls for OpenRouter integration.
- Tenant onboarding UX and self-service configuration flow.
- Privacy/GDPR compliance: data retention, export, erasure, and widget consent.
- External calendar sync strategy (Google Calendar / Microsoft Graph).
