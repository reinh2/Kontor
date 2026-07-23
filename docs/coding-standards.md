# Coding Standards

## Sources of truth

- Formatter: `gofmt` (standard Go formatter, no config file)
- Static analysis: `go vet ./...` (standard Go vet)
- Linter: `golangci-lint` v2, configured in `.golangci.yml`, enforced by the `lint` CI job
- Test framework: `testing` stdlib + `go test`
- CI workflow: `.github/workflows/ci.yml`

When this document conflicts with executable configuration, investigate and update the stale source.

## Verified commands

| Purpose | Command | Verified on | Notes |
|---|---|---|---|
| Format | `make fmt` or `gofmt -w $(find cmd internal db -name '*.go' -type f)` | 2026-07-23 | Standard gofmt |
| Static analysis | `go vet ./...` | 2026-07-24 | Pass |
| Lint | `make lint` or `golangci-lint run ./...` | 2026-07-24 | Pass (0 issues, v2.12.2) |
| Unit/integration tests | `make test` or `go test ./...` | 2026-07-23 | Integration tests need `TEST_DATABASE_URL` |
| Tests with race detector | `make test-race` or `go test -race ./...` | 2026-07-23 | CI always runs with `-race` |
| Integration tests | `make test-integration` | Not run locally | Requires `TEST_DATABASE_URL` env var |
| Build (API) | `go build ./cmd/api` | 2026-07-23 | Pass |
| Build (Worker) | `go build ./cmd/worker` | 2026-07-23 | Pass |
| Docker build | `docker build --build-arg TARGET=api .` | CI only | Multi-stage, distroless final image |
| Dev environment | `make up` or `docker compose up --build` | CI | Starts Postgres + API + Worker + nginx |
| E2E | Compose smoke test in CI only | CI | Authenticated booking flow; not a local command |

## General implementation rules

- Match existing architecture and naming before introducing a new abstraction.
- Keep functions and modules focused on one responsibility.
- Make dependencies explicit; pass them as constructor arguments (dependency injection in `internal/bootstrap/`).
- Prefer clear code over clever code.
- Remove dead code instead of commenting it out.
- Do not leave unexplained TODOs. Link to a stage or state the missing decision.
- Avoid unrelated formatting or refactoring in feature and bug-fix changes.
- Document non-obvious invariants (especially around confirmation and scheduling consistency).

## Naming and organization

- **Files and directories:** lowercase, underscore-separated where needed. Package per domain concept under `internal/`.
- **Types:** PascalCase exported, camelCase unexported. Interfaces named by behavior (e.g., `Notifier`, `CRM`).
- **Functions and variables:** camelCase. Exported constructors use `New*` prefix (e.g., `NewStore`, `NewLogNotifier`).
- **Constants:** PascalCase exported, camelCase unexported. Grouped in `const` blocks.
- **Tests:** `*_test.go` in same package. Integration tests use `_integration_test.go` suffix and check `TEST_DATABASE_URL`.
- **Imports:** stdlib first, then external, then internal. Grouped with blank lines.

## Types and interfaces

- Use interfaces at consumption site, not at definition site (standard Go idiom).
- Keep interfaces small (1-3 methods where possible).
- Validate runtime data at system boundaries (HTTP handlers, config loading).
- Avoid `interface{}` / `any`; use concrete types or constrained generics.

## Error handling and logging

- Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the chain.
- Return errors to callers; do not log-and-return (choose one).
- Use `log/slog` structured logging. Logger is injected via constructors.
- Log levels: `Info` for lifecycle events, `Warn` for recoverable failures, `Error` for unrecoverable failures that need attention.
- Never log credentials, tokens, session identifiers, capability values, or customer PII.
- Retries: bounded attempts with exponential backoff. Tool executor uses configurable max attempts. Job queue uses 30s base with 2^n and 1h cap.
- Idempotency: booking creation uses idempotency keys; job enqueue uses `ON CONFLICT DO NOTHING`.

## Security

- Treat all external input as untrusted (customer messages, webhook payloads, HTTP headers).
- Use parameterized queries via pgx (no string concatenation for SQL).
- Enforce authorization server-side: capability tokens for customers, session tokens for operators.
- Keep secrets outside the repository. Reference only `.env.example` variable names, never values.
- Do not expose internal errors, paths, stack traces, or infrastructure details in HTTP responses.
- Capability tokens: issue once, store only SHA-256 digest. Operator sessions: same pattern.
- Telegram: constant-time secret comparison.
- Tenant channel secrets: AES-GCM encrypted at rest.

## Testing

- A bug fix requires a regression test when practical.
- Test behavior and contracts, not private implementation details.
- Keep tests deterministic and isolated from production services.
- Integration tests require `TEST_DATABASE_URL`; they are skipped when the variable is absent.
- Use table-driven tests for multiple input variations.
- Mock only external boundaries (LLM provider, CRM, notifications). The fake LLM adapter enables deterministic agent testing without mocks.
- Test files live alongside production code in the same package.
- Coverage expectations: not formally defined; critical paths (scheduling, confirmations, agent budget) have comprehensive coverage.

## Database and migrations

- Migrations are forward-only SQL files in `db/migrations/` with sequential numeric prefixes.
- Embedded via `go:embed` in `db/migrations/embed.go` and applied at startup.
- Never edit an already-applied migration.
- Each migration corresponds to a stage and adds only what that stage's runtime needs.
- Use serializable transactions for booking mutations.
- Use `FOR UPDATE SKIP LOCKED` for job claim patterns.
- Idempotent inserts use `ON CONFLICT DO NOTHING`.
- Booking consistency relies on exclusion constraints and schedule locks, not application-level checks alone.

## API and UI contracts

- Customer API: JSON request/response. Bearer capability token in `Authorization` header.
- Operator API: JSON request/response. Session bearer token in `Authorization` header.
- SSE events: `text/event-stream` with `id` field for resumable replay.
- Widget: single `<script>` tag, rendered in closed shadow root, vanilla JS.
- Operator console: SPA using vendored React DS bundle, hash-based routing.
- Error responses: structured JSON with appropriate HTTP status codes; no stack traces or internal paths.

## Git and review hygiene

- Keep commits and diffs focused on one concern.
- Do not commit generated files unless the repository intentionally tracks them (e.g., `web/operator/ds-bundle.js`).
- Never overwrite unrelated local changes.
- Explain migrations, dependencies, breaking changes, security impact, and unverified checks in commit messages or PR descriptions.

## Definition of done

- Requested acceptance criteria are met.
- `go vet ./...` passes.
- `golangci-lint run ./...` passes (or `make check`).
- `go test ./...` passes (excluding known WIP failures).
- New behavior has appropriate tests.
- Failure and security paths were reviewed.
- Documentation reflects changed behavior or architecture.
- `docs/current-status.md` is updated.
- Any unexecuted check and the reason are explicitly reported.
