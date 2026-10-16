import type { ViewElement } from "@archmap/api-client"

export type Positions = Readonly<Record<string, { x: number; y: number }>>

/**
 * Canvas state restored from a saved layout. A view that was never laid out
 * (no positions, nothing expanded) starts from the server's default expansion
 * (e.g. the scope of a container view).
 */
export function layoutToState(
  layout: readonly ViewElement[],
  defaultExpanded: readonly string[],
  knownIds: ReadonlySet<string>
): { positions: Positions; expanded: string[] } {
  const hasLayout = layout.some((e) => (e.x != null && e.y != null) || !e.collapsed)
  const positions: Record<string, { x: number; y: number }> = {}
  const expanded: string[] = []
  for (const e of layout) {
    if (!knownIds.has(e.element_id)) continue
    if (e.x != null && e.y != null) positions[e.element_id] = { x: e.x, y: e.y }
    if (!e.collapsed) expanded.push(e.element_id)
  }
  return { positions, expanded: hasLayout ? expanded : defaultExpanded.filter((id) => knownIds.has(id)) }
}

/**
 * Layout to save. Only elements that were moved or expanded are listed (collapsed
 * and un-moved is the default). Entries for elements that are not part of the
 * model being shown (e.g. draft-only elements saved from another context) are
 * passed through unchanged so editing here does not erase them.
 */
export function stateToLayout(
  positions: Positions,
  expanded: ReadonlySet<string>,
  knownIds: ReadonlySet<string>,
  saved: readonly ViewElement[] = []
): ViewElement[] {
  const ids = new Set([...Object.keys(positions), ...expanded].filter((id) => knownIds.has(id)))
  const out: ViewElement[] = [...ids].sort().map((id) => {
    const p = Object.hasOwn(positions, id) ? positions[id] : undefined
    return { element_id: id, x: p?.x ?? null, y: p?.y ?? null, w: null, h: null, collapsed: !expanded.has(id) }
  })
  for (const e of saved) {
    if (!knownIds.has(e.element_id) && (e.x != null || e.y != null || !e.collapsed)) {
      out.push({
        element_id: e.element_id,
        x: e.x ?? null,
        y: e.y ?? null,
        w: e.w ?? null,
        h: e.h ?? null,
        collapsed: e.collapsed,
      })
    }
  }
  return out
}
