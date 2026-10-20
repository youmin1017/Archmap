import type { Element, ElementKind, LayerRule } from "@archmap/api-client"

export const UNASSIGNED = "Unassigned"

export type Plane = {
  name: string
  /** The catch-all plane for elements without a (known) layer; it is always last. */
  unassigned: boolean
  elements: Element[]
}

const kindOrder: Record<ElementKind, number> = {
  person: 0,
  system: 1,
  container: 2,
  component: 3,
  node: 4,
}

const byKindThenName = (a: Element, b: Element) =>
  kindOrder[a.kind] - kindOrder[b.kind] || a.name.localeCompare(b.name) || a.id.localeCompare(b.id)

/**
 * Splits elements into the view's layers, top layer first. Layer names match
 * case-insensitively; an element with an empty or unknown layer lands on the
 * trailing "Unassigned" layer, which only exists when needed.
 */
export function groupByLayer(elements: readonly Element[], layers: readonly string[]): Plane[] {
  const planes: Plane[] = layers.map((name) => ({ name, unassigned: false, elements: [] }))
  const index = new Map(layers.map((name, i) => [name.trim().toLowerCase(), i]))
  const loose: Element[] = []
  for (const e of elements) {
    const i = index.get(e.layer.trim().toLowerCase())
    if (i === undefined) loose.push(e)
    else planes[i].elements.push(e)
  }
  if (loose.length > 0) planes.push({ name: UNASSIGNED, unassigned: true, elements: loose })
  for (const p of planes) p.elements.sort(byKindThenName)
  return planes
}

/** How a layer is shown: fully, as a thin collapsed bar (another layer has the focus), or not at all. */
export type LaneMode = "open" | "collapsed" | "hidden"

/**
 * Browsing state of the stack: layers the user switched off are hidden; when
 * one layer has the focus ("only this"), every other visible layer collapses
 * to its header so the focused plan gets the room.
 */
export function laneModes(
  planes: readonly Plane[],
  hidden: ReadonlySet<string>,
  focus: string | null
): Map<string, LaneMode> {
  const focused = focus !== null && planes.some((p) => p.name === focus && !hidden.has(p.name)) ? focus : null
  return new Map(
    planes.map((p) => [
      p.name,
      hidden.has(p.name) ? "hidden" : focused === null || focused === p.name ? "open" : "collapsed",
    ])
  )
}

/** Visible-layer toggle. */
export function toggleHidden(hidden: ReadonlySet<string>, name: string): Set<string> {
  const next = new Set(hidden)
  if (!next.delete(name)) next.add(name)
  return next
}

/** Distinct-but-calm layer tints (hue in degrees), cycling for many layers. */
export const planeHue = (i: number) => (210 + i * 47) % 360

/** A view holds at most this many layers (same limit as the API). */
export const MAX_LAYERS = 12
export const MAX_LAYER_NAME = 60

const sameLayer = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase()

/**
 * Why `name` cannot be a layer name, or null when it is fine. `renaming` is the
 * layer being renamed (it may keep its name or only change the case).
 */
export function layerNameError(layers: readonly string[], name: string, renaming?: string): string | null {
  const n = name.trim()
  if (n === "") return "Enter a name."
  if (n.length > MAX_LAYER_NAME) return `At most ${MAX_LAYER_NAME} characters.`
  if (sameLayer(n, UNASSIGNED)) return `"${UNASSIGNED}" is reserved for elements without a layer.`
  if (layers.some((l) => sameLayer(l, n) && !(renaming !== undefined && sameLayer(l, renaming)))) {
    return "A layer with this name already exists."
  }
  if (renaming === undefined && layers.length >= MAX_LAYERS) return `At most ${MAX_LAYERS} layers.`
  return null
}

/** The layer list with `from` renamed to `to` (position kept). */
export function renameLayerIn(layers: readonly string[], from: string, to: string): string[] {
  return layers.map((l) => (sameLayer(l, from) ? to.trim() : l))
}

export function removeLayerFrom(layers: readonly string[], name: string): string[] {
  return layers.filter((l) => !sameLayer(l, name))
}

/** Moves a layer up (-1) or down (+1) in the stack; unchanged when it cannot move. */
export function shiftLayer(layers: readonly string[], name: string, delta: -1 | 1): string[] {
  const i = layers.findIndex((l) => sameLayer(l, name))
  const j = i + delta
  if (i < 0 || j < 0 || j >= layers.length) return [...layers]
  const next = [...layers]
  ;[next[i], next[j]] = [next[j], next[i]]
  return next
}

/** Elements that sit on `layer` (matched like the view groups them). */
export const onLayer = (elements: readonly Element[], layer: string) =>
  elements.filter((e) => sameLayer(e.layer, layer))

/** Classes a layer rule can name: element kinds and element types (ADR-062; the names do not overlap). */
export const ruleClasses = [
  "person",
  "system",
  "container",
  "component",
  "node",
  "database-server",
  "database",
  "schema",
  "cache",
  "queue",
  "proxy",
] as const

const classLabel: Record<string, string> = {
  person: "People",
  system: "Systems",
  container: "Containers",
  component: "Components",
  node: "Nodes",
  "database-server": "Database servers",
  database: "Databases",
  schema: "Schemas",
  cache: "Caches",
  queue: "Queues",
  proxy: "Proxies",
}

/** Whether an element belongs to a rule class (its kind or its type). */
const inClass = (e: Pick<Element, "kind" | "type">, c: string) => e.kind === c || (e.type !== "" && e.type === c)
export const ruleClassLabel = (c: string) => classLabel[c] ?? c

/**
 * The first rule that forbids putting an element of this kind on `layer`, or
 * null. Matches the API (an element without a layer is always fine); the API
 * stays the authority.
 */
export function layerViolation(
  rules: readonly LayerRule[],
  e: Pick<Element, "kind" | "type">,
  layer: string
): LayerRule | null {
  if (layer.trim() === "") return null
  return (
    rules.find(
      (r) => r.layers.length > 0 && inClass(e, r.element_class) && !r.layers.some((l) => sameLayer(l, layer))
    ) ?? null
  )
}

export const layerAllowed = (rules: readonly LayerRule[], e: Pick<Element, "kind" | "type">, layer: string) =>
  layerViolation(rules, e, layer) === null

/** Human wording of a violated rule. */
export const violationMessage = (r: LayerRule) =>
  `${ruleClassLabel(r.element_class)} can only be placed on: ${r.layers.join(", ")}.`

/** Whether any element of `kind` may go on `layer` (kind rules only; type rules depend on the element). */
export const kindAllowedOnLayer = (rules: readonly LayerRule[], kind: ElementKind, layer: string) =>
  layer === "" ||
  rules.every((r) => r.element_class !== kind || r.layers.length === 0 || r.layers.some((l) => sameLayer(l, layer)))

/** An element and the elements of the same layer that sit inside it (its children in the model tree). */
export type GroupNode = { element: Element; children: GroupNode[] }

/**
 * Nests a layer's elements by their parent so that, for example, a Kubernetes
 * namespace shows its services inside it. Elements whose parent is not on this
 * layer stay at the top level; the input order is kept at every level.
 */
export function nestWithin(elements: readonly Element[]): GroupNode[] {
  const nodes = new Map(elements.map((e) => [e.id, { element: e, children: [] as GroupNode[] }]))
  const roots: GroupNode[] = []
  for (const e of elements) {
    const node = nodes.get(e.id)!
    const parent = e.parent_id ? nodes.get(e.parent_id) : undefined
    if (parent && parent !== node) parent.children.push(node)
    else roots.push(node)
  }
  return roots
}
