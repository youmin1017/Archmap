import { create } from "zustand"
import type { ElementKind } from "@archmap/api-client"

import { emptyFilter, type ElementFilter } from "./graph"

type Position = { x: number; y: number }

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

  toggle: (id: string) => void
  expand: (ids: readonly string[]) => void
  collapse: (id: string) => void
  setExpanded: (ids: readonly string[]) => void
  /** Replaces layout state with a saved one (does not count as a user change). */
  loadLayout: (positions: Record<string, Position>, expanded: readonly string[]) => void
  select: (id: string | null) => void
  selectRelations: (ids: readonly string[]) => void
  setConnectFrom: (id: string | null) => void
  /** Select an element, expand its ancestors and center the canvas on it. */
  reveal: (id: string, ancestorIds: readonly string[]) => void
  setPosition: (id: string, pos: Position) => void
  resetPositions: () => void
  setQuery: (query: string) => void
  setKinds: (kinds: readonly ElementKind[]) => void
  setTags: (tags: readonly string[]) => void
  setProject: (project: string | null) => void
  clearFilter: () => void
}

export const useCanvasStore = create<CanvasState>()((set) => ({
  expanded: new Set(),
  selectedId: null,
  selectedRelationIds: [],
  connectFrom: null,
  positions: {},
  filter: emptyFilter,
  revealToken: 0,
  revision: 0,

  toggle: (id) =>
    set((s) => {
      const next = new Set(s.expanded)
      if (!next.delete(id)) next.add(id)
      return { expanded: next, revision: s.revision + 1 }
    }),
  expand: (ids) => set((s) => ({ expanded: new Set([...s.expanded, ...ids]), revision: s.revision + 1 })),
  collapse: (id) =>
    set((s) => {
      const next = new Set(s.expanded)
      next.delete(id)
      return { expanded: next, revision: s.revision + 1 }
    }),
  setExpanded: (ids) => set((s) => ({ expanded: new Set(ids), revision: s.revision + 1 })),
  loadLayout: (positions, expanded) => set({ positions, expanded: new Set(expanded) }),
  select: (id) => set({ selectedId: id, selectedRelationIds: [] }),
  setConnectFrom: (id) => set({ connectFrom: id }),
  selectRelations: (ids) => set({ selectedId: null, selectedRelationIds: ids }),
  reveal: (id, ancestorIds) =>
    set((s) => ({
      selectedId: id,
      selectedRelationIds: [],
      expanded: new Set([...s.expanded, ...ancestorIds]),
      revealToken: s.revealToken + 1,
      revision: s.revision + 1,
    })),
  setPosition: (id, pos) => set((s) => ({ positions: { ...s.positions, [id]: pos }, revision: s.revision + 1 })),
  resetPositions: () => set((s) => ({ positions: {}, revision: s.revision + 1 })),
  setQuery: (query) => set((s) => ({ filter: { ...s.filter, query } })),
  setKinds: (kinds) => set((s) => ({ filter: { ...s.filter, kinds } })),
  setTags: (tags) => set((s) => ({ filter: { ...s.filter, tags } })),
  setProject: (project) => set((s) => ({ filter: { ...s.filter, project } })),
  clearFilter: () => set({ filter: emptyFilter }),
}))
