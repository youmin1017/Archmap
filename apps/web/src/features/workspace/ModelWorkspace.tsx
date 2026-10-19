import { useEffect, useMemo, useState, type ComponentProps } from "react"
import { PanelLeft, PanelRight, X } from "lucide-react"
import type { Element, Relation } from "@archmap/api-client"

import { StatePanel } from "@/components/state-panel"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { CanvasView } from "../canvas/CanvasView"
import type { Decor } from "../canvas/decor"
import { ElementTreePanel } from "../canvas/ElementTreePanel"
import { buildTree, type ElementTree } from "../canvas/graph"
import { LayeredCanvas } from "../layers/LayeredCanvas"
import { useCanvasStore } from "../canvas/store"

// Below these widths the side panels leave the grid and open as drawers over the canvas.
const TREE_INLINE = "(min-width: 768px)"
const PANEL_INLINE = "(min-width: 1024px)"
const matches = (query: string) => typeof window !== "undefined" && window.matchMedia(query).matches

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
  const [treeOpen, setTreeOpen] = useState(false)
  const [panelOpen, setPanelOpen] = useState(false)

  // On narrow screens, selecting something opens the details drawer (the panel is hidden otherwise).
  const selectedId = useCanvasStore((s) => s.selectedId)
  const selectedRelations = useCanvasStore((s) => s.selectedRelationIds.length)
  useEffect(() => {
    if ((selectedId || selectedRelations > 0) && !matches(PANEL_INLINE)) setPanelOpen(true)
  }, [selectedId, selectedRelations])
  // Picking an element in the tree drawer reveals it on the canvas, so get the drawer out of the way.
  useEffect(() => {
    if (selectedId && !matches(TREE_INLINE)) setTreeOpen(false)
  }, [selectedId])

  const drawer = "absolute inset-y-0 z-30 shadow-xl"
  return (
    <div className="relative grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[248px_minmax(0,1fr)] lg:grid-cols-[248px_minmax(0,1fr)_300px] xl:grid-cols-[264px_minmax(0,1fr)_340px]">
      <aside
        data-open={treeOpen}
        className={cn(
          "min-h-0 w-[min(280px,85vw)] border-r bg-sidebar text-sidebar-foreground md:static md:block md:w-auto md:shadow-none",
          drawer,
          "left-0 max-md:data-[open=false]:hidden"
        )}
        aria-label="Elements"
      >
        <ElementTreePanel tree={tree} elements={elements} />
      </aside>
      <main className="relative min-h-0 min-w-0 bg-[radial-gradient(ellipse_at_top,color-mix(in_oklch,var(--primary)_4%,transparent),transparent_60%)]">
        {/* Drawer toggles, only where the panels are not shown inline. */}
        <div className="absolute top-3 right-3 z-20 flex gap-1 lg:hidden">
          <Button
            variant="outline"
            size="icon-sm"
            className="bg-card/90 shadow-island backdrop-blur md:hidden"
            aria-label="Show elements"
            aria-expanded={treeOpen}
            onClick={() => setTreeOpen((o) => !o)}
          >
            <PanelLeft aria-hidden />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            className="bg-card/90 shadow-island backdrop-blur"
            aria-label="Show details"
            aria-expanded={panelOpen}
            onClick={() => setPanelOpen((o) => !o)}
          >
            <PanelRight aria-hidden />
          </Button>
        </div>
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
        data-open={panelOpen}
        className={cn(
          "min-h-0 w-[min(340px,90vw)] overflow-y-auto border-l bg-sidebar text-sidebar-foreground lg:static lg:block lg:w-auto lg:shadow-none",
          drawer,
          "right-0 max-lg:data-[open=false]:hidden"
        )}
        aria-label="Details"
      >
        <Button
          variant="ghost"
          size="icon-sm"
          className="absolute top-2 right-2 z-10 lg:hidden"
          aria-label="Close details"
          onClick={() => setPanelOpen(false)}
        >
          <X aria-hidden />
        </Button>
        {renderPanel(tree)}
      </aside>
    </div>
  )
}

export const Status = StatePanel
