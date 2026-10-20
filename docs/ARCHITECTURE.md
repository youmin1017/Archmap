# Architecture (as of M10)

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
cmd/admin          break-glass CLI: ensure-maintainer (create or restore; password from stdin)
internal/
  platform/        config, logger, db, httpx (Echo shell, error mapping, session + CSRF middleware),
                   apperr (typed domain errors), authctx (Actor in context),
                   reqinfo (client IP / user agent / session token in context), password (argon2id)
  gen/api          generated, do not edit
  apiserver/       composes the per-feature transport handlers into api.StrictServerInterface
  health/          service + transport
  model/           domain (Element, Relation, Snapshot, patch helpers), read-only Service of main,
                   repository/ (sqlc), transport/ (+ shared API mappers)
  lint/            rule engine (errors block merge)
  changeset/       domain, State (effective model), builder (validation + change generation),
                   Between / Diff, Merge3 (3-way rebase), workflow (review, merge, versions),
                   Service, repository/ (sqlc, InTx), transport/
  auth/            providers (local, dev, authtest fake), login pipeline, sessions, rate limiting,
                   RoleResolver; repository/ (sqlc), transport/ (/auth/*, cookies)
  user/            user management (create, disable, reset password, roles, identities, break-glass
                   invariant), repository/ (sqlc), transport/ (/me, /users)
  view/            saved views: Compute (membership by kind + scope), Service, repository/ (sqlc), transport/
  layerrule/       layer rules (which element classes may sit on which layers): Service, repository/, transport/
  seed/            sample model data (used by cmd/seed and integration tests)
  testdb/          testcontainers helper: shared postgres:18, fresh migrated DB per test
db/migrations      goose; db/queries/<feature>.sql -> one sqlc package per feature
```

Rules (also in `AGENTS.md`): by-feature packages with `transport -> service -> repository`; Echo confined to `platform/httpx` (and transport); services and repositories never import Echo.

Middleware order: request id -> slog access log -> recover -> request info (IP, user agent, cookies into the context) -> cross-origin protection (unsafe methods) -> CORS -> session (cookie -> `authctx.Actor`) -> CSRF token (unsafe `/api/*` with a session) and forced password change -> Prometheus -> OpenAPI request validation (only `/api/*`). `GET /metrics` is outside the spec.

### Errors

Services return `apperr` errors (invalid / not found / forbidden / conflict / unauthenticated / too many requests). `httpx.errorHandler` wraps Echo's `ProblemDetailsHTTPErrorHandler` and maps them to 400 / 404 / 403 / 409 / 401 / 429 problem+json. Anything else is a 500 without leaking the message.

### Auth (M7)

Services read an `authctx.Actor{ID, Role}` from `context.Context` and enforce roles themselves (viewer reads; editor/maintainer create drafts; only the author or a maintainer edits a draft). How the actor got there is not their concern:

```
browser --cookie--> httpx session middleware --SessionResolver--> auth.Service.Resolve
                                                     (sessions JOIN users, disabled excluded)
POST /auth/login {provider, username, password}
  -> rate limit -> Provider.Complete -> Identity{issuer, subject, ...}
  -> user_identities by (issuer, subject)   [never by email]
       unknown: provider.AutoProvision ? new viewer user : 403
  -> disabled? local-password policy? -> RoleResolver -> new session + cookie + csrf token
```

`users` is the internal account, `user_identities` the ways to sign in (ADR-053). Providers: `local` (email + argon2id), `dev` (development only; startup fails otherwise), later OIDC as a `redirect` provider (ADR-054). Sessions are server-side and opaque (ADR-055); unsafe requests need a same-origin request and the session's CSRF token (ADR-056). Login failures are rate limited (ADR-057). Roles come from `RoleResolver` (stored roles in M7, ADR-058). Linking identities and the break-glass maintainer invariant: ADR-059; user management: ADR-060.

## Model and changesets (M1)

`elements` / `relations` hold the CURRENT main state (soft delete via `deleted_at`). Main changes only through `changeset.Repository.MaterializeChanges` (called by `Bootstrap`, by merge and by direct edits); there is no other writer.

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
- **Versions**: `GET /versions`, `GET /versions/{n}/model` (read-only history). Each version carries its changeset's `kind` and `reverts`.
- **Review policy** (ADR-072, `internal/settings`, `GET / PUT /settings`): `required` (default) keeps main read-only. Under `direct`, `POST /model/changes` (`changeset.Service.ApplyDirect`) takes the main lock, builds the batch on the current main with the same `builder` as drafts, stores it as a changeset, and finishes in `finishDirect`. If the batch adds lint errors (compared to main), it stays a draft. If it touches a protected element, it is submitted (`in_review`). Otherwise it is committed with the merge's `commit` step as version n+1, and the changeset is marked `kind = 'direct'` (direct changesets are not listed with the drafts). `POST /model/changes/{id}/revert` undoes a direct edit by merging its version back (`Merge3(base = v, mine = v-1, theirs = main)`): a conflict means someone changed the same fields since. The policy is read through the consumer-side `changeset.PolicySource` (implemented by the settings repository).
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

TanStack Start + Router, TanStack Query (SSR integration), shadcn/ui (base-nova, Tailwind 4), Iconify with build-time-bundled icon data, dark / light via `.dark` class. API calls go through `@archmap/api-client` same-origin; Vite proxies `/api` to `:8080` (same origin, so the session cookie just works). No business logic in web.

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

Focus / drill-down (ADR-071): `?focus=<id>` on main, drafts and versions. `graph.focusScope` takes the focused subtree plus its outside neighbours (lifted like a container view on the server), and `graph.focusTree` turns that into the tree the canvas lays out, so `visibleElements` / `aggregateEdges` / ELK work unchanged. `ModelWorkspace` mirrors the URL into `store.focusId`. While focused, the store keeps the view's layout aside (`viewLayout`) and keeps per-focus expand state and drags in memory (`focusLayouts`). It does not bump `revision`, so a focus never autosaves into a view. Double-click drills in, `F` focuses the selection, `Esc` goes up; the breadcrumb sits in the canvas toolbar island. ELK runs in a web worker (`elk-worker.min.js?url`) and falls back to the bundled build inline. React Flow renders only visible elements.

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

Routes: `/` main (read-only, or editable under the `direct` review policy), `/settings` (maintainers), `/changesets` list, `/changesets/$id` draft workspace; both workspaces take `?view=<id>`. Edit mode exists for a draft the user may edit and, under the `direct` review policy, for main. Both pages use `features/editing/useModelEditing` (dialogs, in-place editor, menus, Add menu) and differ only in where a batch goes: `useAddChanges` or `useApplyDirect`. See ADR-030..034 and ADR-072.

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

Custom icon upload (M9). OIDC (M15) is a new `auth.Provider`; the endpoints, tables and login page already handle `redirect` providers.

Users and roles are managed by maintainers (`internal/user`: service, repository with a locking role change, transport; ADR-050). The version browser reads `GET /versions/{n}/model` and `GET /versions/{from}/diff/{to}` (ADR-051). Pages share `StatePanel` for loading / empty / error states (ADR-052).

## Model enrichment (M10)

```
element ── type (database-server | database | schema | cache | queue | proxy)   ADR-062
        ── environment, lifecycle, criticality, runbook_url, docs_url, last_reviewed_at   ADR-065
owner (team | person | user | group) ◄── ownership (element, role) ──► element   ADR-063, ADR-069
effective = own value, else nearest ancestor's: owners per role, environment, deployment   ADR-064
```

Owners and ownerships are model data: change targets `owner` / `ownership` next to `element` / `relation`, so they go through drafts, diff (`Diff.Owners/Ownerships`), 3-way merge (`mergeSet` + dangling references) and snapshots. Ownership ids are `uuidv5(element, role)`, so concurrent assignments of the same role meet on one target; `Merge3` now merges an entity added on both sides field by field (ADR-068). `model.ComputeEffective` derives the inherited values; every model read returns them as `Element.effective`, and lint and the `unowned` view use the same function. Time-dependent lint rules get `lint.Options` (clock, `LINT_REVIEW_MAX_AGE`). Human-owned fields for a later connector sync: `model.HumanOwnedFields` + ownership (ADR-067).

Owners from users and groups (ADR-069, ADR-070): a `user` / `group` owner carries `principal_id`, and its id is `model.PrincipalOwnerID(principal)` (uuidv5; the web computes the same id in `lib/uuidv5.ts`), so picking a user twice or in two drafts meets on one entity. The web's owner picker lists the directory (`GET /directory`) and adds the owner entity in the same batch as the ownership when it is new. The changeset builder checks principals through the consumer-side `changeset.PrincipalSource` (implemented by `user/repository.GroupRepository`), and lint gets them in `lint.Options.Principals` for `owner-inactive`. The owner's stored name is the name at creation; the web shows the live name from the directory. Groups live in the `user` feature (`GroupService`, maintainer-only writes) and are not versioned.
