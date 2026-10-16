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
import { ChevronsDownUp, ChevronsUpDown, Crosshair, Maximize, RotateCcw, Shrink, Expand } from "lucide-react"
import type { Element, Relation } from "@archmap/api-client"

import "@xyflow/react/dist/style.css"

import { Button } from "@/components/ui/button"
import { useColorMode } from "@/lib/use-color-mode"
import { AggregatedEdge } from "./AggregatedEdge"
import { CanvasContextMenu, joinGroups, type MenuEntry, type MenuPosition } from "./CanvasContextMenu"
import { ElementNode, type ElementFlowNode } from "./ElementNode"
import { expandableIds } from "./graph"
import { useCanvasStore } from "./store"
import type { Decor } from "./decor"
import { useCanvasGraph } from "./useCanvasGraph"

// Defined outside the component so React Flow does not re-register them on every render.
const nodeTypes = { element: ElementNode }
const edgeTypes = { aggregated: AggregatedEdge }

/** Semantic zoom: a collapsed node that is zoomed past this on-screen width expands in place. */
export const ZOOM_EXPAND_WIDTH_PX = 340

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
}

function Canvas({ elements, relations, decor, onConnect, onDeleteRequest, menu }: CanvasProps) {
  const { tree, nodes, edges, ready } = useCanvasGraph(elements, relations, decor)
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

  // After a bulk expand / collapse, refit once the layout animation has finished.
  const fitSoon = useCallback(() => {
    setTimeout(() => void rf.fitView({ padding: 0.2, duration: 300 }), 340)
  }, [rf])

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
        setExpanded(expandableIds(tree))
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
          if (node.data.childCount > 0) toggle(node.id)
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
          if (!onDeleteRequest || (e.key !== "Delete" && e.key !== "Backspace")) return
          if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return
          const id = useCanvasStore.getState().selectedId
          if (id) onDeleteRequest(id)
        }}
        deleteKeyCode={null}
        onMoveEnd={onMoveEnd}
        zoomOnDoubleClick={false}
        nodesConnectable={decor?.editable ?? false}
        minZoom={0.1}
        maxZoom={2}
        aria-label="Architecture model canvas"
      >
        <Background gap={24} />
        <Controls showInteractive={false} />
        <MiniMap
          pannable
          zoomable
          ariaLabel="Model overview"
          nodeColor={(n) => minimapColor((n as ElementFlowNode).data.element.kind)}
          className="hidden md:block"
        />
        {connectFrom && (
          <Panel
            position="top-center"
            className="rounded-full border bg-popover px-3 py-1.5 text-sm shadow-md"
            role="status"
          >
            Connecting from <b>{nodes.find((n) => n.id === connectFrom)?.data.element.name}</b>: click the target
            element · Esc to cancel
          </Panel>
        )}
        <Panel position="top-left" className="flex gap-1.5">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setExpanded(expandableIds(tree))
              fitSoon()
            }}
          >
            <ChevronsUpDown aria-hidden /> Expand all
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setExpanded([])
              fitSoon()
            }}
          >
            <ChevronsDownUp aria-hidden /> Collapse all
          </Button>
          <Button variant="outline" size="sm" onClick={() => void rf.fitView({ padding: 0.2, duration: 300 })}>
            <Maximize aria-hidden /> Fit
          </Button>
          <Button variant="outline" size="sm" onClick={resetPositions}>
            <RotateCcw aria-hidden /> Reset layout
          </Button>
        </Panel>
      </ReactFlow>
      <CanvasContextMenu position={menuPos} entries={menuEntries} onClose={() => setMenuPos(null)} />
    </>
  )
}

export function CanvasView(props: CanvasProps) {
  return (
    <ReactFlowProvider>
      <Canvas {...props} />
    </ReactFlowProvider>
  )
}
