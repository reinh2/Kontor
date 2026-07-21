# Kontor engineering notes

This document describes the code that exists in the repository today. The Stage 1 runtime is a conversation-to-booking backend; the screens in `design/` and `docs/img/` are static product-design exports, not a web client.

## Runtime boundary

The implemented path is:

1. Create a demo customer and conversation.
2. Save the inbound message before doing model or tool work.
3. Run a bounded model/tool loop using either the deterministic demo adapter or OpenRouter Chat Completions.
4. Resolve trusted tenant, customer, conversation, message, and capability data from PostgreSQL for every tool execution.
5. List services and staff, calculate slots, and mint a short-lived slot token scoped to the tenant and conversation.
6. Persist a frozen booking proposal. Only a later, unambiguous customer message can authorize it.
7. Recheck the slot under a database lock and create the booking and its first event atomically.
8. Persist the assistant reply and close the agent run, or create an escalation record for bounded-run failures.

The tool contract also names rescheduling, cancellation, CRM contact/deal, and human escalation. The gateway deliberately returns `NOT_IMPLEMENTED` for those calls in Stage 1. There are no CRM, notification, external-calendar, Telegram, or browser-client adapters in this repository.

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
7. Executes every returned tool call sequentially in response order, appending every result before the next model request.
8. Stops when the assistant returns no tool calls, or when an iteration, time, or token limit is reached.

Sequential execution is intentional even though OpenRouter is told that parallel tool calls are permitted: it produces deterministic traces and prevents sibling writes from racing through the same turn.

## Tool boundary and authorization

The gateway compiles the tool schemas at startup and rejects unknown tools, malformed JSON, extra properties, injected identity fields, missing capabilities, and invalid formats before dispatch.

Model arguments never carry trusted ownership. `agenttools.Executor` looks up the run and its conversation to construct `tools.TrustedContext`; the gateway then uses that context for capability checks and scope validation. Slot tokens are HMAC-signed and bind tenant, conversation, service, staff, start/end time, timezone, and expiration.

### Two-phase confirmation

Creating a booking without `confirmation_id` stores a proposal whose canonical arguments and hash exclude the confirmation ID. The proposal is bound to the customer, conversation, originating message, exact tool, and exact arguments.

The application recognizes only whole-message consent such as `yes`, `confirm`, or `book it`. It does not treat a message that also changes the requested action as authorization. The authorization must come from a later inbound message; the application injects the frozen action back into the model context, and the gateway verifies and consumes it when the tool is called again.

## Availability and booking consistency

The pure scheduling engine works in each staff member’s IANA time zone. It merges recurring working windows, subtracts recurring breaks, expands service and existing-booking buffers, uses half-open intervals, and produces a stable 15-minute grid. Tests cover both Europe/Berlin daylight-saving transitions, including the repeated autumn hour.

Slot search is advisory: the returned token is not a hold. `CreateBooking` starts a serializable transaction, reserves the idempotency key, materializes and locks the `(tenant, staff, local date)` schedule row, reloads busy state, and runs the same availability test again. A PostgreSQL GiST exclusion constraint on the buffered occupied range is the final double-booking guard.

Transaction serialization/deadlock failures are retried up to 3 times with short backoff. A repeated idempotency key with identical owner-bound arguments replays the stored booking; different arguments produce an idempotency conflict.

## Persistence and trace shape

The single forward migration carries `tenant_id` through business primary and foreign keys even though the application exposes one fixed tenant. This keeps tenant isolation explicit without claiming that tenant onboarding or runtime multi-tenancy exists.

Agent observability is stored as a hierarchy:

```text
agent_run
└── agent_iteration
    └── tool_execution
        └── tool_execution_attempt (1..N)
```

Run rows capture status, provider, model, token totals, duration, and a sanitized failure. Tool executions retain the model arguments, normalized result, duration, and child attempts. `GET /api/v1/demo/runs/{runID}` returns this persisted shape. There is no dashboard aggregation query yet.

## Defaults and hard limits

| Setting | Default | Purpose |
| --- | ---: | --- |
| Agent iterations | 8 | Maximum model calls in one turn |
| Whole-turn timeout | 25 s | Deadline across model and tool work |
| Tool attempts | 3 | Maximum for retryable failures |
| Per-attempt tool timeout | 5 s | Deadline around one gateway call |
| Conversation token budget | 50,000 | Persistent hard cap, including concurrent reservations |
| Maximum model output | 800 tokens | Per-completion allowance |
| OpenRouter response body | 4 MiB | Read limit before decoding |
| Slot-token lifetime | 5 min | Limits reuse of an offered slot |
| Confirmation lifetime | 10 min ceiling | Also capped by the associated slot-token expiry |
| Slot-search range | 31 days | Gateway and engine bound |

All application defaults can be inspected in [`.env.example`](../.env.example). Demo credentials and `SLOT_TOKEN_SECRET` are local-only values.

## HTTP API

The Stage 1 handler exposes:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | Process liveness |
| `GET` | `/readyz` | PostgreSQL readiness |
| `POST` | `/api/v1/demo/conversations` | Create a demo customer and conversation |
| `POST` | `/api/v1/demo/conversations/{conversationID}/messages` | Persist a customer message and run one agent turn |
| `GET` | `/api/v1/demo/runs/{runID}` | Read the persisted run and nested tool trace |

Request JSON rejects unknown fields and bodies larger than 16 KiB. Customer messages are limited separately by the application service (4,000 bytes by default). Errors use `application/problem+json`.

## Model adapters

The zero-key adapter is deterministic and network-free. It drives the next-Thursday-evening haircut path, deliberately emits two discovery tools in one model response, proposes a booking from a signed slot, waits for confirmation, and then repeats the exact call with server authorization.

The OpenRouter adapter uses non-streaming Chat Completions, supports tool calls and multiple calls per response, requests provider parallel-tool-call support, applies a request timeout, limits response size, and sanitizes provider errors. Provider selection and credentials come from application configuration, not from the adapter itself.

## Tests

At the time this document was written, the repository contains **47 named Go test functions across 12 test files**. The default suite covers the agent bounds, concurrent token reservations, schema and identity rejection, signed token scope, confirmation binding, OpenRouter serialization/errors, timezone and DST behavior, and scheduling adapter behavior.

Two PostgreSQL repository tests exercise idempotent booking, trace nesting, and concurrent booking contention when `TEST_DATABASE_URL` is set; otherwise they skip. Run the suites with:

```sh
make test
make test-race
TEST_DATABASE_URL='postgres://…' make test-integration
```

The integration target adds the race detector. The repository currently has no end-to-end browser tests because there is no browser runtime.
