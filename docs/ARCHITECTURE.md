# Architecture (as of M0)

See `PROJECT_BRIEF.md` for product scope and domain model. This file describes how the code is organised.

## Monorepo

```
apps/api              Go module (archmap/api)
apps/web              TanStack Start app
contracts/openapi.yaml  single source of truth for the HTTP API
packages/api-client   generated TS client (@archmap/api-client)
deploy/compose        PostgreSQL 18 only (local dev)
Taskfile.yml          single entry point (go-task)
```

pnpm workspace covers only TS packages (`apps/web`, `packages/*`). Go is a separate module; no Nx / Turborepo.

## Contract-first pipeline

```
contracts/openapi.yaml
   ├─ oapi-codegen v2 (echo5-server, strict-server, models, embedded-spec) → apps/api/internal/gen/api
   └─ @hey-api/openapi-ts (typescript + client-fetch + zod + sdk w/ validator) → packages/api-client/src/gen
```

`task gen` regenerates both; outputs are committed and CI runs `task gen-check`.
At runtime the API validates requests against the embedded spec (echo-v5-middleware); the TS SDK validates responses with Zod, so contract drift fails loudly.

## API (Go)

```
cmd/api            composition root (config → db → httpx → feature handlers)
cmd/seed           dev seed (3 users)
internal/
  platform/{config,logger,db,httpx}
  gen/api          generated, do not edit
  health/          service.go (no Echo) + transport.go (generated strict interface)
  user/            password hashing (argon2id); users/roles grow in M1+
  user/repository/sqlcgen   sqlc output
db/migrations      goose; db/queries.sql sqlc queries
```

Rules (also in `AGENTS.md`): by-feature packages with `transport → service → repository`; Echo confined to `platform/httpx` (and transport); errors become RFC 9457 problem+json in one place.

Middleware order: request id → slog access log → recover → CORS (configured origins) → Prometheus → OpenAPI request validation (only `/api/*`). `GET /metrics` is outside the spec.

## Web

TanStack Start + Router, TanStack Query (SSR integration), shadcn/ui (base-maia, Tailwind 4), Magic UI via shadcn registry (BorderBeam, gated by `prefers-reduced-motion`), Iconify with build-time-bundled icon data (`TechIcon` stub), dark/light via `.dark` class. API calls go through `@archmap/api-client` same-origin; Vite proxies `/api` → `:8080` in dev. No business logic in web.

## Not yet built

Model / changeset / lint / merge (M1–M2), canvas (M3), draft editing UX (M4), views (M5), polish (M6). Auth/session is deferred; M0 only seeds users.
