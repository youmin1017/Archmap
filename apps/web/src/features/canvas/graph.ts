import type { Element, ElementKind, Environment, Lifecycle, Relation, RelationKind } from "@archmap/api-client"

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

/**
 * nearestVisible for many ids at once: each element's answer is computed once and
 * shared with its descendants, so large models do not walk every ancestor chain per relation.
 */
function nearestVisibleIndex(tree: ElementTree, visible: ReadonlySet<string>) {
  const memo = new Map<string, string | null>()
  const resolve = (id: string): string | null => {
    const known = memo.get(id)
    if (known !== undefined) return known
    // Walk up until a visible or already resolved element, then fill the chain.
    const chain: string[] = []
    const seen = new Set<string>()
    let cur: string | undefined = id
    let answer: string | null = null
    while (cur !== undefined && !seen.has(cur)) {
      if (visible.has(cur)) {
        answer = cur
        break
      }
      const m = memo.get(cur)
      if (m !== undefined) {
        answer = m
        break
      }
      chain.push(cur)
      seen.add(cur)
      const parent: string | null | undefined = tree.byId.get(cur)?.parent_id
      cur = parent && tree.byId.has(parent) ? parent : undefined
    }
    for (const c of chain) memo.set(c, answer)
    return answer
  }
  return resolve
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
  const nearest = nearestVisibleIndex(tree, visible)
  const ancestors = new Map<string, ReadonlySet<string>>()
  const ancestorsOf = (id: string) => {
    let a = ancestors.get(id)
    if (!a) ancestors.set(id, (a = new Set(ancestorIds(tree, id))))
    return a
  }
  for (const r of relations) {
    const source = nearest(r.source_id)
    const target = nearest(r.target_id)
    if (!source || !target || source === target) continue
    if (ancestorsOf(source).has(target) || ancestorsOf(target).has(source)) continue

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
  /** Effective owner id (role owner), UNOWNED for elements without one; null = all. */
  owner: string | null
  /** Effective environment; null = all. */
  environment: Exclude<Environment, ""> | null
  lifecycle: Lifecycle | null
}

/** Owner filter value for elements without an effective owner. */
export const UNOWNED = "__unowned__"

export const emptyFilter: ElementFilter = {
  query: "",
  kinds: [],
  tags: [],
  project: null,
  owner: null,
  environment: null,
  lifecycle: null,
}

export function isFilterActive(f: ElementFilter): boolean {
  return (
    f.query.trim() !== "" ||
    f.kinds.length > 0 ||
    f.tags.length > 0 ||
    f.project !== null ||
    f.owner !== null ||
    f.environment !== null ||
    f.lifecycle !== null
  )
}

export function matchesFilter(e: Element, f: ElementFilter): boolean {
  const q = f.query.trim().toLowerCase()
  if (q && !`${e.name} ${e.technology} ${e.description}`.toLowerCase().includes(q)) return false
  if (f.kinds.length > 0 && !f.kinds.includes(e.kind)) return false
  if (f.tags.length > 0 && !f.tags.some((t) => e.tags.includes(t))) return false
  if (f.project !== null && !inProject(e, f.project)) return false
  if (f.owner !== null) {
    const owner = e.effective?.owners.find((o) => o.role === "owner")?.owner_id ?? null
    if (f.owner === UNOWNED ? owner !== null || e.kind === "person" : owner !== f.owner) return false
  }
  if (f.environment !== null && e.effective?.environment !== f.environment) return false
  if (f.lifecycle !== null && e.lifecycle !== f.lifecycle) return false
  return true
}

/** Owner ids that are someone's effective owner, for the owner filter. */
export function usedOwners(elements: readonly Element[]): Set<string> {
  const out = new Set<string>()
  for (const e of elements) for (const o of e.effective?.owners ?? []) if (o.role === "owner") out.add(o.owner_id)
  return out
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

/** Ids of an element and all its descendants. */
export function subtreeIds(tree: ElementTree, id: string): Set<string> {
  const out = new Set<string>()
  const walk = (cur: string) => {
    if (out.has(cur)) return
    out.add(cur)
    for (const c of childrenOf(tree, cur)) walk(c.id)
  }
  walk(id)
  return out
}

export type FocusScope = {
  /** The focused element and its descendants. */
  inside: ReadonlySet<string>
  /** Outside elements related to the inside, each lifted to its highest ancestor that does not contain the focus. */
  context: ReadonlySet<string>
}

/**
 * What a canvas focused on one element shows (ADR-071): the element's subtree plus
 * the outside elements it is related to (uses and deployed_on). Like a container view
 * on the server (view.Compute), the other end of a relation is lifted to its highest
 * ancestor that does not contain the focus, so context stays at the level of systems.
 */
export function focusScope(tree: ElementTree, relations: readonly Relation[], focusId: string): FocusScope {
  const inside = subtreeIds(tree, focusId)
  const focusAncestors = new Set(ancestorIds(tree, focusId))
  const context = new Set<string>()
  for (const r of relations) {
    const s = inside.has(r.source_id)
    const t = inside.has(r.target_id)
    if (s === t) continue
    const other = s ? r.target_id : r.source_id
    if (!tree.byId.has(other) || focusAncestors.has(other)) continue
    let best = other
    for (const a of ancestorIds(tree, other)) {
      if (focusAncestors.has(a)) break
      best = a
    }
    context.add(best)
  }
  return { inside, context }
}

/**
 * The tree a focused canvas lays out: the focused element and the context elements
 * are the roots; only the focused subtree keeps its children (context cards cannot
 * expand). `byId` stays complete so relations deep inside a context element still
 * route to it.
 */
export function focusTree(tree: ElementTree, scope: FocusScope, focusId: string): ElementTree {
  const children = new Map<string, readonly Element[]>()
  for (const id of scope.inside) {
    const kids = tree.children.get(id)
    if (kids) children.set(id, kids)
  }
  const focus = tree.byId.get(focusId)
  const context = [...scope.context]
    .map((id) => tree.byId.get(id))
    .filter((e): e is Element => e !== undefined)
    .sort(byName)
  return { byId: tree.byId, children, roots: focus ? [focus, ...context] : context }
}

/** Path from the top level down to an element (for the focus breadcrumb). */
export function pathTo(tree: ElementTree, id: string): Element[] {
  return [...ancestorIds(tree, id)]
    .reverse()
    .concat(id)
    .map((a) => tree.byId.get(a))
    .filter((e): e is Element => e !== undefined)
}
