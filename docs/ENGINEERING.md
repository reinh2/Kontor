# Kontor engineering notes

This document describes the code that exists in the repository today. The runtime is a conversation-to-booking backend that Stage 2 fronts with channel delivery — an embeddable chat widget, a durable SSE stream, and a Telegram webhook. The operator screens in `design/` and `docs/img/` are static product-design exports, not a wired application.

## Runtime boundary

The implemented path is:

1. Create a demo customer and conversation.
2. Save the inbound message before doing model or tool work.
3. Run a bounded model/tool loop using either the deterministic demo adapter or the real OpenRouter Chat Completions adapter, including bounded retries for transient provider failures.
4. Resolve trusted tenant, customer, conversation, message, and capability data from PostgreSQL for every tool execution.
5. List services and staff, calculate slots, and mint a short-lived slot token scoped to the tenant and conversation.
6. Persist a frozen booking proposal. Only a later, unambiguous customer message can authorize it.
7. Recheck the slot under a database lock and create the booking and its first event atomically.
8. Persist the assistant reply and close the agent run, execute a requested `escalate_to_human` hand-off, or write a safe fallback plus escalation and dead-letter event for provider or bounded-loop failures.

The tool contract also names rescheduling, cancellation, and CRM contact/deal operations. The gateway deliberately returns `NOT_IMPLEMENTED` for those calls in Stage 1; `escalate_to_human` is implemented and durably marks the conversation for human follow-up. The contract additionally includes `respond_to_customer`, a runner-local terminal control call: every customer-facing reply must arrive through it, so the reply's disposition (`complete` or `clarification_needed`) is structured data the server can act on rather than prose. Stage 2 adds channel delivery around this same core — an embeddable browser widget, a durable SSE event stream, and a Telegram webhook — but there are still no CRM, notification, or external-calendar adapters in this repository.

## Stage scope decision

The schema retains `tenant_id` in business primary and foreign keys, but the executable is deliberately scoped to one fixed tenant resolved from configuration. There is no tenant-selection path, provisioning API, onboarding flow, or tenant-management UI in this build.

The Stage 1 forward migration was limited to data used then that also carries forward: catalogue and schedule data, customers and conversations, bookings, confirmation proposals, agent traces, escalations, dead letters, locks, and idempotency records. Stage 2 added a second forward migration with exactly the two channel-delivery tables its runtime needs — a durable per-conversation event stream and a Telegram update-dedupe table. Reminder/outbox tables and later CRM, billing, identity, tenant-administration, and dashboard-projection tables still wait for their implementing stages; the schema does not speculate ahead of the code.

## Package map

| Package | Responsibility |
| --- | --- |
| `internal/app` | Conversation lifecycle, explicit-consent hand-off, agent-run lifecycle, and safe fallback messages |
| `internal/agent` | Provider-neutral bounded model/tool loop, token reservation, and trace events |
| `internal/llm` | Deterministic demo adapter, test fake, normalized message types, and OpenRouter adapter |
| `internal/tools` | Exact JSON Schema allowlist, capability checks, signed slot tokens, confirmation binding, and stable result envelopes |
| `internal/agenttools` | Trusted-context lookup, per-attempt timeout, retries, and backoff |
| `internal/scheduling` | Pure availability engine, PostgreSQL repository, and tools backend adapter |
| `internal/conversations` | Customers, conversations, channel-conversation binding, messages, explicit-consent parsing, durable turn-event reads, and persistent token accounting |
| `internal/confirmations` | PostgreSQL-backed proposal, authorization, verification, and consumption state machine |
| `internal/agenttrace` | Agent run, model iteration, tool execution, and nested-attempt persistence and reads |
| `internal/agentbudget` | Adapter from the runner’s reservation interface to atomic PostgreSQL token accounting |
| `internal/channels/demohttp` | Demo JSON endpoints, the durable SSE event stream, embedded widget assets, and health/readiness checks |
| `internal/channels/telegram` | Telegram Bot API webhook with a verified secret and durable update dedupe, plus a bounded retrying sender |
| `internal/platform/httpx` | Channel-edge middleware: CORS for the widget and a per-client-IP token-bucket rate limiter |
| `web/widget` | The embeddable single-script chat widget and its demo host page, embedded into the API binary |
| `internal/bootstrap` | Concrete dependency graph for the application binary |
| `internal/platform` | Configuration, database/migration, ID, and logging primitives |
| `db/migrations` | Embedded, forward-only PostgreSQL schema |

## Agent loop

`agent.Runner` treats the model as an untrusted planner. On every iteration it:

1. Clones the current message history and exact tool definitions.
2. Conservatively estimates the request plus maximum response size.
3. Reserves that amount against the conversation’s hard token budget before calling the provider.
4. Settles the reservation against provider-reported usage; a failed or usage-less call keeps the full reservation charged.
5. Records the model iteration.
6. Validates the returned assistant role and normalizes missing tool-call IDs.
7. Handles every returned tool call sequentially in response order, appending every result before the next model request. A refusal or successful human hand-off terminates execution of that batch; later siblings receive persisted `SKIPPED_AFTER_HANDOFF` results and cannot mutate state.
8. Ends the turn only through the mandatory `respond_to_customer` terminal call, or when an iteration, time, or token limit is reached. The terminal call must be the only call in its response and carry no separate assistant text; unstructured terminal text or a mixed batch is a protocol violation that fails the turn instead of reaching the customer.

One model response may contain multiple tool calls. The runner handles that case explicitly: it processes every call sequentially in the order returned, persists its result, appends all results to history, and only then makes the next model request. Sequential execution is intentional even though OpenRouter is told that parallel tool calls are permitted; it produces deterministic traces and prevents sibling writes from racing through the same turn. A refused tool or successful `escalate_to_human` is terminal: remaining siblings are traced as refused with zero attempts and are not dispatched.

The token cap belongs to the conversation rather than to an individual turn. `token_budget`, `tokens_used`, and `tokens_reserved` are persisted on the conversation row. Before a provider call, an atomic update reserves a conservative request-plus-maximum-response estimate for the provider's worst-case retry count only if `tokens_used + tokens_reserved + estimate` remains within the cap. Settlement moves aggregate reported usage into `tokens_used`; a failed or usage-less provider call is charged the full reservation. Concurrent turns and internal OpenRouter retries therefore cannot collectively spend past the hard cap.

## Tool boundary and authorization

The gateway compiles the tool schemas at startup and rejects unknown tools, malformed JSON, extra properties, injected identity fields, missing capabilities, and invalid formats before dispatch.

Model arguments never carry trusted ownership or customer profile data. `agenttools.Executor` joins the run, conversation, and customer to construct `tools.TrustedContext`; the gateway then uses that persisted identity for capability checks, confirmation facts, and booking commands, overriding any model-authored customer object. Slot tokens are HMAC-signed and bind tenant, conversation, service, staff, start/end time, timezone, and expiration.

`escalate_to_human` passes through the same schema and capability boundary. Its backend creates an idempotent escalation associated with the run and source tool call and marks the conversation escalated. The runner treats that successful hand-off as terminal, skips later sibling calls, and closes the run with an escalated outcome.

### Structured replies and the clarification counter

`respond_to_customer` is validated inside the runner and never reaches the executor or gateway. Its validated disposition drives a server-owned counter on the conversation row: a `complete` reply or a durable booking resets `consecutive_clarification_failures` to zero, while each `clarification_needed` reply increments it in the same transaction that persists the reply. The third consecutive clarification outcome is an unconditional hand-off: the server replaces the model's question with a fixed hand-off message, records an `understanding_failed` escalation, and marks the conversation escalated — a database constraint only permits the counter to reach three on an escalated conversation. The model cannot influence the count except through the structured disposition itself.

Once a conversation is escalated, later inbound messages are saved and acknowledged with a fixed reply but create no agent run at all: no model call, no tool access, and no trace rows. Only the pre-hand-off runs appear in the conversation's automation history.

### Two-phase confirmation

Creating a booking without `confirmation_id` stores a proposal whose canonical arguments and hash exclude the confirmation ID. The proposal is bound to the customer, conversation, originating message, exact tool, and exact arguments.

The application recognizes only whole-message consent such as `yes`, `confirm`, or `book it`. It does not treat a message that also changes the requested action as authorization. The authorization must come from a later inbound message; the application injects the frozen action back into the model context, and the gateway verifies and consumes it when the tool is called again.

## Availability and booking consistency

The pure scheduling engine works in each staff member’s IANA time zone. It merges recurring working windows, subtracts recurring breaks, expands service and existing-booking buffers, uses half-open intervals, and produces a stable 15-minute grid. Tests cover both Europe/Berlin daylight-saving transitions, including the repeated autumn hour.

The gateway rejects slot searches earlier than the 15-minute minimum lead time, later than the 365-day booking horizon, or wider than 31 days. It applies the same lead and horizon window before signing returned slots and again when consuming a slot token. The repository additionally compares the requested start with PostgreSQL's `clock_timestamp()` inside the booking transaction, preventing a request delayed around a boundary from creating an already-past booking.

Slot search is advisory: the returned token is not a hold. `CreateBooking` starts a serializable transaction, reserves the idempotency key, materializes and locks the `(tenant, staff, local date)` schedule row, reloads busy state, and runs the same availability test again. A PostgreSQL GiST exclusion constraint on the buffered occupied range is the final double-booking guard.

Transaction serialization/deadlock failures are retried up to 3 times with short backoff. A repeated idempotency key with identical owner-bound arguments replays the stored booking; different arguments produce an idempotency conflict.

## Persistence and trace shape

The deliberately small Stage 1 forward migration carries `tenant_id` through business primary and foreign keys even though the application exposes one fixed tenant. This keeps tenant isolation explicit without claiming that tenant onboarding or runtime multi-tenancy exists; the Stage 2 migration followed the same rule, adding only the two channel tables its runtime needs, and later migrations continue that way.

Agent observability is stored as a hierarchy:

```text
agent_run
└── agent_iteration
    └── tool_execution
        └── tool_execution_attempt (1..N)
```

Run rows capture status, provider, model, token totals, duration, and a sanitized failure. There is exactly one `tool_executions` parent row per model-emitted call. Each execution attempt is a child `tool_execution_attempts` row whose `attempt_no` starts at 1 and increases under that same parent, so a retry does not become a second sibling call. The runner-local `respond_to_customer` control call is traced as a parent with zero attempts, because it never invokes the executor. `GET /api/v1/demo/runs/{runID}` returns this nested shape, matching the expandable attempt treatment in `design/screens/Kontor Agent Trace.dc.html`. There is no dashboard aggregation query yet.

Inbound messages are saved before the agent starts. If the provider or bounded loop fails afterward, the service persists a safe assistant fallback, an escalation, the failed run status, and a pending `dead_letter_events` row with sanitized context for later inspection or replay. A policy-refused tool also creates a durable escalation; it does not disappear as a model-only message.

## Defaults and hard limits

| Setting | Default | Purpose |
| --- | ---: | --- |
| Agent iterations | 8 | Maximum model calls in one turn |
| Whole-turn timeout | 25 s | Deadline across model and tool work |
| Tool attempts | 3 | Maximum for retryable failures |
| Per-attempt tool timeout | 5 s | Deadline around one gateway call |
| OpenRouter attempts | 3 | Maximum requests for transient transport and 408/429/500/502/503/504 failures, including embedded provider errors returned inside HTTP 200 |
| OpenRouter deadline | turn remainder (25 s maximum by default) | One deadline shared by the initial request, retry waits, and retries |
| Conversation token budget | 50,000 | Persistent hard cap, including concurrent reservations |
| Consecutive clarifications | 3 | Server-forced `understanding_failed` hand-off on the third structured clarification outcome |
| Turn queue wait | 750 ms | Bound on in-process admission plus the per-conversation serialization wait before a typed overload |
| Maximum model output | 800 tokens | Per-completion allowance |
| OpenRouter response body | 4 MiB | Read limit before decoding |
| Minimum booking lead | 15 min | Enforced during search, token issue/consume, and booking |
| Maximum booking horizon | 365 days | Upper bound for offered and consumed appointments |
| Slot-token lifetime | 5 min | Limits reuse of an offered slot |
| Confirmation lifetime | 10 min ceiling | Also capped by the associated slot-token expiry |
| Slot-search range | 31 days | Gateway and engine bound |
| Graceful shutdown | 35 s | Validated to exceed the whole-turn timeout by at least 5 s |
| HTTP rate limit | 60/min, burst 20 | Per-client-IP token bucket in front of the admission queue |
| SSE poll interval | 1 s | How quickly a connected widget observes a new committed turn |
| SSE heartbeat | 15 s | Comment frame that keeps a quiet stream open through proxies |
| SSE stream lifetime | 10 min | A single stream then closes; the client resumes from its cursor |
| Telegram send attempts | 3 | Bounded retries for transient Bot API failures |

Environment-configurable defaults can be inspected in [`.env.example`](../.env.example). The OpenRouter retry and scheduling-window values above are currently code-level safety defaults. Demo credentials and `SLOT_TOKEN_SECRET` are local-only values.

## Channels

Stage 2 puts three channels in front of the same conversation service. None of them can bypass confirmation, capabilities, budgets, or scheduling checks — they are delivery surfaces, not new authority.

**Embeddable widget.** `web/widget/kontor.js` is embedded into the API binary and served at `GET /widget/v1/kontor.js`; a host page adds it with a single `<script>` tag. It builds its UI inside a closed shadow root so host-page CSS cannot leak in, keeps the conversation capability in `sessionStorage`, and renders the same confirm-before-book card the JSON API returns. `GET /widget/v1/demo` serves a minimal host page for trying it locally.

**Durable SSE stream.** Each committed turn writes one `conversation_events` row inside the same transaction as the reply it describes, so the stream can never expose an outcome that is later rolled back; the identity column is the SSE event id. `GET /api/v1/demo/conversations/{id}/events` authorizes the conversation capability, replays every event after `Last-Event-ID` (or a `last_event_id` query parameter for clients that cannot set headers), then follows the stream by polling the durable rows rather than holding a dedicated `LISTEN` connection. The handler emits periodic heartbeat comments, caps a single stream's lifetime so a drained server sheds connections predictably, and sets its own per-write deadline through `http.ResponseController` because the server's write timeout is disabled for these long-lived responses. The bundled widget consumes the stream with `fetch`, not `EventSource`, so the bearer capability travels as a header and never appears in a URL, and it reconnects with capped backoff from its last event id.

**Telegram webhook.** `POST /webhooks/v1/telegram` is mounted only when both `TELEGRAM_BOT_TOKEN` and `TELEGRAM_WEBHOOK_SECRET` are configured. It compares the `X-Telegram-Bot-Api-Secret-Token` header in constant time and answers an unverified caller with 404 without touching the store or the application. A verified update claims its `update_id` with an idempotent insert; a redelivery that conflicts is acknowledged with 200 and runs no second turn. First contact from a private chat binds that chat to one conversation through a unique `(tenant, channel, channel_ref)` index. The reply is delivered by a sender that retries transient Bot API failures with bounded exponential backoff and honors `Retry-After`, but treats a permanent 4xx as final. Because the update is durably recorded and the turn outcome is persisted, a delivery failure is logged rather than papered over by asking Telegram to redeliver the inbound update.

**Edge protection.** A per-client-IP token-bucket rate limiter (`60` requests per minute, burst `20` by default) sits in front of the bounded turn-admission queue. It keys on the first `X-Forwarded-For` hop because the container runs behind the bundled nginx proxy, and returns `429` with `Retry-After` and a `problem+json` body; liveness and readiness probes bypass it. A CORS layer lets the configured origin — or `*` for the zero-key demo — call the API from a browser. Because the demo authorizes with a bearer capability rather than cookies, credentials are never allowed, so the wildcard origin stays safe.

## HTTP API

The handler exposes:

| Method | Path | Authorization | Purpose |
| --- | --- | --- | --- |
| `GET` | `/healthz` | None | Process liveness |
| `GET` | `/readyz` | None | PostgreSQL readiness |
| `POST` | `/api/v1/demo/conversations` | None | Create a demo customer and conversation; return its bearer capability once |
| `POST` | `/api/v1/demo/conversations/{conversationID}/messages` | Conversation bearer | Persist a customer message and run one agent turn |
| `GET` | `/api/v1/demo/conversations/{conversationID}/events` | Owning conversation bearer | Replay and follow the durable turn-event stream over SSE |
| `GET` | `/api/v1/demo/runs/{runID}` | Owning conversation bearer | Read the persisted run and nested tool trace |
| `GET` | `/widget/v1/kontor.js` | None | Embeddable single-script chat widget |
| `GET` | `/widget/v1/demo` | None | Minimal host page for the widget |
| `POST` | `/webhooks/v1/telegram` | Telegram secret header | Telegram Bot API webhook; mounted only when the channel is configured |

The creation response contains an opaque `capability_token` and is marked `Cache-Control: no-store`. Only a SHA-256 digest is persisted; the raw value cannot be recovered after that response. Supply it as `Authorization: Bearer <capability_token>` to send messages or read a trace, and a token for one conversation cannot authorize another. This is a narrowly scoped demo capability, not a user identity or tenant authentication system.

Request JSON rejects unknown fields and bodies larger than 16 KiB. Customer messages are limited separately by the application service (4,000 bytes by default). Errors use `application/problem+json`.

## Model adapters

The zero-key adapter is deterministic and network-free. It drives the next-Thursday-evening haircut path, deliberately emits two discovery tools in one model response, proposes a booking from a signed slot, waits for confirmation, and then repeats the exact call with server authorization.

The OpenRouter adapter is wired into the Stage 1 bootstrap, rather than deferred to a later product stage. It uses non-streaming Chat Completions, sends the exact JSON Schema tool definitions, supports multiple calls per response, requests provider parallel-tool-call support, applies a request timeout, limits response size, and sanitizes provider errors. It retries transient transport failures and 408/429/500/502/503/504 responses—including OpenRouter's non-streaming HTTP-200 response shape with an embedded provider error—up to three total attempts with capped exponential jitter, honoring `Retry-After`; all attempts share the adapter deadline. Reported usage across attempts is accumulated for conversation-budget settlement. Provider selection and credentials come from application configuration, not from the adapter itself.

## Tests

The default suite covers agent bounds, multi-call ordering, nested one-based attempts, concurrent token reservations, schema and identity rejection, signed token scope, confirmation binding, OpenRouter serialization/retries/errors, timezone and DST behavior, scheduling consistency, bearer-capability isolation, durable escalation, and provider-failure dead letters. Channel tests cover the Telegram webhook's secret rejection without side effects, update deduplication, single-turn processing, and the sender's retry-versus-permanent policy against a fake Bot API, along with the SSE handler's capability check and `Last-Event-ID` replay and the CORS and rate-limiter middleware.

PostgreSQL-backed tests exercise the complete save/propose/confirm/book flow, the durable turn events it emits, idempotent booking, trace nesting, conversation serialization, capability isolation, durable failures, and concurrent booking contention when `TEST_DATABASE_URL` is set; otherwise they skip. Run the suites with:

```sh
make test
make test-race
TEST_DATABASE_URL='postgres://…' make test-integration
```

The integration target adds the race detector. The repository currently has no end-to-end browser tests because there is no browser runtime.
