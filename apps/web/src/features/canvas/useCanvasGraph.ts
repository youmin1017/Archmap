import { useEffect, useMemo, useState } from "react"
import { MarkerType } from "@xyflow/react"
import { useReducedMotion } from "motion/react"
import type { Element, Relation } from "@archmap/api-client"

import { useAnimatedBoxes } from "./animate"
import type { AggregatedFlowEdge } from "./AggregatedEdge"
import type { ElementFlowNode } from "./ElementNode"
import { hostNameIndex } from "./deployment"
import { aggregateEdges, buildTree, childrenOf, filterMatches, isFilterActive, visibleElements } from "./graph"
import { computeLayout, type Box } from "./layout"
import { edgeDiffStatus, type Decor } from "./decor"
import { useCanvasStore } from "./store"

type Layout = { boxes: ReadonlyMap<string, Box>; entering: ReadonlySet<string> }
const noHosts: readonly string[] = []
const noLayout: Layout = { boxes: new Map(), entering: new Set() }

/**
 * Turns the model (elements + relations) and the ephemeral canvas state
 * (expanded set, drags, filter, selection) into React Flow nodes and edges.
 * Structure comes from the model, positions from ELK (+ user drags): layout is
 * never written back to the model.
 */
export function useCanvasGraph(elements: readonly Element[], relations: readonly Relation[], decor: Decor = {}) {
  const expandedRaw = useCanvasStore((s) => s.expanded)
  const positions = useCanvasStore((s) => s.positions)
  const filter = useCanvasStore((s) => s.filter)
  const selectedId = useCanvasStore((s) => s.selectedId)
  const reducedMotion = useReducedMotion()

  const tree = useMemo(() => buildTree(elements), [elements])
  // Only elements that actually have children can be expanded.
  const expanded = useMemo(() => new Set([...expandedRaw].filter((id) => tree.children.has(id))), [expandedRaw, tree])
  const visible = useMemo(() => visibleElements(tree, expanded), [tree, expanded])
  const visibleIds = useMemo(() => new Set(visible.map((e) => e.id)), [visible])
  const hostNames = useMemo(() => hostNameIndex(tree.byId, relations), [tree, relations])
  const aggregated = useMemo(() => aggregateEdges(tree, relations, visibleIds), [tree, relations, visibleIds])

  const [layout, setLayout] = useState<Layout>(noLayout)
  useEffect(() => {
    let cancelled = false
    computeLayout(tree, expanded, aggregated)
      .then((boxes) => {
        if (cancelled) return
        setLayout((prev) => ({
          boxes,
          entering: prev.boxes.size === 0 ? new Set() : new Set([...boxes.keys()].filter((id) => !prev.boxes.has(id))),
        }))
      })
      .catch((err: unknown) => console.error("layout failed", err))
    return () => {
      cancelled = true
    }
  }, [tree, expanded, aggregated])

  const boxes = useAnimatedBoxes(layout.boxes, !reducedMotion)
  const active = isFilterActive(filter)
  const keep = useMemo(() => (active ? filterMatches(tree, filter).keep : null), [active, tree, filter])

  const nodes = useMemo<ElementFlowNode[]>(
    () =>
      visible
        .filter((e) => boxes.has(e.id))
        .map((e) => {
          const box = boxes.get(e.id)!
          const parentId = e.parent_id && visibleIds.has(e.parent_id) ? e.parent_id : undefined
          const pos = positions[e.id] ?? { x: box.x, y: box.y }
          return {
            id: e.id,
            type: "element",
            position: pos,
            parentId,
            extent: parentId ? "parent" : undefined,
            selected: e.id === selectedId,
            className: layout.entering.has(e.id) ? "archmap-enter" : undefined,
            // Explicit dimensions (also read by the minimap, which ignores inline styles).
            width: box.width,
            height: box.height,
            data: {
              element: e,
              childCount: childrenOf(tree, e.id).length,
              expanded: expanded.has(e.id),
              dimmed: keep !== null && !keep.has(e.id),
              diff: decor.diff?.get(e.id),
              lint: decor.lint?.get(e.id),
              hosts: hostNames.get(e.id) ?? noHosts,
              editable: decor.editable ?? false,
            },
          }
        }),
    [
      visible,
      visibleIds,
      boxes,
      positions,
      selectedId,
      layout.entering,
      tree,
      expanded,
      keep,
      hostNames,
      decor.diff,
      decor.lint,
      decor.editable,
    ]
  )

  const edges = useMemo<AggregatedFlowEdge[]>(
    () =>
      aggregated
        .filter((e) => boxes.has(e.source) && boxes.has(e.target))
        .map((e) => ({
          id: e.id,
          source: e.source,
          target: e.target,
          type: "aggregated",
          markerEnd: { type: MarkerType.ArrowClosed, width: 14, height: 14 },
          data: {
            count: e.count,
            kinds: e.kinds,
            relationIds: e.relationIds,
            diff: edgeDiffStatus(e.relationIds, decor.relationDiff),
            dimmed: keep !== null && !(keep.has(e.source) && keep.has(e.target)),
          },
        })),
    [aggregated, boxes, keep, decor.relationDiff]
  )

  return { tree: tree, nodes, edges, ready: layout.boxes.size > 0, boxes: layout.boxes }
}
