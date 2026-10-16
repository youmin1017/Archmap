import { useMemo, type ComponentProps } from "react"
import type { Element, Relation } from "@archmap/api-client"

import { StatePanel } from "@/components/state-panel"
import { CanvasView } from "../canvas/CanvasView"
import type { Decor } from "../canvas/decor"
import { ElementTreePanel } from "../canvas/ElementTreePanel"
import { buildTree, type ElementTree } from "../canvas/graph"
import { LayeredCanvas } from "../layers/LayeredCanvas"

type Props = {
  elements: readonly Element[]
  relations: readonly Relation[]
  decor?: Decor
  onConnect?: (sourceId: string, targetId: string) => void
  onDeleteRequest?: (id: string) => void
  /** Drag & drop of an element onto another layer (layers views, editable drafts only). */
  onMoveToLayer?: (id: string, layer: string) => void
  menu?: ComponentProps<typeof CanvasView>["menu"]
  /** Changing it remounts the canvas (used when switching views). */
  viewKey?: string
  /** Layer names (top to bottom) of a layers view: shows the stacked layer plans instead of the 2D canvas. */
  layers?: readonly string[]
  /** Right-hand panel; receives the tree so it can resolve names. */
  renderPanel: (tree: ElementTree) => React.ReactNode
}

/** Element tree | canvas | side panel. Shared by main (read-only) and draft (editable) pages. */
export function ModelWorkspace({
  elements,
  relations,
  decor,
  onConnect,
  onDeleteRequest,
  onMoveToLayer,
  menu,
  viewKey,
  layers,
  renderPanel,
}: Props) {
  const tree = useMemo(() => buildTree(elements), [elements])
  return (
    <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[280px_minmax(0,1fr)] xl:grid-cols-[280px_minmax(0,1fr)_360px]">
      <aside className="hidden min-h-0 border-r bg-sidebar text-sidebar-foreground md:block" aria-label="Elements">
        <ElementTreePanel tree={tree} elements={elements} />
      </aside>
      <main className="min-h-0 min-w-0">
        {layers ? (
          <LayeredCanvas
            key={viewKey ?? "layers"}
            elements={elements}
            relations={relations}
            layers={layers}
            decor={decor}
            onConnect={onConnect}
            onMoveToLayer={onMoveToLayer}
            menu={menu}
          />
        ) : (
          <CanvasView
            key={viewKey ?? "all"}
            elements={elements}
            relations={relations}
            decor={decor}
            onConnect={onConnect}
            onDeleteRequest={onDeleteRequest}
            menu={menu}
          />
        )}
      </main>
      <aside
        className="hidden min-h-0 overflow-y-auto border-l bg-sidebar text-sidebar-foreground xl:block"
        aria-label="Details"
      >
        {renderPanel(tree)}
      </aside>
    </div>
  )
}

export const Status = StatePanel
