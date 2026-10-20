import { useEffect, useMemo, useState } from "react"
import { Link } from "@tanstack/react-router"
import { CircleAlert, GitCompare, GitMerge, Lock, Redo2, TriangleAlert, Undo2 } from "lucide-react"

import { AppTopBar } from "@/components/app-top-bar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { NumberTicker } from "@/components/ui/number-ticker"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Toggle } from "@/components/ui/toggle"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { PropertyPanel } from "../canvas/PropertyPanel"
import { buildTree } from "../canvas/graph"
import { useModelEditing } from "../editing/useModelEditing"
import { ModelWorkspace, Status } from "../workspace/ModelWorkspace"
import { diffOverlay, lintMarks } from "./decor"
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
  useUndo,
} from "./queries"
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

export function DraftPage({
  id,
  viewId,
  onViewChange,
  focusId,
  onFocusChange,
}: {
  id: string
  /** Selected view (URL search param); undefined = whole model. */
  viewId?: string
  onViewChange: (viewId: string | undefined) => void
  /** Element the canvas is focused on (URL search param, ADR-071). */
  focusId?: string
  onFocusChange?: (id: string | null) => void
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

  const [showDiff, setShowDiff] = useState(true)
  const [tab, setTab] = useState("details")
  const [reviewTab, setReviewTab] = useState("lint")
  // Review names come from the whole draft (and what it removed), not from the view's subset.
  const reviewTree = useMemo(
    () => buildTree([...(model.data?.elements ?? []), ...(diff.data?.elements.removed ?? [])]),
    [model.data, diff.data]
  )
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

  const isGhost = (elId: string) => overlay.elements.get(elId) === "removed"
  const editing = useModelEditing({
    target: "draft",
    model: model.data,
    elements,
    relations,
    viewLayers,
    activeView: ws.viewModel?.view,
    me: me.data,
    editable,
    canProtect: caps.canProtect,
    isGhost,
    submit: (inputs, onSuccess) => add.mutate(inputs, { onSuccess }),
    pending: add.isPending,
    onEditStart: () => setTab("details"),
  })

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
                  {editing.addMenu}

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
          focusId={focusId}
          onFocusChange={onFocusChange}
          canSaveFocusView={me.data?.role === "editor" || me.data?.role === "maintainer"}
          viewKey={viewId}
          layers={viewLayers}
          elements={elements}
          relations={relations}
          owners={model.data.owners}
          decor={{ diff: overlay.elements, relationDiff: overlay.relations, lint: marks, editable }}
          onConnect={editing.onConnect}
          onMoveToLayer={editing.onMoveToLayer}
          onDeleteRequest={editing.onDeleteRequest}
          menu={editing.menu}
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
              <div className="relative min-h-0 flex-1 overflow-y-auto">
                <TabsContent value="details">
                  <PropertyPanel tree={tree} relations={relations} actions={editing.panelActions} />
                </TabsContent>
                <TabsContent value="review" className="h-full">
                  <ReviewPanel
                    tree={reviewTree}
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

      {editing.dialogs}
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
