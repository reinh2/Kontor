# AGENTS.md

## Read before working

1. `docs/current-status.md` — what is in progress and what is broken
2. `docs/product.md` — scope and non-goals
3. `docs/architecture.md` — module boundaries and invariants
4. `docs/coding-standards.md` — commands, patterns, and definition of done
5. `docs/decisions.md` — durable architectural choices
6. The source files and tests directly related to the task

## Operating rules

- Inspect the existing implementation before proposing a new pattern.
- For multi-file or risky work, state a short plan before editing.
- Prefer the smallest coherent change that satisfies the request.
- Preserve existing public behavior unless the task explicitly changes it.
- Do not add, remove, or upgrade dependencies without explaining why.
- Never read, print, commit, or copy secrets from `.env`, credential files, or production data.
- Validate untrusted input at system boundaries.
- Follow existing error handling (`fmt.Errorf` wrapping), logging (`slog`), naming, and module boundaries.
- Do not silence failing checks, weaken types, or delete tests to make a task pass.
- Add or update tests when behavior changes or a bug is fixed.
- Run verification commands before declaring completion.
- Update `docs/current-status.md` after meaningful work.
- Record durable architectural decisions in `docs/decisions.md`.

## Verified project commands

```bash
# Format
make fmt

# Static analysis
go vet ./...

# Unit tests (no DB required)
go test ./...

# Tests with race detector
make test-race

# Integration tests (requires running PostgreSQL)
TEST_DATABASE_URL='postgres://kontor:kontor@127.0.0.1:5433/kontor?sslmode=disable' make test-integration

# Build
go build ./cmd/api && go build ./cmd/worker

# Dev environment (Docker required)
make up

# Stop dev environment
make down
```

## Key invariants (never break these)

- A booking mutation requires prior explicit customer confirmation bound to exact proposed facts.
- Authorization comes from capability tokens (customers) or session tokens (operators), never from LLM output.
- Booking consistency is enforced by PostgreSQL serializable transactions, not application logic alone.
- Agent budgets (iterations, time, tokens) must produce escalation on exhaustion, not unbounded loops.
- Committed turns are stored before SSE delivery; replay from `Last-Event-ID` must have no gaps.

## Definition of done

- Requested behavior is implemented.
- `go vet ./...` passes.
- `go test ./...` passes (excluding documented WIP failures).
- New behavior has appropriate tests.
- Failure and security paths were considered.
- `docs/current-status.md` is updated.
- Final report names executed checks and any checks that could not be run.
