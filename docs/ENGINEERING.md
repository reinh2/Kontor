# Kontor engineering notes

This document describes the code that exists in the repository today. The Stage 1 runtime is a conversation-to-booking backend; the screens in `design/` and `docs/img/` are static product-design exports, not a web client.

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

The tool contract also names rescheduling, cancellation, and CRM contact/deal operations. The gateway deliberately returns `NOT_IMPLEMENTED` for those calls in Stage 1; `escalate_to_human` is implemented and durably marks the conversation for human follow-up. The contract additionally includes `respond_to_customer`, a runner-local terminal control call: every customer-facing reply must arrive through it, so the reply's disposition (`complete` or `clarification_needed`) is structured data the server can act on rather than prose. There are no CRM, notification, external-calendar, Telegram, or browser-client adapters in this repository.

## Stage scope decision

The schema retains `tenant_id` in business primary and foreign keys, but the executable is deliberately scoped to one fixed tenant resolved from configuration. There is no tenant-selection path, provisioning API, onboarding flow, or tenant-management UI in this build.

The Stage 1 forward migration is limited to data used now that also carries into planned Stages 2–3: catalogue and schedule data, customers and conversations, bookings, confirmation proposals, agent traces, escalations, dead letters, locks, and idempotency records. Channel delivery and reminder/outbox tables will arrive with their implementing stages; the schema does not speculate with later CRM, billing, identity, tenant-administration, or dashboard-projection tables.

## Package map

| Package | Responsibility |
| --- | --- |
| `internal/app` | Conversation lifecycle, explicit-consent hand-off, agent-run lifecycle, and safe fallback messages |
| `internal/agent` | Provider-neutral bounded model/tool loop, token reservation, and trace events |
| `internal/llm` | Deterministic demo adapter, test fake, normalized message types, and OpenRouter adapter |
| `internal/tools` | Exact JSON Schema allowlist, capability checks, signed slot tokens, confirmation binding, and stable result envelopes |
| `internal/agenttools` | Trusted-context lookup, per-attempt timeout, retries, and backoff |
| `internal/scheduling` | Pure availability engine, PostgreSQL repository, and tools backend adapter |
| `internal/conversations` | Customers, conversations, messages, explicit-consent parsing, and persistent token accounting |
| `internal/confirmations` | PostgreSQL-backed proposal, authorization, verification, and consumption state machine |
| `internal/agenttrace` | Agent run, model iteration, tool execution, and nested-attempt persistence and reads |
| `internal/agentbudget` | Adapter from the runner’s reservation interface to atomic PostgreSQL token accounting |
| `internal/channels/demohttp` | Stage 1 JSON endpoints and health/readiness checks |
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

The deliberately small Stage 1 forward migration carries `tenant_id` through business primary and foreign keys even though the application exposes one fixed tenant. This keeps tenant isolation explicit without claiming that tenant onboarding or runtime multi-tenancy exists; later Stage 2–3 migrations will add only the tables their runtime work needs.

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

Environment-configurable defaults can be inspected in [`.env.example`](../.env.example). The OpenRouter retry and scheduling-window values above are currently code-level safety defaults. Demo credentials and `SLOT_TOKEN_SECRET` are local-only values.

## HTTP API

The Stage 1 handler exposes:

| Method | Path | Authorization | Purpose |
| --- | --- | --- | --- |
| `GET` | `/healthz` | None | Process liveness |
| `GET` | `/readyz` | None | PostgreSQL readiness |
| `POST` | `/api/v1/demo/conversations` | None | Create a demo customer and conversation; return its bearer capability once |
| `POST` | `/api/v1/demo/conversations/{conversationID}/messages` | Conversation bearer | Persist a customer message and run one agent turn |
| `GET` | `/api/v1/demo/runs/{runID}` | Owning conversation bearer | Read the persisted run and nested tool trace |

The creation response contains an opaque `capability_token` and is marked `Cache-Control: no-store`. Only a SHA-256 digest is persisted; the raw value cannot be recovered after that response. Supply it as `Authorization: Bearer <capability_token>` to send messages or read a trace, and a token for one conversation cannot authorize another. This is a narrowly scoped demo capability, not a user identity or tenant authentication system.

Request JSON rejects unknown fields and bodies larger than 16 KiB. Customer messages are limited separately by the application service (4,000 bytes by default). Errors use `application/problem+json`.

## Model adapters

The zero-key adapter is deterministic and network-free. It drives the next-Thursday-evening haircut path, deliberately emits two discovery tools in one model response, proposes a booking from a signed slot, waits for confirmation, and then repeats the exact call with server authorization.

The OpenRouter adapter is wired into the Stage 1 bootstrap, rather than deferred to a later product stage. It uses non-streaming Chat Completions, sends the exact JSON Schema tool definitions, supports multiple calls per response, requests provider parallel-tool-call support, applies a request timeout, limits response size, and sanitizes provider errors. It retries transient transport failures and 408/429/500/502/503/504 responses—including OpenRouter's non-streaming HTTP-200 response shape with an embedded provider error—up to three total attempts with capped exponential jitter, honoring `Retry-After`; all attempts share the adapter deadline. Reported usage across attempts is accumulated for conversation-budget settlement. Provider selection and credentials come from application configuration, not from the adapter itself.

## Tests

The default suite covers agent bounds, multi-call ordering, nested one-based attempts, concurrent token reservations, schema and identity rejection, signed token scope, confirmation binding, OpenRouter serialization/retries/errors, timezone and DST behavior, scheduling consistency, bearer-capability isolation, durable escalation, and provider-failure dead letters.

PostgreSQL-backed tests exercise the complete save/propose/confirm/book flow, idempotent booking, trace nesting, conversation serialization, capability isolation, durable failures, and concurrent booking contention when `TEST_DATABASE_URL` is set; otherwise they skip. Run the suites with:

```sh
make test
make test-race
TEST_DATABASE_URL='postgres://…' make test-integration
```

The integration target adds the race detector. The repository currently has no end-to-end browser tests because there is no browser runtime.
