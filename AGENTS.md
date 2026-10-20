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

Auth (M7, ADR-053..061): server-side sessions (HttpOnly cookie) + CSRF token header on unsafe requests. In development (`APP_ENV=development`, `AUTH_DEV_LOGIN=true` in `.env.example`) the login page offers one-click sign-in as the seeded viewer / editor / maintainer; the seeded local password is `archmap-dev-password`. Scripts: `POST /api/v1/auth/login` `{"provider":"dev","username":"editor@archmap.local"}`, keep the cookie, send `X-CSRF-Token` from the response on POST/PUT/DELETE. Break-glass / first maintainer: `go run ./cmd/admin ensure-maintainer -email … -name …` (password on stdin). Never add a header-based or email-matching login path. Web unit tests: `pnpm --filter web test` (vitest).

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
- Every edit is a change in a changeset. Main is read-only unless the review policy is `direct` (ADR-072); a direct edit is then an automatically merged changeset (`changeset.Service.ApplyDirect`), never a write around the changeset tables. Roles are checked in services via `authctx.Actor` from the context.
- sqlc: one package per feature, queries in `db/queries/<feature>.sql`; change SQL/spec, then `task gen`.
- No global state; interfaces defined on the consumer side; table-driven service tests; testcontainers-go for repository/merge integration tests.
- All business rules in Go; web has UI validation only (no TanStack server functions for business logic).
- Model (structure) and Layout (positions) are strictly separate: dragging a node never creates a model change.

## Web UI style rules

Linear/Vercel-like: dense, neutral, crisp. Decoration must carry meaning.

- shadcn style is `base-nova` (`components.json`). Add/update primitives with `pnpm exec shadcn add <name> --overwrite`, then review the diff: `ui/sonner.tsx` must keep `useColorMode` (no `next-themes`), and drop unused imports. Colors only via theme tokens in `src/styles.css` (`--kind-*` for element kinds, `shadow-island` for floating chrome); light and dark both.
- Sizes: controls `h-7`/`h-8` (`size="sm"` in toolbars), radius `rounded-md`/`rounded-lg`, never pills except status badges. Chips/badges inside cards are `rounded bg-muted text-[11px]`.
- No glow, gradients, shimmer/border-beam, colored outlines or thick accent bars. The only gradient is the brand logo tile.
- Element cards (canvas and layers view) use `cardClass(tone)` from `features/canvas/shapes.tsx`: neutral 1px border; kind color only in `KindTile`; selection = primary border + `ring-3 ring-primary/15`; diff = colored border + `DiffMark` (+ / ✎ / −, same marks as the review diff). Keep the two canvases consistent.
- Color has one meaning each: kind (`--kind-*`), diff (emerald added / amber modified / red removed), status (`StatusBadge`), lint (destructive / amber). Layer hue is a small dot only, never a fill.
- Section headings: `text-[11px] font-medium tracking-wide text-muted-foreground uppercase`. Read-only details are label/value lists (`dl`), not disabled inputs. Show element names, never raw ids.
- Chrome: one top bar (brand, nav, breadcrumb context) + one sub-bar (`ViewTabs`, page actions in its `actions` slot). Don't add more stacked bars. Canvas tools are icon buttons with `Tooltip`s in a floating island.
- Forms: group into sections; in dialogs only the body scrolls (header and footer stay visible), footer says what is missing. Show validation errors only after the user touched the field (hint text before). Few options → selectable cards/chips (`role="radio"` / `aria-pressed`), not a select.
- Every panel must stay reachable at any width: side panels go inline at `md`/`lg`, drawers below (`ModelWorkspace`).
- Empty/loading/error states use `StatePanel`. Respect `prefers-reduced-motion`; a11y attributes (`aria-*`, focus-visible rings, keyboard tree) are not optional.
