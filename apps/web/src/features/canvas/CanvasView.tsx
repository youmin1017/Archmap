import { useCallback, useEffect, useRef, useState } from "react"
import {
  Background,
  Controls,
  MiniMap,
  Panel,
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  useStoreApi,
  type Connection,
  type FinalConnectionState,
  type NodeChange,
  type OnMove,
} from "@xyflow/react"
import {
  BookmarkPlus,
  ChevronsDownUp,
  ChevronsUpDown,
  Crosshair,
  Expand,
  Focus,
  Maximize,
  RotateCcw,
  Shrink,
} from "lucide-react"
import type { Element, Relation } from "@archmap/api-client"

import "@xyflow/react/dist/style.css"

import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useColorMode } from "@/lib/use-color-mode"
import { cn } from "@/lib/utils"
import { AggregatedEdge } from "./AggregatedEdge"
import { CanvasContextMenu, joinGroups, type MenuEntry, type MenuPosition } from "./CanvasContextMenu"
import { ElementNode, type ElementFlowNode } from "./ElementNode"
import { expandableIds, pathTo, visibleElements, type ElementTree } from "./graph"
import { useCanvasStore } from "./store"
import type { Decor } from "./decor"
import { useCanvasGraph } from "./useCanvasGraph"

// Defined outside the component so React Flow does not re-register them on every render.
const nodeTypes = { element: ElementNode }
const edgeTypes = { aggregated: AggregatedEdge }

/** Semantic zoom: a collapsed node that is zoomed past this on-screen width expands in place. */
export const ZOOM_EXPAND_WIDTH_PX = 340
/** Above this many elements "Expand all" opens one more level instead of everything. */
export const EXPAND_ALL_LIMIT = 300

/** Ids to expand for "Expand all": everything, or one more level for large models. */
export function expandAllIds(tree: ElementTree, expanded: ReadonlySet<string>): string[] {
  if (tree.byId.size <= EXPAND_ALL_LIMIT) return expandableIds(tree)
  const shown = visibleElements(tree, expanded)
  return [...new Set([...expanded, ...shown.filter((e) => tree.children.has(e.id)).map((e) => e.id)])]
}

/** Element kinds a focus can be saved as a view for, and the view kind (ADR-071). */
export const focusViewKind = { system: "container", container: "component" } as const

// SVG fill attributes cannot use var(); resolve the kind's CSS variable to a concrete color.
function minimapColor(kind: string) {
  return getComputedStyle(document.documentElement).getPropertyValue(`--kind-${kind}`).trim() || "#888"
}

type CanvasProps = {
  elements: readonly Element[]
  relations: readonly Relation[]
  decor?: Decor
  /** Called when the user drags a connection between two visible nodes (draft editing only). */
  onConnect?: (sourceId: string, targetId: string) => void
  /** Called for the Delete / Backspace key on a selected element (draft editing only). */
  onDeleteRequest?: (id: string) => void
  /** Extra right-click menu entries (draft editing). View entries (expand, fit, ...) are always present. */
  menu?: {
    node?: (id: string) => MenuEntry[]
    edge?: (relationIds: readonly string[]) => MenuEntry[]
    pane?: () => MenuEntry[]
    /** Right-click on a layer plane (layered views only). */
    layer?: (name: string, unassigned: boolean) => MenuEntry[]
    /** Right-click on the empty area of a layers view. */
    stack?: () => MenuEntry[]
  }
  /** Drill-down (ADR-071): focus an element (null = whole canvas). Without it, double-click expands. */
  onFocus?: (id: string | null) => void
  /** Offer "Save as view" for the focused element (editors on pages with view tabs). */
  canSaveFocusView?: boolean
}

function Canvas({
  elements,
  relations,
  decor,
  onConnect,
  onDeleteRequest,
  menu,
  onFocus,
  canSaveFocusView,
}: CanvasProps) {
  const { tree, fullTree, focusId, nodes, edges, ready, boxes } = useCanvasGraph(elements, relations, decor)
  const rf = useReactFlow()
  const flowStore = useStoreApi()
  const colorMode = useColorMode()

  const select = useCanvasStore((s) => s.select)
  const selectRelations = useCanvasStore((s) => s.selectRelations)
  const toggle = useCanvasStore((s) => s.toggle)
  const expand = useCanvasStore((s) => s.expand)
  const setExpanded = useCanvasStore((s) => s.setExpanded)
  const setPosition = useCanvasStore((s) => s.setPosition)
  const resetPositions = useCanvasStore((s) => s.resetPositions)
  const revealToken = useCanvasStore((s) => s.revealToken)
  const requestView = useCanvasStore((s) => s.requestView)
  const expandAll = () => setExpanded(expandAllIds(tree, useCanvasStore.getState().expanded))

  // Fit the first layout into view once.
  const fitted = useRef(false)
  useEffect(() => {
    if (ready && !fitted.current) {
      fitted.current = true
      void rf.fitView({ padding: 0.2 })
    }
  }, [ready, rf])

  // "Reveal in canvas" (from the tree): wait for the expand animation, then center on the element.
  const handledReveal = useRef(revealToken)
  useEffect(() => {
    if (revealToken === handledReveal.current) return
    handledReveal.current = revealToken
    const id = useCanvasStore.getState().selectedId
    if (!id) return
    const timer = setTimeout(() => void rf.fitView({ nodes: [{ id }], duration: 400, maxZoom: 1.1, padding: 0.5 }), 340)
    return () => clearTimeout(timer)
  }, [revealToken, rf])

  // After a bulk expand / collapse or a focus change, refit once the new layout (computed
  // in a worker, so its timing varies) has arrived and its animation has finished.
  // Time of the pending request; one that got no new layout within 3 s (nothing changed) lapses.
  const fitRequested = useRef(0)
  const fitSoon = useCallback(() => {
    fitRequested.current = Date.now()
  }, [])
  useEffect(() => {
    if (Date.now() - fitRequested.current > 3000 || boxes.size === 0) return
    fitRequested.current = 0
    const timer = setTimeout(() => void rf.fitView({ padding: 0.2, duration: 300 }), 320)
    return () => clearTimeout(timer)
  }, [boxes, rf])

  // Entering, switching or leaving a focus shows a different graph: refit it.
  const shownFocus = useRef(focusId)
  useEffect(() => {
    if (shownFocus.current === focusId) return
    shownFocus.current = focusId
    fitSoon()
  }, [focusId, fitSoon])

  // A double click drills into an element with children; on the focused element itself it does nothing.
  const canFocus = (id: string) => onFocus !== undefined && id !== focusId && fullTree.children.has(id)
  const focusUp = () => {
    if (!onFocus || !focusId) return
    const parent = fullTree.byId.get(focusId)?.parent_id
    onFocus(parent && fullTree.byId.has(parent) ? parent : null)
  }
  const focusPath = focusId ? pathTo(fullTree, focusId) : []
  const focused = focusId ? fullTree.byId.get(focusId) : undefined
  const savableKind = focused !== undefined && focused.kind in focusViewKind

  // Controlled nodes: apply only what the model-independent UI state cares about.
  const onNodesChange = useCallback(
    (changes: NodeChange<ElementFlowNode>[]) => {
      for (const c of changes) {
        // Dragging writes layout only. It never touches the model.
        if (c.type === "position" && c.position) setPosition(c.id, c.position)
        else if (c.type === "select" && c.selected) select(c.id)
      }
    },
    [select, setPosition]
  )

  const onMoveEnd: OnMove = useCallback(
    (_event, viewport) => {
      const { width, height, nodeLookup } = flowStore.getState()
      const toExpand: string[] = []
      for (const n of nodes) {
        if (n.data.childCount === 0 || n.data.expanded) continue
        const internal = nodeLookup.get(n.id)
        if (!internal) continue
        const w = (internal.measured.width ?? 0) * viewport.zoom
        const h = (internal.measured.height ?? 0) * viewport.zoom
        const sx = internal.internals.positionAbsolute.x * viewport.zoom + viewport.x
        const sy = internal.internals.positionAbsolute.y * viewport.zoom + viewport.y
        const onScreen = sx < width && sy < height && sx + w > 0 && sy + h > 0
        if (onScreen && w >= ZOOM_EXPAND_WIDTH_PX) toExpand.push(n.id)
      }
      if (toExpand.length > 0) expand(toExpand)
    },
    [flowStore, nodes, expand]
  )

  // Right-click menu: element / relation / background, each with view entries plus the draft's edit entries.
  const [menuPos, setMenuPos] = useState<MenuPosition | null>(null)
  const [menuEntries, setMenuEntries] = useState<MenuEntry[]>([])
  const connectFrom = useCanvasStore((s) => s.connectFrom)
  const setConnectFrom = useCanvasStore((s) => s.setConnectFrom)

  const openMenu = (e: { clientX: number; clientY: number; preventDefault: () => void }, entries: MenuEntry[]) => {
    e.preventDefault()
    setMenuEntries(entries)
    setMenuPos({ x: e.clientX, y: e.clientY })
  }

  const viewEntriesForNode = (id: string): MenuEntry[] => {
    const node = nodes.find((n) => n.id === id)
    const out: MenuEntry[] = []
    if (canFocus(id)) out.push({ label: "Focus", icon: Focus, onSelect: () => onFocus?.(id) })
    if (node && node.data.childCount > 0) {
      out.push({
        label: node.data.expanded ? "Collapse" : "Expand",
        icon: node.data.expanded ? Shrink : Expand,
        onSelect: () => toggle(id),
      })
    }
    out.push({
      label: "Zoom to element",
      icon: Crosshair,
      onSelect: () => void rf.fitView({ nodes: [{ id }], duration: 400, maxZoom: 1.2, padding: 0.5 }),
    })
    return out
  }

  const viewEntriesForPane = (): MenuEntry[] => [
    {
      label: "Expand all",
      icon: ChevronsUpDown,
      onSelect: () => {
        expandAll()
        fitSoon()
      },
    },
    {
      label: "Collapse all",
      icon: ChevronsDownUp,
      onSelect: () => {
        setExpanded([])
        fitSoon()
      },
    },
    { label: "Fit view", icon: Maximize, onSelect: () => void rf.fitView({ padding: 0.2, duration: 300 }) },
    { label: "Reset layout", icon: RotateCcw, onSelect: resetPositions },
  ]

  // Connect mode (menu "Connect to…"): Escape cancels, the next element click completes it.
  useEffect(() => {
    if (!connectFrom) return
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setConnectFrom(null)
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [connectFrom, setConnectFrom])

  return (
    <>
      <ReactFlow<ElementFlowNode>
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        colorMode={colorMode}
        onNodesChange={onNodesChange}
        className={connectFrom ? "[&_.react-flow__node]:cursor-crosshair" : undefined}
        onNodeClick={(_, node) => {
          if (connectFrom && node.id !== connectFrom) {
            onConnect?.(connectFrom, node.id)
            setConnectFrom(null)
            return
          }
          select(node.id)
        }}
        onNodeContextMenu={(e, node) => {
          select(node.id)
          openMenu(e, joinGroups(menu?.node?.(node.id) ?? [], viewEntriesForNode(node.id)))
        }}
        onEdgeContextMenu={(e, edge) => {
          const ids = (edge.data as { relationIds?: string[] } | undefined)?.relationIds ?? []
          selectRelations(ids)
          openMenu(e, joinGroups(menu?.edge?.(ids) ?? []))
        }}
        onPaneContextMenu={(e) => openMenu(e, joinGroups(menu?.pane?.() ?? [], viewEntriesForPane()))}
        onNodeDoubleClick={(_, node) => {
          if (onFocus) {
            if (canFocus(node.id)) onFocus(node.id)
          } else if (node.data.childCount > 0) toggle(node.id)
        }}
        onEdgeClick={(_, edge) =>
          selectRelations((edge.data as { relationIds?: string[] } | undefined)?.relationIds ?? [])
        }
        onPaneClick={() => select(null)}
        onConnect={(c: Connection) => onConnect?.(c.source, c.target)}
        onConnectEnd={(event, state: FinalConnectionState) => {
          // Dropping on a node body (not exactly on a handle) also connects.
          if (state.isValid || !state.fromNode) return
          // The connection line overlay swallows the event target, so look at what is under the pointer.
          const point = "changedTouches" in event ? event.changedTouches[0] : event
          const nodeEl = document
            .elementsFromPoint(point.clientX, point.clientY)
            .find((e) => e.closest(".react-flow__node"))
            ?.closest(".react-flow__node")
          const otherId = nodeEl?.getAttribute("data-id")
          if (!otherId || otherId === state.fromNode.id) return
          if (state.fromHandle.type === "target") onConnect?.(otherId, state.fromNode.id)
          else onConnect?.(state.fromNode.id, otherId)
        }}
        onKeyDown={(e) => {
          if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return
          const id = useCanvasStore.getState().selectedId
          // F focuses the selected element, Escape goes up one level (unless connecting).
          if (e.key === "f" && !e.ctrlKey && !e.metaKey && !e.altKey && id && canFocus(id)) {
            e.preventDefault()
            onFocus?.(id)
            return
          }
          if (e.key === "Escape" && focusId && !connectFrom) {
            focusUp()
            return
          }
          if (!onDeleteRequest || (e.key !== "Delete" && e.key !== "Backspace")) return
          if (id) onDeleteRequest(id)
        }}
        onlyRenderVisibleElements
        deleteKeyCode={null}
        onMoveEnd={onMoveEnd}
        zoomOnDoubleClick={false}
        nodesConnectable={decor?.editable ?? false}
        minZoom={0.1}
        maxZoom={2}
        aria-label="Architecture model canvas"
      >
        <Background gap={20} size={1.2} />
        <Controls showInteractive={false} />
        <MiniMap
          pannable
          zoomable
          ariaLabel="Model overview"
          nodeColor={(n) => minimapColor((n as ElementFlowNode).data.element.kind)}
          className="hidden md:block"
          style={{ width: 168, height: 112 }}
        />
        {connectFrom && (
          <Panel
            position="top-center"
            className="rounded-full border border-primary/30 bg-popover px-3 py-1.5 text-sm shadow-island"
            role="status"
          >
            Connecting from <b>{nodes.find((n) => n.id === connectFrom)?.data.element.name}</b>: click the target
            element · Esc to cancel
          </Panel>
        )}
        <Panel
          position="top-left"
          className="flex items-center gap-0.5 rounded-lg border bg-card/90 p-0.5 shadow-island backdrop-blur"
          role="toolbar"
          aria-label="Canvas"
        >
          <ToolButton
            label={tree.byId.size > EXPAND_ALL_LIMIT ? "Expand one more level" : "Expand all"}
            onClick={() => {
              expandAll()
              fitSoon()
            }}
          >
            <ChevronsUpDown aria-hidden />
          </ToolButton>
          <ToolButton
            label="Collapse all"
            onClick={() => {
              setExpanded([])
              fitSoon()
            }}
          >
            <ChevronsDownUp aria-hidden />
          </ToolButton>
          <span className="mx-0.5 h-4 w-px bg-border" aria-hidden />
          <ToolButton label="Fit view" onClick={() => void rf.fitView({ padding: 0.2, duration: 300 })}>
            <Maximize aria-hidden />
          </ToolButton>
          <ToolButton label="Reset layout" onClick={resetPositions}>
            <RotateCcw aria-hidden />
          </ToolButton>
          {focusId && onFocus && (
            <>
              <span className="mx-0.5 h-4 w-px bg-border" aria-hidden />
              <FocusBreadcrumb path={focusPath} onFocus={onFocus} />
              {canSaveFocusView && savableKind && (
                <ToolButton label="Save as view" onClick={() => requestView(focusId)}>
                  <BookmarkPlus aria-hidden />
                </ToolButton>
              )}
            </>
          )}
        </Panel>
      </ReactFlow>
      <CanvasContextMenu position={menuPos} entries={menuEntries} onClose={() => setMenuPos(null)} />
    </>
  )
}

const crumb =
  "max-w-40 truncate rounded px-1.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"

/**
 * Where a focused canvas is: Whole model / ancestors / focused element. Deep paths
 * keep the last two levels; the ones above are in a menu.
 */
function FocusBreadcrumb({
  path,
  onFocus,
}: {
  path: readonly { id: string; name: string }[]
  onFocus: (id: string | null) => void
}) {
  const shown = path.length > 3 ? path.slice(-2) : path
  const hidden = path.length > 3 ? path.slice(0, -2) : []
  const sep = (
    <span className="text-xs text-muted-foreground/60" aria-hidden>
      /
    </span>
  )
  return (
    <nav aria-label="Focus" className="flex min-w-0 items-center gap-0.5">
      <button type="button" className={crumb} onClick={() => onFocus(null)}>
        Whole model
      </button>
      {hidden.length > 0 && (
        <>
          {sep}
          <DropdownMenu>
            <DropdownMenuTrigger render={<button type="button" className={crumb} aria-label="More levels" />}>
              …
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              {hidden.map((e) => (
                <DropdownMenuItem key={e.id} onClick={() => onFocus(e.id)}>
                  {e.name}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </>
      )}
      {shown.map((e, i) => {
        const last = i === shown.length - 1
        return (
          <span key={e.id} className="flex min-w-0 items-center gap-0.5">
            {sep}
            <button
              type="button"
              className={cn(crumb, last && "font-medium text-foreground")}
              aria-current={last ? "location" : undefined}
              onClick={() => !last && onFocus(e.id)}
            >
              {e.name}
            </button>
          </span>
        )
      })}
    </nav>
  )
}

/** Icon button of the floating canvas toolbar; the label shows as a tooltip. */
function ToolButton({ label, onClick, children }: { label: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            onClick={onClick}
            className="text-muted-foreground hover:text-foreground"
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent side="bottom">{label}</TooltipContent>
    </Tooltip>
  )
}

export function CanvasView(props: CanvasProps) {
  return (
    <ReactFlowProvider>
      <Canvas {...props} />
    </ReactFlowProvider>
  )
}
