# Decisions (ADR log)

## ADR-001 Task runner: go-task, not make
`make` is not available on the Windows dev machine. `Taskfile.yml` (go-task) is the single entry point with the same task names as the brief (`db-up|db-down|migrate|gen|dev-api|dev-web|test|lint`). **Status: accepted** (user decision).

## ADR-002 Generated code is committed
Go stubs and the TS client are committed; fresh clones build without codegen. CI runs `task gen-check` (regenerate + `git diff --exit-code`). **Accepted** (user decision).

## ADR-003 Echo v5 + oapi-codegen v2 (stable)
Verified: oapi-codegen **v2.8.0** README lists `echo5-server` (Go 1.25+); generated code compiles against `github.com/labstack/echo/v5` v5.4.0 with `*echo.Context` handlers. Using `echo5-server` + `strict-server` + `embedded-spec`. The experimental V3 (`oapi-codegen-exp`) and std-http fallbacks are **not needed**. Import path verified as exactly `github.com/labstack/echo/v5`.

## ADR-004 Request validation: echo-v5-middleware v0.1.1
`github.com/oapi-codegen/echo-v5-middleware` v0.1.1 is tagged and works (tests cover 404/405 for `/api/*`). Server host validation is disabled (`DoNotValidateServers`); non-`/api/` paths (e.g. `/metrics`) are skipped.

## ADR-005 Error handling: Echo's built-in RFC 9457 handler
Echo v5.4.0 ships `echo.ProblemDetailsHTTPErrorHandler`; used instead of a custom handler. Content type `application/problem+json`. Typed domain errors will implement `echo.ProblemErrorer` in transport (M1+).

## ADR-006 Prometheus: echo-contrib/v5 `echoprometheus`
`github.com/labstack/echo-contrib` v4 line is retracted for Echo v5; `echo-contrib/v5` v5.0.1 provides `echoprometheus`. A per-server `prometheus.Registry` is injected (no global state, test-friendly). Trade-off: pulls echo-contrib's dependency set into go.mod; revisit with a small hand-written middleware if that becomes a burden.

## ADR-007 TS client: @hey-api/openapi-ts 0.99.0 (+ Zod)
Emits types, fetch SDK and Zod schemas; SDK `validator: "zod"` validates requests/responses at runtime. Chosen over openapi-typescript (types only) and orval. Exact version pinned. Output in `packages/api-client/src/gen`.

## ADR-008 PostgreSQL 18 volume path
The `postgres:18` image declares `VOLUME /var/lib/postgresql` (PGDATA is `/var/lib/postgresql/18/docker`). Compose mounts the named volume at `/var/lib/postgresql`, not `/var/lib/postgresql/data`. `uuidv7()` is native.

## ADR-009 Go dev tools
`oapi-codegen` is pinned as a `tool` in `apps/api/go.mod` (`go tool`). `goose`, `sqlc` and `air` run via `go run <pkg>@<exact version>` from the Taskfile: adding them as `tool` dependencies pulled very large dependency graphs into go.mod and made `go get`/`go mod tidy` hang. Versions: goose v3.29.0, sqlc v1.31.1, air v1.67.4.

## ADR-010 Workspace layout
`apps/web` was scaffolded before the brief; kept. Its `pnpm-workspace.yaml` settings moved to the repo root (`packages: apps/web, packages/*`). Web formatter is oxfmt (replaced prettier). All deps pinned exactly (no `latest`, no `^`) for newly added packages.

## ADR-011 Testing deferred
testcontainers-go integration tests start in M1 when repositories exist. M0 tests are the health/metrics/validation table tests and password hashing.

## ADR-012 Go module path
`archmap/api` (no VCS host known yet). Rename when the repository location is decided.
