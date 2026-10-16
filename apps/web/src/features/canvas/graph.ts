import type { Element, ElementKind, Relation, RelationKind } from "@archmap/api-client"

/** Read-only index of the element hierarchy. */
export type ElementTree = {
  byId: ReadonlyMap<string, Element>
  /** Children per parent id, sorted by name. */
  children: ReadonlyMap<string, readonly Element[]>
  roots: readonly Element[]
}

const byName = (a: Element, b: Element) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id)

export function buildTree(elements: readonly Element[]): ElementTree {
  const byId = new Map(elements.map((e) => [e.id, e]))
  const children = new Map<string, Element[]>()
  const roots: Element[] = []
  for (const e of elements) {
    // An element whose parent is missing is treated as a root so it is never lost.
    if (e.parent_id && byId.has(e.parent_id)) {
      const list = children.get(e.parent_id)
      if (list) list.push(e)
      else children.set(e.parent_id, [e])
    } else {
      roots.push(e)
    }
  }
  roots.sort(byName)
  for (const list of children.values()) list.sort(byName)
  return { byId, children, roots }
}

export function childrenOf(tree: ElementTree, id: string): readonly Element[] {
  return tree.children.get(id) ?? []
}

/** Ancestor ids of an element, nearest parent first. */
export function ancestorIds(tree: ElementTree, id: string): string[] {
  const out: string[] = []
  const seen = new Set([id])
  let cur = tree.byId.get(id)
  while (cur?.parent_id && tree.byId.has(cur.parent_id) && !seen.has(cur.parent_id)) {
    out.push(cur.parent_id)
    seen.add(cur.parent_id)
    cur = tree.byId.get(cur.parent_id)
  }
  return out
}

export function descendantCount(tree: ElementTree, id: string): number {
  let n = 0
  for (const c of childrenOf(tree, id)) n += 1 + descendantCount(tree, c.id)
  return n
}

/**
 * Elements shown for a set of expanded ids, parents before children (the order
 * React Flow requires). Roots are always shown; the children of an expanded
 * element are shown when the element itself is shown.
 */
export function visibleElements(tree: ElementTree, expanded: ReadonlySet<string>): Element[] {
  const out: Element[] = []
  const walk = (e: Element) => {
    out.push(e)
    if (expanded.has(e.id)) for (const c of childrenOf(tree, e.id)) walk(c)
  }
  for (const r of tree.roots) walk(r)
  return out
}

/** The element itself if visible, otherwise its nearest visible ancestor. */
export function nearestVisible(tree: ElementTree, id: string, visible: ReadonlySet<string>): string | null {
  if (visible.has(id)) return id
  for (const a of ancestorIds(tree, id)) if (visible.has(a)) return a
  return null
}

export type AggregatedEdge = {
  id: string
  source: string
  target: string
  count: number
  relationIds: string[]
  kinds: Partial<Record<RelationKind, number>>
}

/**
 * Edges between visible nodes. A relation touching a collapsed element is
 * re-routed to its nearest visible ancestor; parallel relations between the
 * same pair collapse into one edge with a count. Relations that end up inside a
 * single node, or between a node and one of its own descendants, are not drawn.
 */
export function aggregateEdges(
  tree: ElementTree,
  relations: readonly Relation[],
  visible: ReadonlySet<string>
): AggregatedEdge[] {
  const edges = new Map<string, AggregatedEdge>()
  for (const r of relations) {
    const source = nearestVisible(tree, r.source_id, visible)
    const target = nearestVisible(tree, r.target_id, visible)
    if (!source || !target || source === target) continue
    if (ancestorIds(tree, source).includes(target) || ancestorIds(tree, target).includes(source)) continue

    const id = `${source}->${target}`
    let edge = edges.get(id)
    if (!edge) {
      edge = { id, source, target, count: 0, relationIds: [], kinds: {} }
      edges.set(id, edge)
    }
    edge.count += 1
    edge.relationIds.push(r.id)
    edge.kinds[r.kind] = (edge.kinds[r.kind] ?? 0) + 1
  }
  return [...edges.values()].sort((a, b) => a.id.localeCompare(b.id))
}

export type ElementFilter = {
  query: string
  kinds: readonly ElementKind[]
  tags: readonly string[]
  /** Highlight the elements of one project (others are dimmed); null = all. */
  project: string | null
}

export const emptyFilter: ElementFilter = { query: "", kinds: [], tags: [], project: null }

export function isFilterActive(f: ElementFilter): boolean {
  return f.query.trim() !== "" || f.kinds.length > 0 || f.tags.length > 0 || f.project !== null
}

export function matchesFilter(e: Element, f: ElementFilter): boolean {
  const q = f.query.trim().toLowerCase()
  if (q && !`${e.name} ${e.technology} ${e.description}`.toLowerCase().includes(q)) return false
  if (f.kinds.length > 0 && !f.kinds.includes(e.kind)) return false
  if (f.tags.length > 0 && !f.tags.some((t) => e.tags.includes(t))) return false
  if (f.project !== null && !inProject(e, f.project)) return false
  return true
}

export const inProject = (e: Element, project: string) =>
  e.projects.some((p) => p.toLowerCase() === project.toLowerCase())

/** Ids that match the filter plus all their ancestors (so a match stays reachable in the tree). */
export function filterMatches(tree: ElementTree, f: ElementFilter): { matches: Set<string>; keep: Set<string> } {
  const matches = new Set<string>()
  const keep = new Set<string>()
  for (const e of tree.byId.values()) {
    if (!matchesFilter(e, f)) continue
    matches.add(e.id)
    keep.add(e.id)
    for (const a of ancestorIds(tree, e.id)) keep.add(a)
  }
  return { matches, keep }
}

export function allTags(elements: readonly Element[]): string[] {
  return [...new Set(elements.flatMap((e) => e.tags))].sort((a, b) => a.localeCompare(b))
}

export function allProjects(elements: readonly Element[]): string[] {
  const byKey = new Map<string, string>()
  for (const p of elements.flatMap((e) => e.projects)) if (!byKey.has(p.toLowerCase())) byKey.set(p.toLowerCase(), p)
  return [...byKey.values()].sort((a, b) => a.localeCompare(b))
}

/** Ids of elements that have children (the only ones that can expand). */
export function expandableIds(tree: ElementTree): string[] {
  return [...tree.children.keys()]
}
