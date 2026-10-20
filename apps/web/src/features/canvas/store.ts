import { create } from "zustand"
import type { ElementKind } from "@archmap/api-client"

import { emptyFilter, type ElementFilter } from "./graph"

type Position = { x: number; y: number }

/** Expand state and drags of one canvas scope (the view itself, or one focused element). */
type ScopeLayout = { expanded: ReadonlySet<string>; positions: Readonly<Record<string, Position>> }

/**
 * Ephemeral canvas UI state (brief: Zustand only for this). Nothing here is
 * model data: dragging a node only writes `positions` (layout), never the model.
 * Persisting layout per view is M5.
 */
type CanvasState = {
  expanded: ReadonlySet<string>
  selectedId: string | null
  /** Relations behind a selected (possibly aggregated) edge. */
  selectedRelationIds: readonly string[]
  /** Connect mode (from the context menu): the next clicked element becomes the target. */
  connectFrom: string | null
  /** User-dragged positions relative to the parent node; they win over ELK. */
  positions: Readonly<Record<string, Position>>
  filter: ElementFilter
  /** Counts user changes to the layout (moves, expand / collapse); drives autosave of a view layout. */
  revision: number
  /** Bumped to ask the canvas to center on `selectedId` after the next layout. */
  revealToken: number
  /**
   * Element the canvas is focused on (drill-down, ADR-071); mirrors the URL. While
   * focused, `expanded` / `positions` hold the focus's own state and the view's
   * layout waits in `viewLayout`, so focusing never changes what a view saves.
   */
  focusId: string | null
  viewLayout: ScopeLayout | null
  /** Expand state and drags per focused element, kept while the page is open (never saved). */
  focusLayouts: Readonly<Record<string, ScopeLayout>>
  /** "Save as view" from a focused canvas: the view tabs open the view dialog for this element. */
  viewRequest: string | null

  toggle: (id: string) => void
  expand: (ids: readonly string[]) => void
  collapse: (id: string) => void
  setExpanded: (ids: readonly string[]) => void
  /** Replaces layout state with a saved one (does not count as a user change). */
  loadLayout: (positions: Record<string, Position>, expanded: readonly string[]) => void
  select: (id: string | null) => void
  selectRelations: (ids: readonly string[]) => void
  setConnectFrom: (id: string | null) => void
  requestView: (elementId: string | null) => void
  /** Enter (id), switch or leave (null) focus. */
  setFocus: (id: string | null) => void
  /** The layout of the view itself (what autosave persists), also while focused. */
  viewLayoutState: () => ScopeLayout
  /** Select an element, expand its ancestors and center the canvas on it. */
  reveal: (id: string, ancestorIds: readonly string[]) => void
  setPosition: (id: string, pos: Position) => void
  resetPositions: () => void
  setQuery: (query: string) => void
  setKinds: (kinds: readonly ElementKind[]) => void
  setTags: (tags: readonly string[]) => void
  setProject: (project: string | null) => void
  setOwner: (owner: string | null) => void
  setEnvironment: (environment: ElementFilter["environment"]) => void
  setLifecycle: (lifecycle: ElementFilter["lifecycle"]) => void
  clearFilter: () => void
}

// Layout changes count as view changes (autosave) only when not focused.
const bump = (s: CanvasState) => (s.focusId === null ? s.revision + 1 : s.revision)

export const useCanvasStore = create<CanvasState>()((set, get) => ({
  expanded: new Set(),
  selectedId: null,
  selectedRelationIds: [],
  connectFrom: null,
  positions: {},
  filter: emptyFilter,
  revealToken: 0,
  revision: 0,
  focusId: null,
  viewLayout: null,
  focusLayouts: {},
  viewRequest: null,
  requestView: (elementId) => set({ viewRequest: elementId }),

  toggle: (id) =>
    set((s) => {
      const next = new Set(s.expanded)
      if (!next.delete(id)) next.add(id)
      return { expanded: next, revision: bump(s) }
    }),
  expand: (ids) => set((s) => ({ expanded: new Set([...s.expanded, ...ids]), revision: bump(s) })),
  collapse: (id) =>
    set((s) => {
      const next = new Set(s.expanded)
      next.delete(id)
      return { expanded: next, revision: bump(s) }
    }),
  setExpanded: (ids) => set((s) => ({ expanded: new Set(ids), revision: bump(s) })),
  // While focused (the URL already has ?focus= when the view's layout arrives), the loaded
  // layout waits aside until the focus is left.
  loadLayout: (positions, expanded) =>
    set((s) =>
      s.focusId === null
        ? { positions, expanded: new Set(expanded), viewLayout: null, focusLayouts: {} }
        : { viewLayout: { positions, expanded: new Set(expanded) }, focusLayouts: {} }
    ),
  setFocus: (id) =>
    set((s) => {
      if (id === s.focusId) return {}
      const current: ScopeLayout = { expanded: s.expanded, positions: s.positions }
      const viewLayout = s.focusId === null ? current : s.viewLayout
      const focusLayouts = s.focusId === null ? s.focusLayouts : { ...s.focusLayouts, [s.focusId]: current }
      // Entering a focus for the first time opens the focused element one level.
      const next = id === null ? viewLayout : (focusLayouts[id] ?? { expanded: new Set([id]), positions: {} })
      return {
        focusId: id,
        viewLayout: id === null ? null : viewLayout,
        focusLayouts,
        expanded: next?.expanded ?? new Set(),
        positions: next?.positions ?? {},
      }
    }),
  viewLayoutState: () => {
    const s = get()
    return s.focusId === null || !s.viewLayout ? { expanded: s.expanded, positions: s.positions } : s.viewLayout
  },
  select: (id) => set({ selectedId: id, selectedRelationIds: [] }),
  setConnectFrom: (id) => set({ connectFrom: id }),
  selectRelations: (ids) => set({ selectedId: null, selectedRelationIds: ids }),
  reveal: (id, ancestorIds) =>
    set((s) => ({
      selectedId: id,
      selectedRelationIds: [],
      expanded: new Set([...s.expanded, ...ancestorIds]),
      revealToken: s.revealToken + 1,
      revision: bump(s),
    })),
  setPosition: (id, pos) => set((s) => ({ positions: { ...s.positions, [id]: pos }, revision: bump(s) })),
  resetPositions: () => set((s) => ({ positions: {}, revision: bump(s) })),
  setQuery: (query) => set((s) => ({ filter: { ...s.filter, query } })),
  setKinds: (kinds) => set((s) => ({ filter: { ...s.filter, kinds } })),
  setTags: (tags) => set((s) => ({ filter: { ...s.filter, tags } })),
  setProject: (project) => set((s) => ({ filter: { ...s.filter, project } })),
  setOwner: (owner) => set((s) => ({ filter: { ...s.filter, owner } })),
  setEnvironment: (environment) => set((s) => ({ filter: { ...s.filter, environment } })),
  setLifecycle: (lifecycle) => set((s) => ({ filter: { ...s.filter, lifecycle } })),
  clearFilter: () => set({ filter: emptyFilter }),
}))
