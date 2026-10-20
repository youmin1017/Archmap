# Project: Archmap — collaborative, versioned system-architecture modeling tool

> Kickoff instruction for Claude Code:
> "Read docs/PROJECT_BRIEF.md fully. Use plan mode. Implement **M0 only**, then stop and summarize."

## 1. Vision

A web app for developers / DevOps (backend, frontend, devops, mlops…) to model system architecture as a **model-first** (not canvas-first) graph, in the spirit of the C4 model, IcePanel (clean, beautiful UI, hierarchical levels) and Ilograph (smooth in-place zoom / expand).

Multiple people will use it, so the main model must be protected by **draft → review → merge** workflows, full history, and lint rules. GUI-first; YAML import/export comes later.

Architecture is described in layers, e.g. Physical machine → Infra → Cloud (Docker, K8s) → Applications → Reverse proxy (Nginx, Traefik). The deployment dimension is modeled with nested `node` elements and `deployed_on` relations.

Long-term (NOT in MVP, but keep the schema extensible): live status binding from Prometheus / OpenEverest, K8s auto-discovery, API endpoint catalog, YAML/DSL import-export, SSO.

## 2. Tech stack (fixed)

### Backend

- Go (use the version required by Echo v5's go.mod).
- **Echo v5** (`github.com/labstack/echo/v5`). Echo v5 has breaking changes vs v4 (e.g. handlers are `func(c *echo.Context) error`). BEFORE writing any HTTP code, read the official docs and `API_CHANGES_V5.md` in the Echo repo. Do NOT write v4-style code. Verify the import path is exactly `github.com/labstack/echo/v5` (beware of look-alike forks).
- `pgx` + `sqlc`, `goose` migrations, `slog`.
- PostgreSQL **18** (may use native `uuidv7()`).
- **Spec-first OpenAPI** at `contracts/openapi.yaml`, with **oapi-codegen** generating Go types and the server interface. Echo v5 support status is uncertain, so in M0 verify in this order and record the outcome in `docs/DECISIONS.md`:
  1. Stable oapi-codegen (v2): confirm from its docs / README / changelog that it supports `echo/v5` (handlers use `*echo.Context`). If yes, use it.
  2. Otherwise the experimental V3 (`oapi-codegen-exp`), which states Echo v5 support (Go 1.25+). Pin an exact version and note that it is experimental.
  3. Otherwise generate the **std-http** (net/http ServeMux) server and mount it into Echo, keeping Echo as an outer shell only.
  - For request validation, evaluate `oapi-codegen/echo-v5-middleware` (check whether a release is tagged and that it works). If not usable, validate in tests against the spec.
- Do NOT hand-write DTOs that duplicate generated types. Domain types stay separate from generated transport types; map between them in `transport/`.

### Frontend

- **TanStack Start** (React, TypeScript strict). Read the CURRENT official docs before scaffolding; do not rely on memory (the API changes quickly).
- TanStack Router (bundled), **TanStack Query** for server state, Zustand only for ephemeral canvas UI state.
- Canvas: `@xyflow/react` (React Flow) with nested nodes; `elkjs` for auto layout.
- **API client**: generate a typed TS client from `contracts/openapi.yaml` into `packages/api-client`. Prefer a generator that can also emit runtime validation (e.g. Zod schemas); verify the tool's current status first and record the choice. If no suitable generator exists, generate types only and wrap responses with hand-written Zod parsing in one module so backend drift fails loudly in dev.
- **UI kit: shadcn/ui** (Tailwind). Use the shadcn CLI to add components; own the code in `components/ui`.
- **Magic UI** (magicui.design) for tasteful motion / polish, installed via the shadcn registry / CLI per its docs. Rules:
  - Use sparingly: e.g. BorderBeam / ShineBorder on the selected or `protected` node, NumberTicker for lint / diff counters, AnimatedList for activity / history feeds, subtle shimmer for loading / empty states.
  - NEVER put heavy animations on every canvas node (performance with hundreds of nodes). Animated effects on nodes are gated to selected / hovered nodes only, and must respect `prefers-reduced-motion`.
- **Icons**:
  - Element KINDS (person / system / container / component / node) use **lucide** icons (bundled with shadcn).
  - TECHNOLOGY badges use **Iconify** as the loading layer (`@iconify/react`), with icon data **bundled at build time** (no runtime network fetches; the app must work offline). Import only the needed subset.
  - `TechIcon` resolution order: (1) user-uploaded custom icon, (2) simple-icons (monochrome, `currentColor`), (3) devicon / logos (colored) as fallback, (4) generic lucide icon.
  - Maintain a case-insensitive alias map from free-text technology / tags to icon slug (e.g. "postgres" → postgresql, "k8s" → kubernetes).
  - Handle dark-mode contrast: brand colors that are too dark on dark backgrounds fall back to `currentColor` / foreground.
- Tailwind dark / light mode via shadcn theming. Visual direction: clean, modern, IcePanel-like (rounded cards, kind-specific accent colors, tech badges, subtle shadows).

### Local dev

- `deploy/compose/compose.yaml` runs ONLY PostgreSQL 18. API (`go run` / air) and web (`pnpm dev`) run natively on the host. Check the official `postgres:18` image notes for the correct data-volume mount path (it differs from older versions).
- Integration tests use testcontainers-go (they must not depend on the manually started DB).
- No Kubernetes and no full-stack compose in the MVP.

### Monorepo layout (polyglot; NO Nx / Turborepo)

```
/apps/api            (Go module)
/apps/web            (TanStack Start)
/contracts           (openapi.yaml — single source of truth for the API)
/packages/api-client (generated TS client)
/deploy/compose      (postgres 18 only)
/docs
Taskfile.yml  AGENTS.md  pnpm-workspace.yaml
```

- Taskfile.yml (go-task) is the single entry point: `task db-up|db-down|migrate|gen|dev-api|dev-web|test|lint`. `task gen` regenerates Go server stubs and the TS client from the OpenAPI spec. Decide whether generated code is committed or generated in CI, and record it.
- pnpm workspace covers only TS packages. Provide CI path filters so api / web jobs run only when relevant files change.

## 3. Architecture rules (Go)

- NOT MVC. Package **by feature**, layered inside each feature: `transport (Echo handlers) → service → repository (sqlc)`.
  ```
  apps/api/
    cmd/api/main.go
    internal/
      platform/   (config, db, logger, httpx, auth middleware, error handler)
      model/      (elements, relations)
      changeset/  (drafts, changes, diff, merge, versions)
      view/       (views + layout)
      lint/       (rule engine)
      user/       (users, roles)
  ```
- **Echo is confined to `transport/` and `platform/httpx`.** Services and repositories must NOT import Echo; they receive `context.Context` and plain types. Handlers stay thin: bind → validate → call service → map result / error.
- Centralized error handling: typed domain errors → RFC 7807 problem+json via Echo's HTTP error handler.
- Interfaces are defined on the consumer side, only where needed. No global state. Pass `context.Context` everywhere.
- Middleware: request id, slog access log, recover, CORS (dev), session auth, and a Prometheus metrics endpoint (use the Echo-team-maintained Prometheus middleware if compatible with v5; verify).
- Table-driven tests for services; integration tests with testcontainers-go for repositories and merge logic.
- All business rules live in Go. The web app contains no domain logic beyond UI validation. Do NOT use TanStack Start server functions for business logic (thin proxy / SSR only).

## 4. Domain model

Core idea: **Model (structure / semantics) is strictly separated from Layout (positions / visual state).** A pure drag of a node must never create a model change.

Entities (UUIDv7 ids). Updated through M10; `docs/DECISIONS.md` has the reasons.

- `elements`: id, parent_id (nullable tree), kind (`person|system|container|component|node`), name, description, technology, tags text[], protected bool, layer, projects text[], replicas, properties jsonb, type, environment, lifecycle, criticality, runbook_url, docs_url, last_reviewed_at, version int, deleted_at, created_at, updated_at
  - `node` = deployment infrastructure (physical machine, VM, k8s cluster, namespace, docker host, reverse proxy like Nginx / Traefik…), nestable via parent_id.
  - `type` refines the kind (`database-server|database|schema|cache|queue|proxy`); a database server is a container and its logical databases are components under it (ADR-062). The `database` kind of M6 was retired into `container` + type `database`.
- `relations`: id, source_id, target_id, kind (`uses|deployed_on`), description, technology, version, deleted_at, created_at, updated_at
  - `deployed_on` links container / component → node (how "app runs on infra" is expressed). Descendants inherit their ancestors' deployment.
- `owners` (team or person, contact, on-call URL) and `element_owners` (element, owner, role `owner|maintainer|oncall|escalation`, one owner per role): versioned model data; owners and environment are inherited from ancestors (ADR-063, ADR-064). An owner is a named team / person, or stands for a user or a group (`principal_id`, id `uuidv5(principal)`, live name shown; ADR-069).
- `groups` (name unique ignoring case, description, soft delete) and `group_members`: account administration managed by maintainers, not versioned (ADR-070).
- `views`: id, name, kind (`landscape|context|container|component|deployment|custom|layers|unowned`), scope_element_id (nullable), description, layers text[]
- `view_elements` (LAYOUT only): view_id, element_id, x, y, w, h, collapsed bool, included bool (custom view membership)
- `changesets`: id, title, description, author_id, base_version int, status (`draft|in_review|merged|rejected|abandoned`), approved_by, approved_at, created_at, merged_at
- `changes`: id, changeset_id, seq, batch_id, op (`add|update|delete`), target_type (`element|relation|owner|ownership`), target_id, patch jsonb, before jsonb, undone_at
- `model_versions`: number (monotonic), changeset_id, merged_by, merged_at, summary; `model_snapshots` (full model per version, base of 3-way merges); `merge_resolutions`
- `layer_rules`: which element classes (kind or type) may sit on which layers (ADR-048)
- `users` (id, email, name, role `viewer|editor|maintainer`, disabled_at, last_login_at), `user_identities`, `sessions`, `auth_flows` (M7, ADR-053..061)
- Reserve (table only, no logic): `bindings` (element_id, kind, config jsonb) for future Prometheus / health bindings.
- Custom icons: deferred (ADR-052).

`elements` / `relations` hold the CURRENT main (materialized) state. History = `changes` of merged changesets + `model_versions`.

## 5. Core behaviors

### 5.1 Draft / merge workflow (most important part)

- Every modification is a change inside a **changeset**. Under the default review policy (`required`) main is read-only and edits happen in drafts. Under `direct` (a maintainer setting, ADR-072) an editor's save on main is a changeset merged at once as a new version. Protected elements still go to review, and edits that add lint errors are kept as drafts.
- Reads inside a draft return "main at base_version + applied changes" (effective model; keep simple, optimize later).
- Each GUI edit becomes one or more `changes` (with `before` captured) → per-draft undo / redo + audit trail.
- A draft can be submitted for review, approved and merged by a `maintainer`, rejected, or abandoned. Approval is a separate step; an author may approve and merge their own draft only when `MERGE_ALLOW_SELF` is set, and changes to protected elements always need an approver and merger other than the author (ADR-022, ADR-023).
- **Diff** endpoint: structured diff main vs draft (added / removed / modified elements and relations, field-level for modified).
- **Merge** (optimistic concurrency): if main version == base_version → fast-forward. Otherwise rebase with per-target field-level 3-way merge; disjoint fields auto-merge; same-field edits or delete-vs-update produce a `conflict` list requiring explicit resolution ("mine" / "theirs"). Never silently overwrite.
  - The full rebase is implemented (`changeset.Merge3`); the "reject stale merges" fallback was never needed.
- `protected` elements change only via a changeset that was `in_review` and merged by a maintainer.
- Soft delete only; deleting an element with children requires an explicit cascade flag visible in the diff.
- Drafts may stay unmerged forever (this is how temporary diagrams are supported). Views can be created inside a draft.
- No pessimistic locks. Optionally show a soft "Alice is editing this view" hint later; never block.

### 5.2 Lint rules (run on a draft; errors block merge, warnings are shown in review)

Small rule engine (`lint.Rule` interface):

- ERROR `hierarchy`: component → container, container → system (both require a parent), node may nest in node, person / system are roots.
- ERROR `relation`: endpoints exist; no duplicate (source, target, kind); `deployed_on` goes from a container / component to a node.
- ERROR `duplicate-sibling-name`: same name (ignoring case and surrounding spaces) under the same parent.
- ERROR `type-kind`: an element type fits its kind (M10). ERROR `ownership`: assignments point at live elements / owners; named owners' names are unique; a user / group owner points at an existing, active principal.
- ERROR `owner-required`: an element whose effective environment is prod has an effective owner (M10).
- WARN `orphan`: no relations, no parent and no children, other than person / system.
- WARN `uses-technology`: `uses` relation without technology.
- WARN `deployment`: container / component not deployed on a node, directly or through an ancestor (effective deployment, M10).
- WARN `stale-review`: last review older than `LINT_REVIEW_MAX_AGE` (180 days); never reviewed only for prod (M10).
- WARN `deprecated-dependency`: an active element uses or is deployed on a deprecated one (M10).
- WARN `owner-inactive`: an element is assigned to a disabled user or a deleted group (ADR-069).
- Layer rules (ADR-048) are enforced when editing, not by lint.

### 5.3 Auth (MVP)

- Email + password (argon2id) with session cookie; dev seed with 3 users (viewer / editor / maintainer). Hide auth behind an interface so OIDC can be added later.
- Role checks in the service layer, not only in middleware.

## 6. API (REST / JSON, `/api/v1`, defined in `contracts/openapi.yaml`)

- `GET /model/elements?parent_id=&q=`, `GET /model/elements/{id}`; same for relations; `GET /model/owners` (main, read-only; older versions via `/versions/{n}/model`)
- `POST /changesets`, `GET /changesets`, `GET /changesets/{id}`
- `GET /changesets/{id}/model`, `GET /changesets/{id}/diff`, `GET /changesets/{id}/lint`
- `POST /changesets/{id}/changes` (batch of ops), `DELETE /changesets/{id}/changes/last` (undo), `POST /changesets/{id}/changes/redo`
- `POST /changesets/{id}/submit | approve | reject | abandon | merge | resolve`
- `GET /versions`, `GET /versions/{n}/model`
- `GET /versions/{from}/diff/{to}`, `GET /changesets/{id}/conflicts`
- `GET / POST /views`, `PUT / DELETE /views/{id}`, `GET /views/{id}/model`, `PUT /views/{id}/layout` (layout writes create NO changes / versions)
- `GET / PUT /layer-rules`, `/users*` (maintainers), `GET / PUT /settings` (review policy; writes: maintainers), `POST /model/changes` and `POST /model/changes/{id}/revert` (direct edits), `GET /directory` (users and groups for pickers), `/groups*` (writes: maintainers), `/auth/*`, `GET /me`, `PUT /me/password`; `GET /metrics` (Prometheus)

## 7. Frontend requirements

- **Canvas** (React Flow): hierarchical nodes — a parent visually contains its children. **Semantic zoom / expand-collapse**: a collapsed node shows a summary; expanding (double-click or zoom threshold) reveals children in place with a smooth transition. Edges between collapsed nodes aggregate (show a count) and re-route to the visible ancestor.
- Custom node card: kind icon (lucide), name, description snippet, `TechIcon` badges, lint / diff status indicator, protected marker.
- Layout: ELK for new / untouched nodes; user drags persist as Layout only.
- Sidebar: element tree (search, filter by kind / tag) and property panel (shadcn form components).
- Edit mode only inside a draft. Persistent top bar: context (Main @ vN, or Draft "title"), Undo / Redo, Lint status (NumberTicker), "Submit for review".
- Pages: `/` main (views are chosen with `?view=`, not a separate page), `/changesets`, `/changesets/:id` (canvas + diff + lint + conflicts), `/versions` (history; open a past version read-only; consecutive direct edits are grouped), `/users`, `/settings`, `/login`.
- Diff overlay: green added / red removed / amber modified + list panel.
- TanStack Query with optimistic updates for drafts; typed client from `packages/api-client`.
- Accessibility basics: keyboard navigation in tree / sidebar, visible focus, `prefers-reduced-motion` respected, sufficient contrast in both themes.

## 8. Milestones — implement ONE at a time, then stop and summarize

Before coding: enter plan mode, read this whole file, write `docs/ARCHITECTURE.md` and `docs/DECISIONS.md` (ADR style), and ask me about anything ambiguous. Create `AGENTS.md` with build / test / run commands and the architecture rules above.

- **M0 Scaffold**: monorepo, compose (postgres 18 only), Taskfile.yml, OpenAPI skeleton + codegen pipeline for Go and TS including the Echo v5 verification in section 2, Echo v5 health + metrics endpoints, TanStack Start app with shadcn / Magic UI / Iconify wired, one real API call through the generated client, seed users. DoD: `task gen` is reproducible; `task db-up && task dev-api` and `task dev-web` show a themed page that successfully calls the API.
- **M1 Model + Changeset core**: migrations, sqlc queries, element / relation services, changeset + changes + effective model + undo / redo. Seed a sample model (a small system with API, DB, queue, and a deployment tree: physical host → docker → nginx / traefik → containers). DoD: tests cover add / update / delete in a draft without touching main.
- **M2 Diff, lint, merge, versions**: diff, lint engine, fast-forward (+ rebase / 3-way if not time-boxed), conflict reporting, version history, protected elements. DoD: table-driven + integration tests for every scenario in 5.1.
- **M3 Canvas**: nested React Flow rendering of main, ELK layout, expand / collapse semantic zoom, edge aggregation, element tree + property panel, TechIcon badges. DoD: smooth expand / collapse on the seed model.
- **M4 Draft editing UX**: create / edit / delete / connect on canvas inside a draft, undo / redo, top bar, lint panel, submit / approve / merge flow, diff overlay, conflict resolution UI.
- **M5 Views & layout**: multiple views over the same model (landscape / container / deployment), layout persistence separated from model.
- **M6 Polish**: custom icon upload (highest-priority source in `TechIcon`), roles UI, version browser, empty / loading / error states with Magic UI polish, README with screenshots.

## 9. Non-goals for MVP

Realtime multi-cursor collaboration (CRDT), Prometheus / OpenEverest / K8s integration for live status, API endpoint catalog, YAML / DSL import-export, SSO, full-stack compose, Kubernetes deployment of the app itself.

## 10. Working agreements

- Small, reviewable commits (conventional commits). Small functions; no premature abstraction.
- Every milestone ends with: passing tests, updated docs, and a short "what I did / what's next / open questions" summary.
- Library APIs here change fast (Echo v5, oapi-codegen, TanStack Start, Magic UI, shadcn, Iconify). Always check current official docs before using them; never code from memory.
- If a requirement is unclear or conflicts, ask instead of guessing. Record non-obvious choices in `docs/DECISIONS.md`.
