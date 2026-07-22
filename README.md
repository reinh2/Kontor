# Kontor

**An action-taking front desk that turns a customer conversation into a safely confirmed appointment.**

Service businesses lose bookings when a customer has to wait for opening hours, switch channels, or repeat the same details to several systems. A useful agent should do more than answer: it should find a real opening, ask before it changes anything, complete the booking, and leave an audit trail a person can inspect.

## A Thursday-evening haircut

Marta asks, “Can I get a haircut on Thursday evening?” Kontor checks the service catalogue, eligible staff, working hours, breaks, existing bookings, and time zones, then returns a real opening. When Marta chooses a time, Kontor shows the exact appointment and waits for an explicit confirmation before it writes the booking.

The wider business flow continues by updating the customer in the CRM and sending a reminder. Those two hand-offs are visible in the product designs below, but they are not implemented in the current runtime: today’s executable demo finishes with a persisted Kontor customer, a confirmed booking, and a trace of the agent run. This distinction is intentional—this README describes the code that exists, not the roadmap as if it had shipped.

## Screens

![Agent trace showing a customer conversation beside the model and tool timeline](docs/img/trace-live-full@2x.png)

*Agent trace design — the conversation and every model/tool step share one timeline. The shown 2.94 s run, nested retry, CRM step, and confirmation sender are fixture data; the backend currently persists model calls, tool calls, nested attempts, token usage, and total run duration.*

<table>
  <tr>
    <td width="50%"><img src="docs/img/chat-booking@2x.png" alt="Customer booking confirmation flow"></td>
    <td width="50%"><img src="docs/img/dashboard-live@2x.png" alt="Operator dashboard with booking and agent metrics"></td>
  </tr>
  <tr>
    <td><em>Customer chat design — a human-readable confirmation sits between choosing a slot and creating the booking.</em></td>
    <td><em>Operator dashboard design — business outcomes and agent health in one view. The displayed 2.9 s median run latency is seeded fixture data, not a production benchmark.</em></td>
  </tr>
  <tr>
    <td colspan="2"><img src="docs/img/calendar-week-full@2x.png" alt="Week calendar for three staff members"></td>
  </tr>
  <tr>
    <td colspan="2"><em>Calendar design — bookings, breaks, time off, status changes, and conflicts are visible in the weekly operating view.</em></td>
  </tr>
</table>

The images are static exports from [`design/screens`](design/screens); the repository does not yet contain the browser application that makes them interactive.

## What it does

- Runs the haircut flow locally without an API key through a deterministic model substitute, or through the real OpenRouter Chat Completions adapter added in Stage 1. The OpenRouter path sends the exact tool schemas, accepts tool calls, and retries bounded transient provider failures.
- Lists services and eligible staff, then calculates slots on a 15-minute grid across working hours, breaks, booking buffers, busy periods, and IANA time zones, including daylight-saving transitions.
- Rejects searches and bookings outside the temporal safety window: 15 minutes of minimum lead time, a 365-day booking horizon, and a 31-day maximum search range. Booking creation also rechecks against the PostgreSQL clock so a queued request cannot cross into the past.
- Requires a server-authorized, argument-bound confirmation before creating a booking. Slot offers expire after 5 minutes; confirmation proposals have a 10-minute ceiling and cannot outlive the slot offer.
- Rechecks availability inside a serializable transaction and uses both a per-staff/day lock and a PostgreSQL exclusion constraint to prevent double-booking.
- Makes booking requests idempotent, so a repeated client request returns the original booking instead of creating another one.
- Handles multiple tool calls returned in one model response. Calls are processed sequentially in response order and all results are appended before the next model request; after a terminal refusal or human hand-off, remaining siblings are traced as skipped and are never executed.
- Persists customers, conversations, messages, agent runs, model iterations, parent tool calls, one-based child retry attempts, bookings, booking events, escalations, and dead-letter events in PostgreSQL.
- Exposes a small JSON demo API. Conversation creation returns an opaque, conversation-scoped bearer capability once; sending messages and reading traces require it, and only its SHA-256 digest is stored.
- Enforces a persisted 50,000-token hard cap per conversation by atomically reserving a conservative allowance—including the provider's worst-case retry count—before every model request and settling aggregate usage afterward.
- Executes `escalate_to_human` as a durable hand-off. Provider and bounded-loop failures return a safe customer message, create an escalation, and retain a dead-letter event for inspection or replay.
- Bounds each turn to 8 model iterations and 25 seconds by default. Retryable tool failures get at most 3 attempts, each with a 5-second timeout and capped exponential backoff; OpenRouter requests separately get at most 3 attempts within one provider deadline.

## Quick start

You need Docker with Compose. The default Compose profile sets `DEMO_MODE=true` and `LLM_PROVIDER=fake`, so it does not need an LLM API key.

```sh
git clone https://github.com/reinh2/kontor.git
cd kontor
docker compose up --build
```

The service listens on `http://localhost:8080`; [the health endpoint](http://localhost:8080/healthz) returns JSON when startup is complete. Its Stage 1 endpoints are under `/api/v1/demo`, and `/readyz` exposes database readiness. `POST /api/v1/demo/conversations` returns a `capability_token` only in its creation response. Pass it as `Authorization: Bearer <capability_token>` when sending messages or reading that conversation's run traces.

To exercise real tool-calling behavior, copy [`.env.example`](.env.example), set `LLM_PROVIDER=openrouter`, `OPENROUTER_API_KEY`, and `OPENROUTER_MODEL`, then restart Compose. This switches the same bounded agent loop and server-side tool gateway from the deterministic adapter to OpenRouter; it does not bypass confirmation, capabilities, budgets, or scheduling checks.

## Architecture

```mermaid
flowchart LR
    Customer["Demo API client"] --> HTTP["HTTP channel"]
    HTTP --> App["Conversation service"]
    App --> Runner["Bounded agent runner"]
    Runner <--> Model["Deterministic demo<br/>or OpenRouter"]
    Runner --> Executor["Retrying tool executor"]
    Executor --> Gateway["Schema, capability,<br/>and confirmation gateway"]
    Gateway --> Schedule["Scheduling engine<br/>and repository"]
    Schedule --> DB[(PostgreSQL)]
    App --> DB
    Runner --> Trace["Trace and token budget"]
    Trace --> DB
    Exports["Static UX exports"] -. visualize the intended operator experience .-> Trace
```

The model can request actions, but it never owns identity, authorization, or the final scheduling decision. See [Engineering notes](docs/ENGINEERING.md) for the runtime path, persistence model, failure semantics, and test strategy.

## Design decisions

- **Scope one tenant without erasing tenancy.** Every business key retains `tenant_id`, but this build resolves one fixed tenant from configuration and intentionally has no tenant onboarding or tenant-management UI.
- **Keep the first schema small.** The Stage 1 migration contains only data needed now that also carries into planned Stages 2–3. Channel delivery, reminder/outbox, CRM, identity, billing, and operator-UI tables arrive with the stage that uses them instead of being pre-created.
- **Propose, then act.** A mutating tool first returns an exact summary. A later, unambiguous customer message authorizes only those frozen arguments.
- **Keep authority outside the prompt.** Tenant, customer profile, conversation, inbound-message identity, and capabilities are resolved from persisted server state rather than model-authored JSON; model-supplied customer details are never used for a booking.
- **Make the database the final arbiter.** Signed slot tokens improve the hand-off, but the booking transaction still locks and rechecks the schedule before inserting.
- **Consume complete model responses safely.** If a response contains several tool calls, the loop handles them sequentially in response order before asking the model again. A refusal or successful hand-off is terminal for that batch, so later calls are recorded as skipped rather than executed.
- **Bound autonomy and spend.** Iteration, time, output-token, provider-retry, tool-retry, and persisted conversation-token limits turn failure into a controlled escalation rather than an unbounded loop.
- **Record the parent action and its attempts.** Each model-emitted tool call has one `tool_executions` parent; retries are nested `tool_execution_attempts` numbered from 1, matching the expandable trace design.
- **Fail visibly after saving input.** Provider and agent-loop failures leave the inbound message, safe fallback, escalation, failed run trace, and dead-letter event in durable storage.

## Limitations

Kontor is a demonstration project, not a production booking service.

- It runs as one fixed demo tenant (`Salon Nord`); there is no user identity system, tenant onboarding, or tenant-management UI. The demo API does enforce a generated bearer capability on each conversation after creation, but that is not a full authentication or account system.
- The HTTP surface is a JSON demo API. The web widget, Telegram channel, streaming updates, operator dashboard, trace viewer, and calendar shown above are designs, not wired application screens.
- `list_services`, `list_staff`, `find_slots`, `create_booking`, and `escalate_to_human` execute. Rescheduling, cancellation, and CRM contact/deal contracts remain allowlisted but return `NOT_IMPLEMENTED` for later stages.
- There is no HubSpot or CSV CRM adapter in this codebase yet, and no outbound email, SMS, or reminder sender. A customer row is stored in Kontor’s own database only.
- Calendar synchronization is currently a `noop`; PostgreSQL is the appointment source of truth for the demo.
- Explicit requests for a person and server-side tool refusals are enforced hand-offs. The instruction to escalate after three failed attempts to understand a request is currently a model policy, not an independently persisted server counter.
- The `2.9 s` dashboard median is illustrative fixture data. The backend records individual run durations but does not yet aggregate operational metrics.
- The default secret and database credentials are demo values and must not be used outside a local environment.

## Licence

No licence file has been added to this repository. Until the owner chooses one, the source is not offered under an open-source licence and normal copyright restrictions apply.
