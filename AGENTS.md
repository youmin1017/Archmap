# Archmap

Collaborative, versioned system-architecture modeling tool. Full spec: `docs/PROJECT_BRIEF.md`. Design: `docs/ARCHITECTURE.md`. Non-obvious choices: `docs/DECISIONS.md`.

Work milestone by milestone (M0..M6); stop and summarize after each. Check current official docs before using Echo v5, oapi-codegen, TanStack Start, shadcn, Magic UI, Iconify — never code from memory.

## Commands (go-task, `Taskfile.yml` — install with `scoop install task`)

| Task | What |
|---|---|
| `task db-up` / `task db-down` | PostgreSQL 18 via `deploy/compose/compose.yaml` (needs `.env`, copy `.env.example`) |
| `task migrate` / `task seed` | goose migrations / dev users (viewer, editor, maintainer) + sample model (main v1) |
| `task gen` | Regenerate Go server stubs + sqlc + TS client from `contracts/openapi.yaml` (generated code is committed) |
| `task gen-check` | `gen` + `git diff --exit-code` (CI drift check) |
| `task dev-api` / `task dev-web` | API (air, :8080) / web (Vite, :3000, proxies `/api`) |
| `task test` / `task lint` | all tests (integration tests need Docker; `go test -short` skips them) / vet + eslint + tsc + oxfmt check |

Auth is not built yet: with `AUTH_DEV_HEADER=true` (in `.env.example`) send `X-Dev-User: editor@archmap.local` (or viewer / maintainer) to act as a seeded user. The web dev server does this for you via `DEV_USER` (default viewer). Web unit tests: `pnpm --filter web test` (vitest).

Pre-commit hooks: **lefthook** (`lefthook.yml`; Go tool dep in `apps/api/go.mod`; install with `task hooks` once the repo is a git repo). Runs oxfmt + eslint on staged web files, gofmt + go vet on Go files.

Web formatter is **oxfmt** (`pnpm --filter web format`), config `apps/web/.oxfmtrc.json`. Dependencies are pinned to exact versions.

## Layout

`apps/api` (Go) · `apps/web` (TanStack Start) · `contracts/openapi.yaml` (source of truth) · `packages/api-client` (generated TS client, hey-api + Zod) · `deploy/compose` (postgres only).

## Go architecture rules

- Package by feature; layers `transport (Echo/generated handlers) → service → repository (sqlc)`. NOT MVC.
- Echo only in `internal/platform/httpx` and `transport` files. Services/repositories take `context.Context` and plain types, never import Echo.
- Echo v5: `github.com/labstack/echo/v5`, handlers `func(c *echo.Context) error`. Never write v4 style.
- Never hand-write DTOs duplicating generated types (`internal/gen/api`); map between domain and transport types.
- Errors: return `internal/platform/apperr` errors from services; `httpx` maps them to RFC 9457 problem+json (Echo's `ProblemDetailsHTTPErrorHandler`).
- Main is read-only through the API; every edit is a change in a changeset (draft). Roles are checked in services via `authctx.Actor` from the context.
- sqlc: one package per feature, queries in `db/queries/<feature>.sql`; change SQL/spec, then `task gen`.
- No global state; interfaces defined on the consumer side; table-driven service tests; testcontainers-go for repository/merge integration tests.
- All business rules in Go; web has UI validation only (no TanStack server functions for business logic).
- Model (structure) and Layout (positions) are strictly separate: dragging a node never creates a model change.
