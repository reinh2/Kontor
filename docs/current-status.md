# Current Project Status

- **Last updated:** 2026-07-24
- **Branch / worktree:** `main`
- **Current goal:** Stage 7 — production hardening and launch

## Stable working state

- Stages 1–6 are complete: conversation-to-booking core, channels (widget, SSE, Telegram), design implementation, reminders/CRM, operator console with live data and calendar commands, and multi-tenancy + operator identity.
- All local checks are green: `gofmt -l` reports no unformatted files, `go vet ./...` passes, `go test ./...` passes, and `go test -race -count=1 ./...` passes.
- Both binaries (`cmd/api`, `cmd/worker`) build and run successfully.


## Recently completed

- **Hallucinated-service reply fix (2026-07-24).** A widget customer asking for
  "25 july colour on 09:00" received `I am sorry, "colour" is not a valid
  service. Please choose from the available services.` followed by an invented
  UUID — for a service the tenant does offer. The trace showed zero tool calls:
  the model answered from memory and the runner delivered the prose verbatim.
  Five defects on that path were fixed:
  - **Prose bypassed the terminal control call (root cause).** The runner now
    discards a response with no tool call, re-prompts with a protocol
    correction, and fails closed after two corrections. A turn that already
    produced a confirmation proposal is likewise re-prompted instead of allowed
    to keep acting. See ADR-013.
  - **`find_slots` exhausted the token budget.** One day search returned every
    slot with its ~750-byte signed token (~26 KB per call), so two or three
    searches consumed a whole conversation budget. The gateway now returns at
    most the 12 earliest slots and reports `truncated`.
  - **Schema rejections were unactionable.** `arguments do not match the tool's
    v1 JSON Schema` now names the failing path and keyword, and the offending
    property names for `additionalProperties`/`required` — never instance
    values — so the `fix_arguments` resolution can be acted on.
  - **The Gemini schema sanitizer hid its own constraints.** Stripping
    `pattern`, `format`, `minLength`, and `maxLength` left the model generating
    values it could not know were invalid (a timestamp-shaped
    `idempotency_key`). Those constraints now survive as prose in the node's
    `description`.
  - **Catalogue misses escalated to a human.** An unknown `service_id` /
    `staff_id` on the read-only `list_staff` and `find_slots` mapped to
    `NOT_FOUND_OR_NOT_OWNED`, which the executor treats as a refusal and the
    runner turns into a terminal hand-off. These carry no ownership, so a miss
    is now `INVALID_ARGUMENT` / `fix_arguments`. Mutating tools are unchanged.
  - **Retried calls were billed several times over.** A response whose
    successful attempt reported its usage was still charged the full worst-case
    reservation whenever any earlier attempt had failed, because
    `Response.UsageIncomplete` is sticky across the adapter's retry loop. One
    run reported 39 329 real tokens and was charged ~82 000 of a 100 000 budget,
    escalating as `budget_exhausted` before the customer could confirm. Charging
    now prices unaccounted attempts at their prompt, capped by the reservation.
    See ADR-014.
  - **The confirming turn made the model retype the slot token.** Enforcement of
    ADR-001 compared the model's re-sent arguments to the frozen ones, so a
    corrupted ~600-character `slot_token` killed a booking the customer had
    already approved (`slot token is invalid or has been tampered with`). A call
    carrying a `confirmation_id` that matches the conversation's live proposal
    now executes the server's frozen `ArgumentsJSON` directly. See ADR-015.
  - **Two limits were set below what one real booking costs.**
    `AGENT_MAX_OUTPUT_TOKENS=800` truncated the model response mid-`slot_token`
    (`finish_reason: length`), and `AGENT_CONVERSATION_TOKEN_BUDGET=50000` could
    not hold a two-turn booking once the conservative estimator's reservations
    are counted. Defaults are now 2500 and 400000 in `.env.example` and
    `compose.yaml`; the config ceiling and the `conversations_token_budget_check`
    constraint (migration `000008`) were raised to 2 000 000 to match.
  - **The model asked for confirmation in prose instead of proposing.** The
    system prompt said only that a mutation "requires the server's two-phase
    confirmation", which several models read as "ask the customer in text
    first". They replied `Please confirm: Colour with Nadia P. …` without
    calling `create_booking`, so no server proposal existed and the customer's
    "yes" could authorize nothing. The prompt now states that the proposal *is*
    the tool call and that asking for consent before it returns
    `confirmation_required` is forbidden.
  - **Models invented an `idempotency_key` the contract rejected.** Observed
    values included an email address and a timestamp carrying a `+` offset —
    three different models, three different pattern violations, each losing a
    booking after the customer had chosen a slot. The field is now optional and
    the gateway derives it from the verified action (tenant, conversation,
    customer, tool, and the slot claims), so it is identical for the same
    appointment and stable across the confirming turn.
  - Verified against the running Compose stack with `LLM_PROVIDER=openrouter`
    and `google/gemini-2.5-flash-lite`: the same message now runs
    `list_services -> list_staff -> find_slots`, quotes real slots, and asks for
    the missing contact instead of denying the service. A full booking completes
    end to end — proposal, customer confirmation, `bookings` row `confirmed`,
    and the `send_reminder` / `crm_upsert_contact` outbox jobs processed.
- **Gemini tool calling schema compatibility fix (2026-07-24).** The OpenRouter
  wire format now strips JSON Schema keywords unsupported by Google Gemini
  (`$schema`, `pattern`, `maxProperties`, `minLength`, `maxLength`) from tool
  parameter definitions before sending them to the model. Gemini silently fell
  back to text-only responses instead of emitting tool calls when these keys
  were present, causing the agent to respond with "The `list_services` tool is
  not available for me to use." Server-side validation in `gateway.go` continues
  to enforce the full Draft 2020-12 schemas with all constraints.
- **LLM booking flow bug fix (2026-07-23).** Fixed two production-impacting
  defects in the real-model booking path:
  - **Dead-letter persistence (SQL type error):** `jsonb_build_object` in
    `persistHandoff` used untyped parameters (`$5`, `$6`, `$7`), causing
    PostgreSQL SQLSTATE 42P08. Added explicit `::text` casts. The fallback
    transaction (escalation + dead-letter + SSE event) now commits atomically
    instead of rolling back and leaving `agent_runs` in `running` state.
  - **Token budget over-estimation:** `ConservativeTokenEstimator` treated raw
    bytes as tokens (1:1), inflating a typical 10-tool turn to ~33k tokens and
    exhausting a 50k budget in one turn. Changed to ceiling(bytes÷3) — still
    conservatively above real BPE average (~3.5–4 bytes/token) while reducing
    reservations ~3×. Provider failures still charge full reservation (hard cap
    preserved). See ADR-012.
  - Regression tests cover: realistic estimation bounds, provider-failure full
    charge, SSE event `pending_confirmation_active: false`, and source-level
    SQL type-cast verification.
- **Recoverable model tool-name errors (2026-07-23).** A model selecting an
  unregistered tool (for example, `list_available_slots` instead of the
  supplied `find_slots`) now receives a normal tool error and can ask a
  clarification or retry with an allowed tool. No server action is attempted.
  Gateway policy and ownership refusals remain terminal human hand-offs.
- **Booking-context and slot-picker guardrails (2026-07-23).** The booking
  instructions now retain known customer facts, infer a missing year from the
  tenant's current local date, and require a half-open local-day range for
  `find_slots` so `date_from == date_to` cannot be generated. The embedded
  widget no longer guesses available slots by matching times in assistant
  prose, preventing false “Choose a time” controls from being shown.
- **OpenAI-compatible tool namespace normalization (2026-07-23).** The agent
  accepts `default_api.<tool>` only when its suffix exactly matches a
  server-registered tool (for example `default_api.list_services` becomes
  `list_services`). Other unknown names remain blocked; this prevents a model
  formatting variant from turning into a customer-facing invented outage.
- **Contactless widget creation migration (2026-07-23).** Migration `000007`
  removes the obsolete database contact check so a widget visitor can start a
  conversation without email or phone, matching the server-side contact
  capture flow. A booking still requires a literal email or E.164 phone from
  the authenticated customer's message.
- **Operator navigation and Claude Design integration.** The embedded operator
  console now exposes Overview, Runs, Calendar, Inbox, Analytics, and Settings
  with keyboard-native, labelled navigation and the existing responsive mobile
  sidebar replacement. `#/` and `#/dashboard` remain Dashboard-compatible
  aliases while `#/overview` is canonical; existing Runs and Calendar routes
  are unchanged. Inbox reads tenant-scoped escalated runs, Analytics reads the
  live dashboard aggregate, and Settings reads the session identity with its
  owner/staff boundary. All three screens implement loading, retryable error,
  and applicable empty states. The received Claude Design HTML package is
  documented and mapped at [`docs/design-integration.md`](design-integration.md);
  its adopted UI is implemented in `web/operator/` and `web/widget/` without a
  Node/npm build step or dependency. Embedded-asset regression tests cover the
  routes and safety-critical widget markers.
- **Direct OpenAI provider.** Added `LLM_PROVIDER=openai` with `OPENAI_API_KEY`, `OPENAI_MODEL`, and optional `OPENAI_BASE_URL`. It uses the existing bounded Chat Completions tool-calling path without OpenRouter attribution headers; fake and OpenRouter modes remain supported. Unit coverage verifies direct authentication and headers, and Compose now passes the settings to both API and worker.
- **Real-model booking-flow guardrails.** The system prompt and contracts now
  direct service → staff → slot discovery, contact collection, and safe
  `fix_arguments` clarification. A contact-less conversation can start, but
  only an email or E.164 phone literally present in the authenticated user's
  saved message is added to its profile; booking identity remains server-owned.
  New non-consent, clarification, and failed turns withdraw live proposals,
  and the SSE/widget snapshot explicitly clears stale confirmation cards.
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
4. Build and gate provider/model changes on the versioned multilingual real-model booking evaluation suite recorded in `ROADMAP.md`.

## Blockers and open questions

- **Model selection (resolved for now, 2026-07-24).** `LLM_PROVIDER=openai` with
  `OPENAI_MODEL=gpt-5.4`. Measured on one scripted three-turn booking:

  | Model | Bookings completed | Tokens | Failure mode |
  |---|---|---|---|
  | gpt-5.4 | 2 / 2 | ~50 000 | — |
  | gpt-5.5 | 1 / 2 | ~51 500 | asked for confirmation in prose (fixed since) |
  | gpt-5.4-mini | 0 / 3 | ~36 000 | loses the slot token between turns |
  | gpt-4.1-mini | 0 / 1 | ~82 000 | same, and burns the most tokens of any tier |
  | gpt-4.1 | 0 / 2 | — | does not reach a proposal |
  | gemini-2.5-flash-lite | 0 / 4 to booking, 2 / 4 to proposal | — | skips tools, invents arguments |

  `gpt-5.4` is the cheapest model that completes the flow, at the same token
  cost as `gpt-5.5`; the mini tiers are cheaper per token but do not finish, so
  they cost more per completed booking, not less. Every failure is a safe
  escalation, never a wrong action.
  **The `gpt-5.6` family cannot be used on this code path:** OpenAI rejects
  function tools for it on `/v1/chat/completions` unless the request targets
  `/v1/responses` or sets `reasoning_effort: "none"`, neither of which the
  adapter does. Supporting it would mean a Responses API adapter — worth
  considering, not required.
- A provider response that reports no usage at all still writes off the full
  worst-case reservation (ADR-012). That is the intended hard-cap behaviour, but
  with a model that returns occasional empty responses it remains the largest
  single consumer of a conversation budget. Worth revisiting if empty responses
  turn out to be frequent in production.

- Stage 7 scope decision: minimum production observability (metrics only vs. metrics + tracing + alerting) for the first launch.
- External calendar sync strategy (Google Calendar / Microsoft Graph) is still open.

## Verification status

| Check | Result | Command / evidence | Last run |
|---|---|---|---|
| Format | pass (no unformatted files) | `gofmt -l` | 2026-07-23 |
| Static analysis | pass | `go vet ./...` | 2026-07-23 |
| Tests | pass | `go test ./...` | 2026-07-23 |
| Tests (race) | pass | `go test -race ./...` | 2026-07-23 |
| Build (API) | pass | `go build ./cmd/api` | 2026-07-23 |
| Build (Worker) | pass | `go build ./cmd/worker` | 2026-07-23 |
| Embedded UI regression | pass | `go test ./web/operator ./web/widget`; `node --check` for both embedded scripts | 2026-07-23 |
| Integration | not run locally | needs `TEST_DATABASE_URL`; runs in CI | — |
| E2E | not run locally | CI Compose smoke (runs on push) | — |

## Changed areas requiring attention

- `internal/scheduling/repository.go` — customer reschedule/cancel now use the retry + `mapDatabaseError` wrapper (D-1).
- `internal/jobqueue/postgres.go` and `cmd/worker/main.go` — stale-claim reaper and shutdown-safe terminal writes (D-2).
- New integration tests under `internal/scheduling/` (`reschedule_conflict_integration_test.go`, `stale_claim_integration_test.go`) that require `TEST_DATABASE_URL`.
- `web/operator/index.html` and `web/widget/kontor.js` — Overview aliases,
  Inbox/Analytics/Settings, and adopted Claude Design UI components; the
  inventory and source provenance are in `docs/design-integration.md`.

## Handoff notes

- The audit fixes are code-complete and pass local `vet`/`test`/`-race`; the two new integration tests could not be executed locally (no PostgreSQL/Docker available) and are expected to run in CI.
