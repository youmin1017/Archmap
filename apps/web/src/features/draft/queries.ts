import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import {
  abandonChangeset,
  addChanges,
  approveChangeset,
  createChangeset,
  createView,
  deleteView,
  getChangeset,
  getChangesetConflicts,
  getChangesetDiff,
  getChangesetLint,
  getChangesetModel,
  getMe,
  listLayerRules,
  replaceLayerRules,
  getViewModel,
  listChangesets,
  listElements,
  listRelations,
  listVersions,
  listViews,
  mergeChangeset,
  redoChange,
  rejectChangeset,
  resolveChangeset,
  setViewLayout,
  submitChangeset,
  undoChange,
  updateView,
  type ChangeInput,
  type LayerRule,
  type ChangesetStatus,
  type Model,
  type Resolution,
  type ViewElement,
  type ViewInput,
} from "@archmap/api-client"

import "@/lib/api"
import { applyInputs } from "./optimistic"

export const keys = {
  me: ["me"] as const,
  main: ["model", "main"] as const,
  versions: ["versions"] as const,
  changesets: (status?: ChangesetStatus) => ["changesets", status ?? "all"] as const,
  draft: (id: string) => ["draft", id] as const,
  detail: (id: string) => ["draft", id, "detail"] as const,
  model: (id: string) => ["draft", id, "model"] as const,
  diff: (id: string) => ["draft", id, "diff"] as const,
  lint: (id: string) => ["draft", id, "lint"] as const,
  conflicts: (id: string) => ["draft", id, "conflicts"] as const,
  views: ["views"] as const,
  layerRules: ["layer-rules"] as const,
  /** Under the draft's key so every draft edit refetches the views computed from it. */
  viewModel: (viewId: string, changesetId?: string) =>
    changesetId ? (["draft", changesetId, "view", viewId] as const) : (["model", "main", "view", viewId] as const),
}

/** Human-readable message from an SDK error (problem+json body, Error, or anything else). */
export function errMessage(e: unknown): string {
  if (e instanceof Error) return e.message.slice(0, 300)
  if (typeof e === "object" && e !== null) {
    const p = e as { detail?: unknown; title?: unknown }
    if (typeof p.detail === "string") return p.detail
    if (typeof p.title === "string") return p.title
  }
  return "Something went wrong"
}

// ---- reads -----------------------------------------------------------------

export const useMe = () =>
  useQuery({ queryKey: keys.me, queryFn: async () => (await getMe({ throwOnError: true })).data })

export const useMainVersion = () =>
  useQuery({
    queryKey: keys.versions,
    queryFn: async () => (await listVersions({ throwOnError: true })).data.versions,
  })

export const useMainModel = () =>
  useQuery({
    queryKey: keys.main,
    queryFn: async (): Promise<Model> => {
      const [els, rels] = await Promise.all([
        listElements({ throwOnError: true }),
        listRelations({ throwOnError: true }),
      ])
      return { elements: els.data.elements, relations: rels.data.relations }
    },
  })

export const useChangesets = (status?: ChangesetStatus) =>
  useQuery({
    queryKey: keys.changesets(status),
    queryFn: async () =>
      (await listChangesets({ query: status ? { status } : undefined, throwOnError: true })).data.changesets,
  })

export const useChangesetDetail = (id: string) =>
  useQuery({
    queryKey: keys.detail(id),
    queryFn: async () => (await getChangeset({ path: { id }, throwOnError: true })).data,
  })

export const useDraftModel = (id: string) =>
  useQuery({
    queryKey: keys.model(id),
    queryFn: async () => (await getChangesetModel({ path: { id }, throwOnError: true })).data,
  })

export const useDraftDiff = (id: string) =>
  useQuery({
    queryKey: keys.diff(id),
    queryFn: async () => (await getChangesetDiff({ path: { id }, throwOnError: true })).data,
  })

export const useDraftLint = (id: string) =>
  useQuery({
    queryKey: keys.lint(id),
    queryFn: async () => (await getChangesetLint({ path: { id }, throwOnError: true })).data,
  })

export const useDraftConflicts = (id: string, enabled = true) =>
  useQuery({
    queryKey: keys.conflicts(id),
    enabled,
    queryFn: async () => (await getChangesetConflicts({ path: { id }, throwOnError: true })).data.conflicts,
  })

// ---- writes ----------------------------------------------------------------

/** Everything derived from a draft's change log is refetched after it changes. */
export function invalidateDraft(qc: QueryClient, id: string) {
  return Promise.all([
    qc.invalidateQueries({ queryKey: keys.draft(id) }),
    qc.invalidateQueries({ queryKey: ["changesets"] }),
  ])
}

export function useCreateChangeset() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: { title: string; description?: string }) =>
      (await createChangeset({ body: v, throwOnError: true })).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["changesets"] }),
    onError: (e) => toast.error(errMessage(e)),
  })
}

/**
 * Records one batch (one GUI edit). The cached draft model is patched
 * optimistically so the canvas responds instantly; a rejection restores it.
 */
export function useAddChanges(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (inputs: ChangeInput[]) =>
      (await addChanges({ path: { id }, body: { changes: inputs }, throwOnError: true })).data,
    onMutate: async (inputs) => {
      await qc.cancelQueries({ queryKey: keys.model(id) })
      const previous = qc.getQueryData<Model>(keys.model(id))
      if (previous) qc.setQueryData<Model>(keys.model(id), applyInputs(previous, inputs))
      return { previous }
    },
    onError: (e, _inputs, ctx) => {
      if (ctx?.previous) qc.setQueryData(keys.model(id), ctx.previous)
      toast.error(errMessage(e))
    },
    onSettled: () => invalidateDraft(qc, id),
  })
}

function useDraftAction(id: string, run: () => Promise<unknown>, success?: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => run(),
    onSuccess: () => {
      if (success) toast.success(success)
    },
    onError: (e) => toast.error(errMessage(e)),
    onSettled: () => invalidateDraft(qc, id),
  })
}

export const useUndo = (id: string) => useDraftAction(id, () => undoChange({ path: { id }, throwOnError: true }))
export const useRedo = (id: string) => useDraftAction(id, () => redoChange({ path: { id }, throwOnError: true }))
export const useSubmit = (id: string) =>
  useDraftAction(id, () => submitChangeset({ path: { id }, throwOnError: true }), "Submitted for review")
export const useApprove = (id: string) =>
  useDraftAction(id, () => approveChangeset({ path: { id }, throwOnError: true }), "Approved")
export const useReject = (id: string) =>
  useDraftAction(id, () => rejectChangeset({ path: { id }, throwOnError: true }), "Rejected")
export const useAbandon = (id: string) =>
  useDraftAction(id, () => abandonChangeset({ path: { id }, throwOnError: true }), "Abandoned")

export function useResolve(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (resolutions: Resolution[]) =>
      (await resolveChangeset({ path: { id }, body: { resolutions }, throwOnError: true })).data.conflicts,
    onError: (e) => toast.error(errMessage(e)),
    onSettled: () => invalidateDraft(qc, id),
  })
}

/** Merge answers 409 with a conflict list (not an error for the UI): returned as data. */
export function useMerge(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const res = await mergeChangeset({ path: { id } })
      if (res.response?.status === 409 && res.error && "conflicts" in (res.error as object)) {
        return { merged: false as const, conflicts: (res.error as { conflicts: unknown[] }).conflicts }
      }
      if (res.error !== undefined) throw res.error
      return { merged: true as const, version: res.data.version }
    },
    onSuccess: (r) => {
      if (r.merged) {
        toast.success(`Merged as version ${r.version}`)
        void qc.invalidateQueries({ queryKey: keys.main })
        void qc.invalidateQueries({ queryKey: keys.versions })
      } else {
        toast.warning("Merge needs conflict resolution")
      }
    },
    onError: (e) => toast.error(errMessage(e)),
    onSettled: () => invalidateDraft(qc, id),
  })
}

// ---- views -----------------------------------------------------------------

export const useViews = () =>
  useQuery({ queryKey: keys.views, queryFn: async () => (await listViews({ throwOnError: true })).data.views })

/** What a view shows, from main or (with changesetId) from a draft's effective model. */
export const useViewModel = (viewId: string | undefined, changesetId?: string) =>
  useQuery({
    queryKey: keys.viewModel(viewId ?? "", changesetId),
    enabled: viewId !== undefined,
    queryFn: async () =>
      (
        await getViewModel({
          path: { id: viewId ?? "" },
          query: changesetId ? { changeset_id: changesetId } : undefined,
          throwOnError: true,
        })
      ).data,
  })

export function useCreateView() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: ViewInput) => (await createView({ body, throwOnError: true })).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.views }),
    onError: (e) => toast.error(errMessage(e)),
  })
}

export function useUpdateView() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: { id: string; body: ViewInput }) =>
      (await updateView({ path: { id: v.id }, body: v.body, throwOnError: true })).data,
    onSuccess: () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: keys.views }),
        qc.invalidateQueries({ queryKey: ["model"] }),
        qc.invalidateQueries({ queryKey: ["draft"] }),
      ]),
    onError: (e) => toast.error(errMessage(e)),
  })
}

export function useDeleteView() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => deleteView({ path: { id }, throwOnError: true }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.views }),
    onError: (e) => toast.error(errMessage(e)),
  })
}

/** Saves a view's layout (replaces it). Never touches the model, changes or versions. */
export function useSaveLayout() {
  return useMutation({
    mutationFn: async (v: { id: string; elements: ViewElement[] }) =>
      (await setViewLayout({ path: { id: v.id }, body: { elements: v.elements }, throwOnError: true })).data,
    onError: (e) => toast.error(`Layout not saved: ${errMessage(e)}`),
  })
}

// ---- layer rules -----------------------------------------------------------

/** Which layers each element class may be placed on (the API enforces them on edits). */
export const useLayerRules = () =>
  useQuery({
    queryKey: keys.layerRules,
    queryFn: async () => (await listLayerRules({ throwOnError: true })).data.rules,
  })

export function useReplaceLayerRules() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (rules: LayerRule[]) => (await replaceLayerRules({ body: { rules }, throwOnError: true })).data,
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.layerRules }),
    onError: (e) => toast.error(errMessage(e)),
  })
}
