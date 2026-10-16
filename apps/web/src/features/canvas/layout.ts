import ELK from "elkjs/lib/elk.bundled.js"
import type { ElkExtendedEdge, ElkNode } from "elkjs/lib/elk-api"

import { childrenOf, type AggregatedEdge, type ElementTree } from "./graph"

/** Size of a collapsed (leaf) card. Cards have a fixed size so layout needs no DOM measuring. */
export const LEAF_SIZE = { width: 232, height: 96 } as const
/** Space an expanded container reserves above its children for its header. */
export const HEADER_HEIGHT = 44
const PADDING = 16

/** Position relative to the parent node (React Flow's convention for nested nodes) and size. */
export type Box = { x: number; y: number; width: number; height: number }

const elk = new ELK()

const rootOptions = {
  "elk.algorithm": "layered",
  "elk.direction": "RIGHT",
  // Lay out all levels in one pass so edges crossing hierarchy boundaries are considered.
  "elk.hierarchyHandling": "INCLUDE_CHILDREN",
  "elk.spacing.nodeNode": "28",
  "elk.layered.spacing.nodeNodeBetweenLayers": "56",
  "elk.padding": "[top=24,left=24,bottom=24,right=24]",
}

const containerOptions = {
  "elk.padding": `[top=${HEADER_HEIGHT + PADDING},left=${PADDING},bottom=${PADDING},right=${PADDING}]`,
}

/**
 * Computes boxes for the visible part of the model with ELK. Collapsed elements
 * are fixed-size leaves; expanded ones are compound nodes sized around their
 * children. Elements listed in `pinned` keep their layout position (user drags)
 * as constraints are not modelled: callers overlay pinned positions afterwards.
 */
export async function computeLayout(
  tree: ElementTree,
  expanded: ReadonlySet<string>,
  edges: readonly AggregatedEdge[]
): Promise<Map<string, Box>> {
  const toElk = (id: string): ElkNode => {
    const kids = expanded.has(id) ? childrenOf(tree, id) : []
    if (kids.length === 0) return { id, ...LEAF_SIZE }
    return { id, layoutOptions: containerOptions, children: kids.map((k) => toElk(k.id)) }
  }

  const graph: ElkNode = {
    id: "root",
    layoutOptions: rootOptions,
    children: tree.roots.map((r) => toElk(r.id)),
    edges: edges.map((e): ElkExtendedEdge => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  }

  const laid = await elk.layout(graph)

  const boxes = new Map<string, Box>()
  const collect = (n: ElkNode) => {
    for (const c of n.children ?? []) {
      boxes.set(c.id, {
        x: c.x ?? 0,
        y: c.y ?? 0,
        width: c.width ?? LEAF_SIZE.width,
        height: c.height ?? LEAF_SIZE.height,
      })
      collect(c)
    }
  }
  collect(laid)
  return boxes
}
