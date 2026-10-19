import { useEffect, useMemo, useState } from "react"
import { Link } from "@tanstack/react-router"
import { toast } from "sonner"
import { ChevronDown, CircleAlert, GitCompare, GitMerge, Lock, Plus, Redo2, TriangleAlert, Undo2 } from "lucide-react"
import type { Element, ElementKind, Relation } from "@archmap/api-client"

import { AppTopBar } from "@/components/app-top-bar"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { NumberTicker } from "@/components/ui/number-ticker"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Toggle } from "@/components/ui/toggle"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { technologySuggestions } from "@/components/tech-icon/catalog"
import { cn } from "@/lib/utils"
import { hostIds } from "../canvas/deployment"
import { PropertyPanel } from "../canvas/PropertyPanel"
import { allProjects, ancestorIds, buildTree, descendantCount } from "../canvas/graph"
import { kinds, kindMeta } from "../canvas/kind"
import { useCanvasStore } from "../canvas/store"
import { ModelWorkspace, Status } from "../workspace/ModelWorkspace"
import { diffOverlay, lintMarks } from "./decor"
import { ElementDialog } from "./ElementDialog"
import { ElementEditor } from "./ElementEditor"
import type { ElementFormValues } from "./ElementFields"
import { RelationDialog, type RelationFormValues } from "./RelationDialog"
import {
  addElementInput,
  addRelationInput,
  deleteElementInput,
  deleteRelationInput,
  deploymentInputs,
  elementPatch,
  relationPatch,
} from "./patches"
import {
  useAbandon,
  useAddChanges,
  useApprove,
  useChangesetDetail,
  useDraftConflicts,
  useDraftDiff,
  useDraftLint,
  useDraftModel,
  useMe,
  useMerge,
  useRedo,
  useReject,
  useResolve,
  useSubmit,
  useLayerRules,
  useUndo,
  useUpdateView,
} from "./queries"
import { edgeEntries, layerEntries, nodeEntries, paneEntries, stackEntries, type DraftMenuDeps } from "./menuEntries"
import { LayerNameDialog } from "../views/LayerNameDialog"
import { layerViolation, onLayer, removeLayerFrom, renameLayerIn, shiftLayer, violationMessage } from "../layers/layers"
import { ViewTabs } from "../views/ViewTabs"
import { useViewWorkspace } from "../views/useViewWorkspace"
import { ReviewPanel } from "./ReviewPanel"
import { StatusBadge } from "./StatusBadge"
import { capabilities, statusLabel, undoRedoState, workflowActions, type WorkflowAction } from "./workflow"

const actionLabel: Record<WorkflowAction, string> = {
  submit: "Submit for review",
  approve: "Approve",
  reject: "Reject",
  merge: "Merge",
  abandon: "Abandon",
}

type ElementDialogState = {
  kind?: ElementKind
  parentId?: string | null
  layer?: string
} | null
type RelationDialogState = { source: string; target: string } | { relation: Relation } | null

export function DraftPage({
  id,
  viewId,
  onViewChange,
}: {
  id: string
  /** Selected view (URL search param); undefined = whole model. */
  viewId?: string
  onViewChange: (viewId: string | undefined) => void
}) {
  const me = useMe()
  const detail = useChangesetDetail(id)
  const cs = detail.data?.changeset
  const changes = useMemo(() => detail.data?.changes ?? [], [detail.data])
  const caps = capabilities(me.data, cs)
  const open = cs?.status === "draft" || cs?.status === "in_review"

  const model = useDraftModel(id)
  const diff = useDraftDiff(id)
  const lint = useDraftLint(id)
  const conflicts = useDraftConflicts(id, open)

  const add = useAddChanges(id)
  const undo = useUndo(id)
  const redo = useRedo(id)
  const submit = useSubmit(id)
  const approve = useApprove(id)
  const reject = useReject(id)
  const abandon = useAbandon(id)
  const merge = useMerge(id)
  const resolve = useResolve(id)
  const updateView = useUpdateView()
  const layerRules = useLayerRules().data ?? []

  const [showDiff, setShowDiff] = useState(true)
  const [tab, setTab] = useState("details")
  const [reviewTab, setReviewTab] = useState("lint")
  const [elementDialog, setElementDialog] = useState<ElementDialogState>(null)
  /** Element whose details are being edited in place (Edit / Save / Cancel in the panel). */
  const [editingId, setEditingId] = useState<string | null>(null)
  const [relationDialog, setRelationDialog] = useState<RelationDialogState>(null)
  const [deleteTarget, setDeleteTarget] = useState<Element | null>(null)
  const [layerDialog, setLayerDialog] = useState<{ renaming?: string } | null>(null)
  const [layerDelete, setLayerDelete] = useState<string | null>(null)

  const select = useCanvasStore((s) => s.select)
  const selectedId = useCanvasStore((s) => s.selectedId)
  const reveal = useCanvasStore((s) => s.reveal)

  // The draft's own model (no ghosts): what dialogs validate and offer.
  const liveTree = useMemo(() => buildTree(model.data?.elements ?? []), [model.data])
  const overlay = useMemo(() => diffOverlay(showDiff ? diff.data : undefined), [showDiff, diff.data])

  // Canvas shows the effective model plus, with the diff overlay on, ghosts of what the draft removed.
  const wholeElements = useMemo(
    () => [...(model.data?.elements ?? []), ...overlay.removedElements],
    [model.data, overlay.removedElements]
  )
  const wholeRelations = useMemo(
    () => [...(model.data?.relations ?? []), ...overlay.removedRelations],
    [model.data, overlay.removedRelations]
  )
  // A selected view shows its own subset of the draft's effective model (no removed-element ghosts there).
  const ws = useViewWorkspace({
    viewId,
    changesetId: id,
    fullModel: model.data,
    canSaveLayout: me.data?.role === "editor" || me.data?.role === "maintainer",
  })
  const elements = viewId ? (ws.elements ?? []) : wholeElements
  // A layers view shows the stacked layer plans; its layer names also feed the element form's suggestions.
  const viewLayers = ws.viewModel?.view.kind === "layers" ? ws.viewModel.view.layers : undefined
  const technologyOptions = useMemo(
    () => technologySuggestions((model.data?.elements ?? []).map((e) => e.technology)),
    [model.data]
  )
  const projectOptions = useMemo(() => allProjects(model.data?.elements ?? []), [model.data])
  const layerOptions = useMemo(
    () => [...new Set([...(viewLayers ?? []), ...(model.data?.elements ?? []).map((e) => e.layer).filter(Boolean)])],
    [viewLayers, model.data]
  )
  const relations = viewId ? (ws.relations ?? []) : wholeRelations
  const marks = useMemo(() => lintMarks(lint.data, model.data?.relations ?? []), [lint.data, model.data])

  const { canUndo, canRedo } = undoRedoState(changes)
  const editable = caps.canEdit
  const busy = add.isPending || undo.isPending || redo.isPending

  // Keyboard: Ctrl/Cmd+Z undo, Ctrl/Cmd+Shift+Z or Ctrl+Y redo.
  useEffect(() => {
    if (!editable) return
    const onKey = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey)) return
      const t = e.target
      if (t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement) return
      const key = e.key.toLowerCase()
      if (key === "z" && !e.shiftKey && canUndo) {
        e.preventDefault()
        undo.mutate()
      } else if ((key === "z" && e.shiftKey) || key === "y") {
        if (canRedo) {
          e.preventDefault()
          redo.mutate()
        }
      }
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [editable, canUndo, canRedo, undo, redo])

  // Leaving the element (or the draft becoming read-only) ends the in-place edit.
  useEffect(() => {
    setEditingId((cur) => (editable && cur === selectedId ? cur : null))
  }, [selectedId, editable])

  const isGhost = (elId: string) => overlay.elements.get(elId) === "removed"

  // The draft's own relations (not a view's subset): what the deployment diff must compare against.
  const draftRelations = model.data?.relations ?? []

  /** Saves the in-place editor: the element's changed fields and its deployment, as one undoable batch. */
  function saveElement(existing: Element, values: ElementFormValues) {
    const inputs = [
      elementPatch(existing, values, caps.canProtect),
      ...deploymentInputs(existing.id, draftRelations, values.deployedOn),
    ].filter((i) => i !== null)
    setEditingId(null)
    if (inputs.length > 0) add.mutate(inputs)
  }

  function startEditing(e: Element) {
    if (isGhost(e.id)) return
    select(e.id)
    setTab("details")
    setEditingId(e.id)
  }

  function submitElement(values: ElementFormValues) {
    const target = elementDialog
    if (!target) return
    const newId = crypto.randomUUID()
    add.mutate([addElementInput(newId, values, caps.canProtect), ...deploymentInputs(newId, [], values.deployedOn)])
    const parentChain = values.parentId ? [values.parentId, ...ancestorIds(liveTree, values.parentId)] : []
    reveal(newId, parentChain)
    setElementDialog(null)
  }

  function submitRelation(values: RelationFormValues) {
    const target = relationDialog
    if (!target) return
    if ("relation" in target) {
      const input = relationPatch(target.relation, values)
      setRelationDialog(null)
      if (input) add.mutate([input])
      return
    }
    add.mutate([addRelationInput(crypto.randomUUID(), target.source, target.target, values)])
    setRelationDialog(null)
  }

  function startConnect(source: string, target: string) {
    if (!editable) return
    if (isGhost(source) || isGhost(target)) return
    setRelationDialog({ source, target })
  }

  function requestDelete(elId: string) {
    const el = liveTree.byId.get(elId)
    if (editable && el && !isGhost(elId)) setDeleteTarget(el)
  }

  const relationEnds = (r: Relation) => ({
    source: liveTree.byId.get(r.source_id)?.name ?? "unknown",
    target: liveTree.byId.get(r.target_id)?.name ?? "unknown",
  })

  // Layers belong to the view (not the model), so only whoever may edit the view can change them.
  const activeView = ws.viewModel?.view
  const mayEditView =
    activeView !== undefined &&
    viewLayers !== undefined &&
    (me.data?.role === "maintainer" || (me.data?.role === "editor" && activeView.created_by === me.data.id))

  function setLayer(elId: string, layer: string) {
    const el = liveTree.byId.get(elId)
    if (!editable || !el || isGhost(elId) || el.layer === layer) return
    const violated = layerViolation(layerRules, el, layer)
    if (violated) {
      toast.error(violationMessage(violated))
      return
    }
    add.mutate([{ op: "update", target_type: "element", target_id: elId, patch: { layer } }])
  }

  function saveLayers(next: string[], onDone?: () => void) {
    if (!activeView) return
    updateView.mutate(
      {
        id: activeView.id,
        body: {
          name: activeView.name,
          kind: "layers",
          description: activeView.description,
          scope_element_id: null,
          layers: next,
        },
      },
      { onSuccess: onDone }
    )
  }

  /** Rename keeps the elements on the layer: they get an update in this draft, then the view follows. */
  function renameLayer(from: string, to: string) {
    if (!viewLayers) return
    const next = renameLayerIn(viewLayers, from, to)
    const moved = onLayer(elements, from).filter((e) => !isGhost(e.id) && e.layer !== to)
    const done = () => setLayerDialog(null)
    if (moved.length === 0) {
      saveLayers(next, done)
      return
    }
    add.mutate(
      moved.map((e) => ({
        op: "update" as const,
        target_type: "element" as const,
        target_id: e.id,
        patch: { layer: to },
      })),
      { onSuccess: () => saveLayers(next, done) }
    )
  }

  const layerOps: DraftMenuDeps["layerOps"] =
    editable && mayEditView
      ? {
          add: () => setLayerDialog({}),
          rename: (name) => setLayerDialog({ renaming: name }),
          move: (name, delta) => saveLayers(shiftLayer(viewLayers, name, delta)),
          remove: (name) => setLayerDelete(name),
        }
      : undefined

  // Right-click menus (only while editing): wired to the same dialogs and mutations as the toolbar.
  const menuDeps: DraftMenuDeps = {
    element: (elId) => liveTree.byId.get(elId),
    relation: (elId) => relations.find((r) => r.id === elId),
    nameOf: (elId) => liveTree.byId.get(elId)?.name ?? "unknown",
    isGhost,
    addElement: (kind, parentId, layer) => setElementDialog({ kind, parentId, layer }),
    layers: viewLayers,
    layerRules,
    setLayer,
    layerOps,
    editElement: startEditing,
    deleteElement: requestDelete,
    startConnect: (elId) => useCanvasStore.getState().setConnectFrom(elId),
    editRelation: (r) => setRelationDialog({ relation: r }),
    deleteRelation: (r) => add.mutate([deleteRelationInput(r.id)]),
  }
  const menu = editable
    ? {
        node: (elId: string) => nodeEntries(menuDeps, elId),
        edge: (ids: readonly string[]) => edgeEntries(menuDeps, ids),
        pane: () => paneEntries(menuDeps),
        layer: (name: string, unassigned: boolean) => layerEntries(menuDeps, name, unassigned),
        stack: () => stackEntries(menuDeps),
      }
    : undefined

  const lintErrors = lint.data?.errors ?? 0
  const lintWarnings = lint.data?.warnings ?? 0
  const actions = workflowActions(me.data, cs)

  function runAction(action: WorkflowAction) {
    if (action === "submit") submit.mutate()
    else if (action === "approve") approve.mutate()
    else if (action === "reject") reject.mutate()
    else if (action === "abandon") abandon.mutate()
    else
      merge.mutate(undefined, {
        onSuccess: (r) => {
          if (!r.merged) {
            setTab("review")
            setReviewTab("conflicts")
          }
        },
      })
  }

  if (detail.isError) {
    return (
      <div className="flex h-svh flex-col">
        <AppTopBar />
        <Status
          tone="error"
          title="Could not load this changeset"
          onRetry={() => void detail.refetch()}
          action={
            <Button size="sm" variant="ghost" render={<Link to="/changesets" />}>
              Back to changesets
            </Button>
          }
        >
          It may not exist, or you may not be allowed to see it.
        </Status>
      </div>
    )
  }

  return (
    <div className="flex h-svh flex-col">
      <AppTopBar
        context={
          cs && (
            <>
              <span className="max-w-[16rem] truncate font-medium" title={cs.title}>
                {cs.title}
              </span>
              <StatusBadge status={cs.status} />
              <Badge variant="outline" className="hidden font-mono text-muted-foreground lg:inline-flex">
                base v{cs.base_version}
              </Badge>
              {diff.data?.stale && (
                <Badge variant="outline" className="border-amber-500/40 text-amber-700 dark:text-amber-400">
                  Main moved on
                </Badge>
              )}
              {!editable && (
                <Badge variant="secondary" className="hidden text-muted-foreground lg:inline-flex">
                  <Lock data-icon="inline-start" aria-hidden />
                  {cs.status === "draft" ? "Not your draft" : "Read-only"}
                </Badge>
              )}
            </>
          )
        }
      />

      {model.isSuccess && (
        <ViewTabs
          me={me.data}
          activeId={viewId}
          onSelect={onViewChange}
          model={model.data}
          activeModel={ws.viewModel}
          actions={
            /* Draft tools, lint status and the review workflow. */
            <div className="flex items-center gap-1.5" role="toolbar" aria-label="Draft actions">
              {editable && (
                <>
                  <DropdownMenu>
                    <DropdownMenuTrigger render={<Button size="sm" variant="outline" />}>
                      <Plus aria-hidden /> <span className="hidden lg:inline">Add</span>
                      <ChevronDown aria-hidden className="text-muted-foreground" />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuGroup>
                        <DropdownMenuLabel>Add element</DropdownMenuLabel>
                        {kinds.map((k) => {
                          const Icon = kindMeta[k].icon
                          return (
                            <DropdownMenuItem
                              key={k}
                              onClick={() => {
                                const sel = useCanvasStore.getState().selectedId
                                setElementDialog({ kind: k, parentId: sel && !isGhost(sel) ? sel : null })
                              }}
                            >
                              <Icon aria-hidden style={{ color: `var(--kind-${k})` }} /> {kindMeta[k].label}
                            </DropdownMenuItem>
                          )
                        })}
                      </DropdownMenuGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>

                  <div className="flex items-center">
                    <IconAction
                      label="Undo"
                      shortcut="Ctrl+Z"
                      disabled={!canUndo || busy}
                      onClick={() => undo.mutate()}
                    >
                      <Undo2 aria-hidden />
                    </IconAction>
                    <IconAction
                      label="Redo"
                      shortcut="Ctrl+Shift+Z"
                      disabled={!canRedo || busy}
                      onClick={() => redo.mutate()}
                    >
                      <Redo2 aria-hidden />
                    </IconAction>
                  </div>
                </>
              )}

              <button
                type="button"
                onClick={() => {
                  setTab("review")
                  setReviewTab("lint")
                }}
                className="flex h-7 items-center gap-2.5 rounded-md border px-2 text-xs tabular-nums transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                aria-label={`Lint: ${lintErrors} errors, ${lintWarnings} warnings. Show lint panel`}
                title="Lint: errors and warnings"
              >
                <span
                  className={cn(
                    "flex items-center gap-1",
                    lintErrors > 0 ? "text-destructive" : "text-muted-foreground"
                  )}
                >
                  <CircleAlert className="size-3.5" aria-hidden />
                  <NumberTicker value={lintErrors} className="text-current tabular-nums" />
                </span>
                <span
                  className={cn(
                    "flex items-center gap-1",
                    lintWarnings > 0 ? "text-amber-600 dark:text-amber-400" : "text-muted-foreground"
                  )}
                >
                  <TriangleAlert className="size-3.5" aria-hidden />
                  <NumberTicker value={lintWarnings} className="text-current tabular-nums" />
                </span>
              </button>

              <Toggle
                size="sm"
                variant="outline"
                pressed={showDiff}
                onPressedChange={setShowDiff}
                aria-label="Diff overlay"
                title="Highlight what this draft changes on the canvas"
              >
                <GitCompare aria-hidden /> <span className="hidden lg:inline">Diff</span>
              </Toggle>

              {actions.length > 0 && <span className="mx-0.5 h-5 w-px bg-border" aria-hidden />}
              {actions.map((a) => (
                <Button
                  key={a.action}
                  size="sm"
                  variant={
                    a.action === "merge" || a.action === "submit" || a.action === "approve" ? "default" : "ghost"
                  }
                  disabled={
                    !a.enabled ||
                    submit.isPending ||
                    approve.isPending ||
                    reject.isPending ||
                    abandon.isPending ||
                    merge.isPending
                  }
                  title={a.reason}
                  onClick={() => runAction(a.action)}
                >
                  {a.action === "merge" && <GitMerge aria-hidden />}
                  {actionLabel[a.action]}
                </Button>
              ))}
            </div>
          }
        />
      )}

      {model.isPending && <Status tone="loading">Loading the draft…</Status>}
      {viewId && ws.loading && <Status tone="loading">Loading the view…</Status>}
      {viewId && ws.error && (
        <Status tone="error" title="This view could not be loaded">
          It may have been deleted. Pick another one above.
        </Status>
      )}
      {viewId && ws.scopeMissing && (
        <Status title="Nothing to show">
          The element this view is scoped to does not exist in this draft. Edit the view to pick another.
        </Status>
      )}
      {model.isError && cs && (
        <Status
          title="No editable model"
          action={
            <Button size="sm" variant="outline" render={<Link to="/" />}>
              View main
            </Button>
          }
        >
          This changeset is {statusLabel[cs.status].toLowerCase()}.
        </Status>
      )}
      {model.isSuccess && !(viewId && (ws.loading || ws.error || ws.scopeMissing)) && (
        <ModelWorkspace
          viewKey={viewId}
          layers={viewLayers}
          elements={elements}
          relations={relations}
          decor={{ diff: overlay.elements, relationDiff: overlay.relations, lint: marks, editable }}
          onConnect={startConnect}
          onMoveToLayer={editable ? setLayer : undefined}
          onDeleteRequest={requestDelete}
          menu={menu}
          renderPanel={(tree) => (
            <Tabs value={tab} onValueChange={setTab} className="h-full min-h-0 gap-0">
              <TabsList variant="line" className="h-11! w-full shrink-0 justify-start gap-3 border-b px-3">
                <TabsTrigger value="details" className="flex-none">
                  Details
                </TabsTrigger>
                <TabsTrigger value="review" className="flex-none">
                  Review
                  {lintErrors + lintWarnings > 0 && (
                    <span className="rounded-full bg-muted px-1.5 text-[10px] tabular-nums">
                      {lintErrors + lintWarnings}
                    </span>
                  )}
                </TabsTrigger>
              </TabsList>
              <div className="min-h-0 flex-1 overflow-y-auto">
                <TabsContent value="details">
                  <PropertyPanel
                    tree={tree}
                    relations={relations}
                    actions={
                      editable
                        ? {
                            editingId,
                            onEdit: startEditing,
                            renderEditor: (e) => (
                              <ElementEditor
                                key={e.id}
                                element={e}
                                tree={liveTree}
                                deployedOn={hostIds(draftRelations, e.id)}
                                layerOptions={layerOptions}
                                projectOptions={projectOptions}
                                technologyOptions={technologyOptions}
                                layerRules={layerRules}
                                canProtect={caps.canProtect}
                                pending={add.isPending}
                                onSave={(values) => saveElement(e, values)}
                                onCancel={() => setEditingId(null)}
                              />
                            ),
                            onDelete: (e) => requestDelete(e.id),
                            onAddChild: (e) => !isGhost(e.id) && setElementDialog({ parentId: e.id }),
                            onEditRelation: (r) => setRelationDialog({ relation: r }),
                            onDeleteRelation: (r) => add.mutate([deleteRelationInput(r.id)]),
                          }
                        : undefined
                    }
                  />
                </TabsContent>
                <TabsContent value="review" className="h-full">
                  <ReviewPanel
                    tree={tree}
                    relations={relations}
                    lint={lint.data}
                    diff={diff.data}
                    conflicts={conflicts.data}
                    changes={changes}
                    canResolve={caps.canResolve}
                    resolving={resolve.isPending}
                    onResolve={(rs) => resolve.mutate(rs)}
                    tab={reviewTab}
                    onTabChange={setReviewTab}
                  />
                </TabsContent>
              </div>
            </Tabs>
          )}
        />
      )}

      <ElementDialog
        open={elementDialog !== null}
        onOpenChange={(o) => !o && setElementDialog(null)}
        tree={liveTree}
        defaultKind={elementDialog?.kind}
        defaultParentId={elementDialog?.parentId}
        defaultLayer={elementDialog?.layer}
        layerOptions={layerOptions}
        projectOptions={projectOptions}
        technologyOptions={technologyOptions}
        layerRules={layerRules}
        canProtect={caps.canProtect}
        pending={add.isPending}
        onSubmit={submitElement}
      />

      <RelationDialog
        open={relationDialog !== null}
        onOpenChange={(o) => !o && setRelationDialog(null)}
        sourceName={
          relationDialog === null
            ? ""
            : "relation" in relationDialog
              ? relationEnds(relationDialog.relation).source
              : (liveTree.byId.get(relationDialog.source)?.name ?? "")
        }
        targetName={
          relationDialog === null
            ? ""
            : "relation" in relationDialog
              ? relationEnds(relationDialog.relation).target
              : (liveTree.byId.get(relationDialog.target)?.name ?? "")
        }
        relation={relationDialog && "relation" in relationDialog ? relationDialog.relation : undefined}
        pending={add.isPending}
        onSubmit={submitRelation}
      />

      {viewLayers && (
        <LayerNameDialog
          open={layerDialog !== null}
          onOpenChange={(o) => !o && setLayerDialog(null)}
          layers={viewLayers}
          renaming={layerDialog?.renaming}
          pending={add.isPending || updateView.isPending}
          onSubmit={(name) =>
            layerDialog?.renaming
              ? renameLayer(layerDialog.renaming, name)
              : saveLayers([...viewLayers, name], () => setLayerDialog(null))
          }
        />
      )}

      <AlertDialog open={layerDelete !== null} onOpenChange={(o) => !o && setLayerDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete layer {layerDelete}?</AlertDialogTitle>
            <AlertDialogDescription>
              {layerDelete && onLayer(elements, layerDelete).length > 0
                ? `${onLayer(elements, layerDelete).length} element(s) keep their layer name but are shown as Unassigned in this view until they are moved. The model is not changed.`
                : "The layer is empty. The model is not changed."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (viewLayers && layerDelete) saveLayers(removeLayerFrom(viewLayers, layerDelete))
                setLayerDelete(null)
              }}
            >
              Delete layer
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={deleteTarget !== null} onOpenChange={(o) => !o && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {deleteTarget?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget && (
                <DeleteSummary element={deleteTarget} tree={liveTree} relations={model.data?.relations ?? []} />
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (!deleteTarget) return
                add.mutate([deleteElementInput(deleteTarget.id, descendantCount(liveTree, deleteTarget.id) > 0)])
                select(null)
                setDeleteTarget(null)
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

/** Icon-only toolbar button with a tooltip (label + keyboard shortcut). */
function IconAction({
  label,
  shortcut,
  disabled,
  onClick,
  children,
}: {
  label: string
  shortcut?: string
  disabled?: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={<Button size="icon-sm" variant="ghost" aria-label={label} disabled={disabled} onClick={onClick} />}
      >
        {children}
      </TooltipTrigger>
      <TooltipContent side="bottom">
        {label}
        {shortcut && <kbd className="font-mono text-[10px] opacity-60">{shortcut}</kbd>}
      </TooltipContent>
    </Tooltip>
  )
}

function DeleteSummary({
  element,
  tree,
  relations,
}: {
  element: Element
  tree: ReturnType<typeof buildTree>
  relations: readonly Relation[]
}) {
  const descendants = descendantCount(tree, element.id)
  const touching = relations.filter((r) => r.source_id === element.id || r.target_id === element.id).length
  return (
    <>
      This removes it from the draft only; main is unchanged until the draft is merged.
      {descendants > 0 && (
        <>
          {" "}
          It also deletes its <b>{descendants}</b> nested element{descendants === 1 ? "" : "s"}.
        </>
      )}
      {touching > 0 && (
        <>
          {" "}
          <b>{touching}</b> relation{touching === 1 ? "" : "s"} touching it will be deleted too.
        </>
      )}{" "}
      You can undo this.
    </>
  )
}
