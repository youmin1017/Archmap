import { useEffect, useRef } from "react"
import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useQueryClient } from "@tanstack/react-query"
import { Lock, Radio, Undo2 } from "lucide-react"

import { AppTopBar } from "@/components/app-top-bar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { PropertyPanel } from "@/features/canvas/PropertyPanel"
import { NewDraftButton } from "@/features/draft/NewDraftDialog"
import { keys, useApplyDirect, useMainModel, useMainVersion, useMe, useRevertDirect } from "@/features/draft/queries"
import { directSummary, undoTarget } from "@/features/editing/direct"
import { useModelEditing } from "@/features/editing/useModelEditing"
import { useSettings } from "@/features/settings/queries"
import { ViewTabs } from "@/features/views/ViewTabs"
import { useViewWorkspace } from "@/features/views/useViewWorkspace"
import { ModelWorkspace, Status } from "@/features/workspace/ModelWorkspace"

export const Route = createFileRoute("/")({
  component: Landscape,
  validateSearch: (s: Record<string, unknown>): { view?: string; focus?: string } => ({
    view: typeof s.view === "string" ? s.view : undefined,
    focus: typeof s.focus === "string" ? s.focus : undefined,
  }),
})

/** How often a live main looks for other people's edits (ADR-072). */
const POLL_MS = 15_000

/**
 * Main: the merged model. Read-only unless the review policy is "direct": then
 * editors edit it in place and every save is a new version (ADR-072).
 */
function Landscape() {
  const versions = useMainVersion()
  const model = useMainModel()
  const me = useMe()
  const settings = useSettings()
  const { view, focus } = Route.useSearch()
  const navigate = Route.useNavigate()
  const goTo = useNavigate()
  const qc = useQueryClient()
  const version = versions.data?.[0]?.number ?? 0

  const canEditRole = me.data?.role === "editor" || me.data?.role === "maintainer"
  const live = settings.data?.review_policy === "direct"
  const editable = live && canEditRole
  const ws = useViewWorkspace({ viewId: view, fullModel: model.data, canSaveLayout: canEditRole })
  const elements = ws.elements ?? []
  const relations = ws.relations ?? []

  const openDraft = (id: string) => void goTo({ to: "/changesets/$id", params: { id } })
  const apply = useApplyDirect(openDraft)
  const revert = useRevertDirect(openDraft)
  const nameOf = (id: string) => model.data?.elements.find((e) => e.id === id)?.name ?? "unknown"

  const editing = useModelEditing({
    target: "main",
    model: model.data,
    elements,
    relations,
    viewLayers: ws.viewModel?.view.kind === "layers" ? ws.viewModel.view.layers : undefined,
    activeView: ws.viewModel?.view,
    me: me.data,
    editable,
    canProtect: me.data?.role === "maintainer",
    submit: (inputs, onSuccess) =>
      apply.mutate({ inputs, summary: directSummary(inputs, nameOf) }, { onSuccess: () => onSuccess?.() }),
    pending: apply.isPending,
  })

  // Undo reverses my newest direct edit (a new version); the API refuses it if others changed the same things.
  const undoable = editable ? undoTarget(versions.data ?? [], me.data?.id) : undefined
  const busy = apply.isPending || revert.isPending
  useEffect(() => {
    if (!editable) return
    const onKey = (e: KeyboardEvent) => {
      if (!(e.ctrlKey || e.metaKey) || e.shiftKey || e.key.toLowerCase() !== "z") return
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return
      if (!undoable || busy) return
      e.preventDefault()
      revert.mutate(undoable.changeset_id)
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [editable, undoable, busy, revert])

  // Live main: look for other people's versions now and then (and on focus), and reload main when one lands.
  const { refetch: refetchVersions } = versions
  useEffect(() => {
    if (!live) return
    const timer = setInterval(() => void refetchVersions(), POLL_MS)
    return () => clearInterval(timer)
  }, [live, refetchVersions])
  const seenVersion = useRef(version)
  useEffect(() => {
    if (version > seenVersion.current && seenVersion.current > 0) void qc.invalidateQueries({ queryKey: keys.main })
    seenVersion.current = version
  }, [version, qc])

  const toolbar = (
    <div className="flex items-center gap-1.5" role="toolbar" aria-label="Main actions">
      {editable && (
        <>
          {editing.addMenu}
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  size="icon-sm"
                  variant="ghost"
                  aria-label={undoable ? `Undo “${undoable.summary}”` : "Undo"}
                  disabled={!undoable || busy}
                  onClick={() => undoable && revert.mutate(undoable.changeset_id)}
                />
              }
            >
              <Undo2 aria-hidden />
            </TooltipTrigger>
            <TooltipContent side="bottom">
              {undoable ? `Undo “${undoable.summary}”` : "Nothing of yours to undo"}
              <kbd className="font-mono text-[10px] opacity-60">Ctrl+Z</kbd>
            </TooltipContent>
          </Tooltip>
          <span className="mx-0.5 h-5 w-px bg-border" aria-hidden />
        </>
      )}
      <NewDraftButton variant={editable ? "outline" : "default"} />
    </div>
  )

  return (
    <div className="flex h-svh flex-col">
      <AppTopBar
        context={
          <>
            <span className="font-medium">Main</span>
            {version > 0 && (
              <Badge variant="outline" className="font-mono">
                v{version}
              </Badge>
            )}
            {editable ? (
              <Badge variant="secondary" title="Review policy is direct: every save is a new version">
                <Radio data-icon="inline-start" aria-hidden /> Live
              </Badge>
            ) : (
              <Badge variant="secondary" className="text-muted-foreground">
                <Lock data-icon="inline-start" aria-hidden /> Read-only
              </Badge>
            )}
          </>
        }
      />

      {model.isSuccess && model.data.elements.length > 0 && (
        <ViewTabs
          me={me.data}
          activeId={view}
          onSelect={(id) => void navigate({ search: { view: id } })}
          model={model.data}
          activeModel={ws.viewModel}
          actions={toolbar}
        />
      )}

      {model.isPending && <Status tone="loading">Loading the model…</Status>}
      {model.isError && (
        <Status tone="error" title="Could not load the model" onRetry={() => void model.refetch()}>
          {String(model.error.message).slice(0, 240)}. Is the API running (`task dev-api`)?
        </Status>
      )}
      {model.isSuccess && model.data.elements.length === 0 && (
        <Status title="The model is empty" action={editable ? editing.addMenu : <NewDraftButton />}>
          {editable
            ? "Add the first elements; each save is a new version."
            : "Run `task seed` to load the sample model, or start a draft and add the first elements."}
        </Status>
      )}
      {view && ws.loading && <Status tone="loading">Loading the view…</Status>}
      {view && ws.error && (
        <Status tone="error" title="This view could not be loaded">
          It may have been deleted. Pick another one above.
        </Status>
      )}
      {view && ws.scopeMissing && (
        <Status title="Nothing to show">
          The element this view is scoped to does not exist in this model. Edit the view to pick another.
        </Status>
      )}
      {model.isSuccess && model.data.elements.length > 0 && !ws.loading && !ws.error && !ws.scopeMissing && (
        <ModelWorkspace
          viewKey={view}
          layers={ws.viewModel?.view.kind === "layers" ? ws.viewModel.view.layers : undefined}
          elements={elements}
          relations={relations}
          owners={model.data.owners}
          decor={{ editable }}
          onConnect={editing.onConnect}
          onMoveToLayer={editing.onMoveToLayer}
          onDeleteRequest={editing.onDeleteRequest}
          menu={editing.menu}
          renderPanel={(tree) => <PropertyPanel tree={tree} relations={relations} actions={editing.panelActions} />}
          focusId={focus}
          onFocusChange={(id) => void navigate({ search: (prev) => ({ ...prev, focus: id ?? undefined }) })}
          canSaveFocusView={canEditRole}
        />
      )}
      {editing.dialogs}
    </div>
  )
}
