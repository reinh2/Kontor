# Claude Code project instructions

@AGENTS.md
@docs/product.md
@docs/architecture.md
@docs/coding-standards.md
@docs/decisions.md
@docs/current-status.md

## Claude-specific workflow

- Start by reading `docs/current-status.md` to understand what is stable and what is in progress.
- Use plan mode for architecture changes, broad refactors, migrations, or unclear multi-step work.
- Ask a question only when missing information materially changes the implementation and cannot be inferred from the repository.
- Prefer editing existing files over creating parallel implementations.
- Keep context efficient: inspect targeted files first, then expand only when evidence requires it.
- Before finishing, review the diff for scope creep, accidental secret exposure, missing tests, and stale documentation.

## Project-specific rules

- This is a Go 1.25 project using only stdlib + pgx + jsonschema. Do not introduce new dependencies without justification.
- Migrations are forward-only SQL in `db/migrations/`. Never edit an applied migration.
- The two-phase confirmation invariant (propose → confirm) must hold for all booking mutations. Do not bypass it.
- Use `log/slog` for logging. Inject the logger via constructors.
- Integration tests check for `TEST_DATABASE_URL`; unit tests must not require a database.
- Frontend (widget + operator console) is embedded via `go:embed`. No npm/webpack/Node.js in the build.
