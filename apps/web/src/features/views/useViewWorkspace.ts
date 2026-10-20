import { useEffect, useMemo, useRef } from "react"
import type { Element, Model, Relation } from "@archmap/api-client"

import { useCanvasStore } from "../canvas/store"
import { useSaveLayout, useViewModel } from "../draft/queries"
import { layoutToState, stateToLayout } from "./layoutMap"

const SAVE_DELAY_MS = 700

type Options = {
  /** Selected view (from the URL); undefined = whole model. */
  viewId: string | undefined
  /** Draft whose effective model the view is computed from; undefined = main. */
  changesetId?: string
  /** The complete model, shown when no view is selected. */
  fullModel: Model | undefined
  /** Only editors persist layout; for viewers moving things is just a local preview. */
  canSaveLayout: boolean
}

/**
 * Resolves what the canvas shows (whole model or a view), loads a view's saved
 * layout into the canvas store when the view is opened, and autosaves layout
 * changes (debounced). Layout is never part of the model: saving it creates no
 * changes and no versions.
 */
export function useViewWorkspace({ viewId, changesetId, fullModel, canSaveLayout }: Options) {
  const viewModel = useViewModel(viewId, changesetId)
  const save = useSaveLayout()
  const vm = viewId ? viewModel.data : undefined

  const elements: readonly Element[] | undefined = viewId ? vm?.elements : fullModel?.elements
  const relations: readonly Relation[] | undefined = viewId ? vm?.relations : fullModel?.relations

  // Load the saved layout once per view; later refetches (draft edits) must not overwrite local moves.
  const appliedFor = useRef<string | undefined | null>(null)
  useEffect(() => {
    const key = viewId ?? ""
    if (appliedFor.current === key) return
    const { loadLayout } = useCanvasStore.getState()
    if (!viewId) {
      loadLayout({}, [])
    } else {
      if (!vm) return
      const known = new Set(vm.elements.map((e) => e.id))
      const state = layoutToState(vm.layout, vm.expanded, known)
      loadLayout({ ...state.positions }, state.expanded)
    }
    appliedFor.current = key
  }, [viewId, vm])

  // Autosave: pending changes are flushed when the view changes or the page closes.
  const revision = useCanvasStore((s) => s.revision)
  const savedRevision = useRef(revision)
  const latest = useRef({ viewId, vm, canSaveLayout, save: save.mutate })
  latest.current = { viewId, vm, canSaveLayout, save: save.mutate }

  const flush = useRef(() => {})
  flush.current = () => {
    const { viewId: id, vm: model, canSaveLayout: can, save: mutate } = latest.current
    const rev = useCanvasStore.getState().revision
    if (!id || !model || !can || rev === savedRevision.current || appliedFor.current !== id) return
    // While focused, the store holds the focus's layout; the view's own layout waits aside.
    const { positions, expanded } = useCanvasStore.getState().viewLayoutState()
    const known = new Set(model.elements.map((e) => e.id))
    savedRevision.current = rev
    mutate({ id, elements: stateToLayout(positions, expanded, known, model.layout) })
  }

  useEffect(() => {
    if (!viewId || !canSaveLayout || revision === savedRevision.current) return
    const timer = setTimeout(() => flush.current(), SAVE_DELAY_MS)
    return () => clearTimeout(timer)
  }, [viewId, canSaveLayout, revision])

  // Switching views or leaving the page saves what is pending.
  useEffect(() => {
    const flushNow = flush.current
    return () => {
      flushNow()
    }
  }, [viewId])
  useEffect(() => {
    const onHide = () => flush.current()
    window.addEventListener("pagehide", onHide)
    return () => window.removeEventListener("pagehide", onHide)
  }, [])

  // New view -> its first load is not a user change.
  useEffect(() => {
    savedRevision.current = useCanvasStore.getState().revision
  }, [viewId])

  return useMemo(
    () => ({
      viewModel: vm,
      elements,
      relations,
      loading: viewId ? viewModel.isPending : fullModel === undefined,
      error: viewId ? viewModel.isError : false,
      scopeMissing: vm?.scope_missing ?? false,
      saving: save.isPending,
    }),
    [vm, elements, relations, viewId, viewModel.isPending, viewModel.isError, fullModel, save.isPending]
  )
}
