import { createFileRoute } from "@tanstack/react-router"

import { DraftPage } from "@/features/draft/DraftPage"

export const Route = createFileRoute("/changesets/$id")({
  component: Page,
  validateSearch: (s: Record<string, unknown>): { view?: string; focus?: string } => ({
    view: typeof s.view === "string" ? s.view : undefined,
    focus: typeof s.focus === "string" ? s.focus : undefined,
  }),
})

function Page() {
  const { id } = Route.useParams()
  const { view, focus } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <DraftPage
      id={id}
      viewId={view}
      onViewChange={(v) => void navigate({ search: { view: v } })}
      focusId={focus}
      onFocusChange={(f) => void navigate({ search: (prev) => ({ ...prev, focus: f ?? undefined }) })}
    />
  )
}
