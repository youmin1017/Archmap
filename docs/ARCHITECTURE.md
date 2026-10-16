# Architecture (as of M5 + layers view)

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

`task gen` regenerates both (plus sqlc); outputs are committed and CI runs `task gen-check`.
At runtime the API validates requests against the embedded spec (echo-v5-middleware); the TS SDK validates responses with Zod, so contract drift fails loudly.

## API (Go)

```
cmd/api            composition root (config -> db -> httpx -> feature handlers)
cmd/seed           dev seed: 3 users + sample model (via changeset.Bootstrap)
internal/
  platform/        config, logger, db, httpx (Echo shell, error mapping, dev auth),
                   apperr (typed domain errors), authctx (Actor in context)
  gen/api          generated, do not edit
  apiserver/       composes the per-feature transport handlers into api.StrictServerInterface
  health/          service + transport
  model/           domain (Element, Relation, Snapshot, patch helpers), read-only Service of main,
                   repository/ (sqlc), transport/ (+ shared API mappers)
  lint/            rule engine (errors block merge)
  changeset/       domain, State (effective model), builder (validation + change generation),
                   Between / Diff, Merge3 (3-way rebase), workflow (review, merge, versions),
                   Service, repository/ (sqlc, InTx), transport/
  user/            password hashing (argon2id), repository/ (actor lookup), transport/ (/me)
  view/            saved views: Compute (membership by kind + scope), Service, repository/ (sqlc), transport/
  seed/            sample model data (used by cmd/seed and integration tests)
  testdb/          testcontainers helper: shared postgres:18, fresh migrated DB per test
db/migrations      goose; db/queries/{user,model,changeset}.sql -> one sqlc package per feature
```

Rules (also in `AGENTS.md`): by-feature packages with `transport -> service -> repository`; Echo confined to `platform/httpx` (and transport); services and repositories never import Echo.

Middleware order: request id -> slog access log -> recover -> dev auth (only when `AUTH_DEV_HEADER=true`) -> CORS -> Prometheus -> OpenAPI request validation (only `/api/*`). `GET /metrics` is outside the spec.

### Errors

Services return `apperr` errors (invalid / not found / forbidden / conflict / unauthenticated). `httpx.errorHandler` wraps Echo's `ProblemDetailsHTTPErrorHandler` and maps them to 400 / 404 / 403 / 409 / 401 problem+json. Anything else is a 500 without leaking the message.

### Auth (interim)

Services read an `authctx.Actor{ID, Role}` from `context.Context` and enforce roles themselves (viewer reads; editor/maintainer create drafts; only the author or a maintainer edits a draft). Until real session auth exists, the dev-only `X-Dev-User: <email>` header (config `AUTH_DEV_HEADER`, off by default) fills the same context value.

## Model and changesets (M1)

`elements` / `relations` hold the CURRENT main state (soft delete via `deleted_at`). Main is read-only through the API; the only writers are `changeset.Repository.MaterializeChanges` (called by `Bootstrap`, later by merge).

A **changeset** (draft) owns an ordered log of **changes**: `add` (patch = full entity), `update` (patch = changed fields, `before` = their old values), `delete` (`before` = full snapshot). Changes are recorded in **batches** (`batch_id`, one GUI edit). Validation is structural only (kinds, names, targets / parents exist, no parent cycles, relation endpoints exist); hierarchy and duplicate rules are lint (M2). Deleting an element with children needs `cascade`; descendants and touching relations become explicit delete changes.

**Effective model** = current main snapshot + the draft's active (not undone) changes, applied in Go by `changeset.State` (pure, unit tested). **Undo** sets `undone_at` on the latest active batch, **redo** clears it on the most recently undone batch, and recording a new batch deletes the undone tail. `AddChanges` / `Undo` / `Redo` lock the changeset row (`FOR UPDATE`), so concurrent edits get a gap-free `seq`.

REST surface (M1): `GET /model/elements|relations[/{id}]`, `POST|GET /changesets`, `GET /changesets/{id}`, `GET /changesets/{id}/model`, `POST /changesets/{id}/changes`, `DELETE /changesets/{id}/changes/last`, `POST /changesets/{id}/changes/redo`, `GET /me`.

## Review, merge and history (M2)

```
draft --submit--> in_review --approve (maintainer)--> --merge (maintainer)--> merged (version n+1)
  |                  |--reject (maintainer)--> rejected
  '--abandon (author/maintainer, from draft or in_review)--> abandoned
```

- **Effective model** is now `snapshot(base_version) + active changes`; `model_snapshots` stores main after every merged version (ADR-020).
- **Diff** (`GET /changesets/{id}/diff`): `changeset.Between(base snapshot, effective model)`: added / removed / modified elements and relations, modified ones with field-level before / after. `stale` tells when main moved past the draft's base.
- **Lint** (`internal/lint`, `GET /changesets/{id}/lint`): rule engine over the effective model; errors block merge, warnings are informational (ADR-024).
- **Merge** (`POST /changesets/{id}/merge`): lock main, then fast-forward if `main == base_version`, else `Merge3` (field-level 3-way, ADR-021). Conflicts -> `409` + conflict list, nothing changes; `POST /resolve` stores `mine` / `theirs` choices; `GET /conflicts` dry-runs. The merged result is linted, protected-element rules are checked (ADR-023), then applied to main as add / update / soft-delete changes derived from `diff(theirs, result)`, snapshotted, and recorded in `model_versions`.
- **Versions**: `GET /versions`, `GET /versions/{n}/model` (read-only history).
- Pure and independently tested: `Merge3`, `Between` / `Diff.Changes`, `lint.Run`; the `Service` workflow is tested on an in-memory repository and against Postgres (including concurrent merges).

## Testing

- Unit: table-driven service tests on an in-memory repository, pure `State` logic.
- Integration (testcontainers-go, `postgres:18`, need Docker; skipped by `go test -short`): migrations + sample bootstrap, "draft never touches main" (main tables dumped before / after must be identical), undo / redo persistence, role matrix, concurrent edits, filters, and a full HTTP workflow through Echo.

## Views and layout (M5)

Views are saved perspectives over the model, deliberately outside the changeset workflow (ADR-036): they never create changes or versions.

- **Membership** is computed in Go by `view.Compute` from main or a draft's effective model (ADR-037): landscape, context, container, component, deployment (by kind + scope) and custom (manual list). The result is a set of roots plus their subtrees, so the canvas can expand and roll relations up as usual.
- **Layout** (x, y, collapsed per element) is saved per view with `PUT /views/{id}/layout` (ADR-038), autosaved by the client.
- REST: `GET|POST /views`, `PUT|DELETE /views/{id}`, `GET /views/{id}/model[?changeset_id=]`, `PUT /views/{id}/layout`.
- **Layers views** (`kind: layers`): elements carry an optional `layer`; the view stores the ordered layer names; the web client renders one flat plan per layer, stacked top to bottom (ADR-040..043).

## Web

TanStack Start + Router, TanStack Query (SSR integration), shadcn/ui (base-maia, Tailwind 4), Magic UI via shadcn registry (BorderBeam on the selected protected node, gated by `prefers-reduced-motion`), Iconify with build-time-bundled icon data, dark / light via `.dark` class. API calls go through `@archmap/api-client` same-origin; Vite proxies `/api` to `:8080` and adds the dev user header. No business logic in web.

### Canvas (M3), `src/features/canvas`

```
graph.ts        pure: tree index, visible set, edge aggregation, filters   (unit tested)
layout.ts       pure-ish: ELK compound layout adapter                      (unit tested, runs in node)
animate.ts      box tweening + useAnimatedBoxes hook                       (math unit tested)
store.ts        Zustand: expanded set, selection, dragged positions, filter (ephemeral UI state only)
useCanvasGraph  model + UI state -> React Flow nodes / edges
ElementNode / AggregatedEdge / CanvasView   rendering, semantic zoom, toolbar, minimap
ElementTreePanel / PropertyPanel            sidebar tree (search, kind / tag filters) and read-only details
```

Route `/` is the landscape of main: top bar (Main @ vN, API status, theme), element tree, canvas, property panel. See ADR-025..029.

### Draft editing (M4), `src/features/draft`

```
queries.ts        TanStack Query hooks (me, main, changesets, draft model / diff / lint / conflicts) and mutations
                  (add changes with optimistic preview, undo / redo, submit / approve / reject / abandon / merge / resolve)
optimistic.ts     pure cache preview of a change batch                       (unit tested)
patches.ts        form values -> change inputs                               (unit tested)
decor.ts          diff overlay / lint marks / change-log summaries           (unit tested)
workflow.ts       capabilities and available review actions per role / status (unit tested)
ElementDialog / RelationDialog / NewDraftDialog   forms
ReviewPanel       Lint | Diff | Conflicts | History tabs
DraftPage         /changesets/$id: draft bar (add, undo / redo, lint counters, diff overlay, workflow buttons) + workspace
```

Routes: `/` main (read-only), `/changesets` list, `/changesets/$id` draft workspace; both workspaces take `?view=<id>`. Edit mode exists only for a draft the user may edit; see ADR-030..034.

### Views (M5), `src/features/views`

```
ViewTabs / ViewDialog   tab strip and create / edit dialog
useViewWorkspace        what the canvas shows (whole model or a view), layout load + debounced autosave
layoutMap.ts            saved layout <-> canvas state                       (unit tested)
```

See ADR-038, ADR-039.

### Layers view, `src/features/layers`

```
layers.ts          pure: group by layer, lane modes (hide / focus), layer tints   (unit tested)
LayeredCanvas      stacked flat lanes, layer list (show / hide / focus), SVG link overlay, context menu
```

Elements also carry `projects` (many per element); the project picker in the element panel highlights them in every canvas (ADR-044, ADR-045). In a draft, layers can be created / renamed / reordered / deleted from the context menu and cards can be dragged between layers (ADR-046).

Layer rules (`layer_rules`, `GET|PUT /layer-rules`) restrict which element classes may be placed on which layers; the changeset service enforces them and the UI mirrors them (ADR-048). `database` is an element kind drawn as a cylinder; elements also carry `replicas` and `properties`, and the layers view groups nested elements (ADR-047).
Deployment is edited as a "Deployed on" field that is saved as `deployed_on` relations; elements are edited in place in the details panel, and the technology field suggests known technologies (ADR-049).

`ModelWorkspace` shows `LayeredCanvas` instead of the 2D canvas when the selected view is a layers view; `views/LayersEditor` edits the layer list. See ADR-042, ADR-043.

## Not yet built

Polish (M6: custom icons, roles UI, version browser). Real auth / session is deferred (dev header only).

Users and roles are managed by maintainers (`internal/user`: service, repository with a locking role change, transport; ADR-050). The version browser reads `GET /versions/{n}/model` and `GET /versions/{from}/diff/{to}` (ADR-051). Pages share `StatePanel` for loading / empty / error states (ADR-052).

