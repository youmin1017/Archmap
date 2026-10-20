import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { getSettings, updateSettings, type ReviewPolicy } from "@archmap/api-client"
import { toast } from "sonner"

import "@/lib/api"
import { errMessage } from "../draft/queries"

const settingsKey = ["settings"] as const

/** Workspace settings (review policy, ADR-072); readable by every signed-in user. */
export const useSettings = () =>
  useQuery({
    queryKey: settingsKey,
    queryFn: async () => (await getSettings({ throwOnError: true })).data,
    staleTime: 30_000,
  })

export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (reviewPolicy: ReviewPolicy) =>
      (await updateSettings({ body: { review_policy: reviewPolicy }, throwOnError: true })).data,
    onSuccess: (s) => {
      qc.setQueryData(settingsKey, s)
      toast.success(
        s.review_policy === "direct" ? "Main can now be edited directly" : "Edits now need a reviewed draft"
      )
    },
    onError: (e) => toast.error(errMessage(e)),
  })
}
