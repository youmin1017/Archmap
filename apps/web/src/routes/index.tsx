import { createFileRoute } from "@tanstack/react-router"
import { Lock } from "lucide-react"

import { AppTopBar } from "@/components/app-top-bar"
import { Badge } from "@/components/ui/badge"
import { PropertyPanel } from "@/features/canvas/PropertyPanel"
import { NewDraftButton } from "@/features/draft/NewDraftDialog"
import { useMainModel, useMainVersion, useMe } from "@/features/draft/queries"
import { ViewTabs } from "@/features/views/ViewTabs"
import { useViewWorkspace } from "@/features/views/useViewWorkspace"
import { ModelWorkspace, Status } from "@/features/workspace/ModelWorkspace"

export const Route = createFileRoute("/")({
  component: Landscape,
  validateSearch: (s: Record<string, unknown>): { view?: string } => ({
    view: typeof s.view === "string" ? s.view : undefined,
  }),
})

/** Main: the merged model, read-only. Edits start from a draft. */
function Landscape() {
  const versions = useMainVersion()
  const model = useMainModel()
  const me = useMe()
  const { view } = Route.useSearch()
  const navigate = Route.useNavigate()
  const version = versions.data?.[0]?.number ?? 0

  const ws = useViewWorkspace({
    viewId: view,
    fullModel: model.data,
    canSaveLayout: me.data?.role === "editor" || me.data?.role === "maintainer",
  })
  const elements = ws.elements ?? []
  const relations = ws.relations ?? []

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
            <Badge variant="secondary" className="text-muted-foreground">
              <Lock data-icon="inline-start" aria-hidden /> Read-only
            </Badge>
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
          actions={<NewDraftButton />}
        />
      )}

      {model.isPending && <Status tone="loading">Loading the model…</Status>}
      {model.isError && (
        <Status tone="error" title="Could not load the model" onRetry={() => void model.refetch()}>
          {String(model.error.message).slice(0, 240)}. Is the API running (`task dev-api`) with AUTH_DEV_HEADER=true?
        </Status>
      )}
      {model.isSuccess && model.data.elements.length === 0 && (
        <Status title="The model is empty" action={<NewDraftButton />}>
          Run `task seed` to load the sample model, or start a draft and add the first elements.
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
          renderPanel={(tree) => <PropertyPanel tree={tree} relations={relations} />}
        />
      )}
    </div>
  )
}
