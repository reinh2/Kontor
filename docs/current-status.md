# Current Project Status

- **Last updated:** 2026-07-23 by Kiro (AI context setup)
- **Branch / worktree:** `stage5-operator-calendar-commands` (ahead of `main` by 1 commit)
- **Current goal:** Complete Stage 6 — multi-tenancy and operator identity

## Stable working state

- Stages 1–5 are complete and merged to `main`: conversation-to-booking core, channels (widget, SSE, Telegram), design implementation, reminders/CRM, operator console with live data and calendar commands.
- CI passes on `main`: vet, race-detector tests, Docker builds, authenticated Compose smoke test.
- Both binaries (`cmd/api`, `cmd/worker`) build and run successfully.

## Recently completed

- Stage 5 operator calendar commands: admin-token-guarded create/reschedule/cancel endpoints with optimistic version checks, transactional reminder updates, and full SPA wiring (commit `d7005d0` on current branch).
- Operator live reads: dashboard aggregates, keyset-paginated run feed, full trace detail, tenant-timezone calendar.
- Migration `000005`: `cancelled` job state for retired reminders.

## In progress

- **Stage 6: multi-tenancy and identity** — partially implemented in working tree (not yet merged):
  - `internal/identity/`: operator authentication (bcrypt), sessions (SHA-256 digest), middleware, password handling. Tests pass.
  - `internal/tenants/`: tenant store with encrypted channel config, context helpers, mutations. Tests pass.
  - `internal/channels/onboardinghttp/`: tenant provisioning and operator management routes. Tests pass.
  - `internal/channels/tenanthttp/`: host-based tenant resolution middleware. **1 test failing** (`TestPublicTenantScopesEachHostToItsOwnTenant`).
  - `internal/channels/telegram/multitenant.go`: multi-tenant webhook wiring.
  - `internal/channels/demohttp/multitenant.go`: multi-tenant demo routes.
  - `internal/channels/operatorhttp/multitenant.go`: multi-tenant operator store.
  - Migration `000006_stage6_identity_tenants.sql`: operators, sessions, tenant_channels tables.
  - `cmd/api/main.go`: updated to Stage 6 HTTP handler with identity middleware.
  - Several new files have unformatted Go code (not yet `gofmt`-ed).

## Next actions

1. Fix the failing `tenanthttp` middleware test (tenant context propagation issue).
2. Format new Stage 6 files (`make fmt`).
3. Complete multi-tenant Telegram webhook resolution.
4. Wire operator login/logout through the onboarding handler end-to-end.
5. Update CI smoke test for tenant-scoped routes.
6. Merge Stage 6 to `main`.

## Blockers and open questions

- `TestPublicTenantScopesEachHostToItsOwnTenant` fails — tenant not found in request context after middleware runs. Root cause not yet diagnosed.
- Stage 6 scope decision: should tenant onboarding be self-service or admin-provisioned only for initial launch?

## Verification status

| Check | Result | Command / evidence | Last run |
|---|---|---|---|
| Format | Unformatted files exist (Stage 6 WIP) | `gofmt -l` | 2026-07-23 |
| Static analysis | pass | `go vet ./...` | 2026-07-23 |
| Tests | 1 failure (`tenanthttp`) | `go test ./...` | 2026-07-23 |
| Build (API) | pass | `go build ./cmd/api` | 2026-07-23 |
| Build (Worker) | pass | `go build ./cmd/worker` | 2026-07-23 |
| E2E | not run locally | CI Compose smoke (runs on push) | — |

## Changed areas requiring attention

- `internal/channels/tenanthttp/` — new middleware with failing test.
- `cmd/api/main.go` — Stage 6 handler wiring; route structure changed.
- `deploy/nginx/default.conf` — updated for tenant provisioning body size.
- `compose.yaml` — new env vars for multi-tenancy and identity.

## Handoff notes

- The working tree is on branch `stage5-operator-calendar-commands` but contains Stage 6 work (branch name is stale).
- `main` branch (`f1f3f83`) is the last stable point with all tests passing.
- All Stage 6 additions are uncommitted or in new untracked files.
