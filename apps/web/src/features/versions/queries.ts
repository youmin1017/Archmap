import { useQuery } from "@tanstack/react-query"
import { getVersionDiff, getVersionModel, type Model } from "@archmap/api-client"

import "@/lib/api"

// Merged versions never change, so these reads can be cached for the session.
const forever = { staleTime: Infinity } as const

export const useVersionModel = (n: number | undefined) =>
  useQuery({
    queryKey: ["versions", n, "model"] as const,
    enabled: n !== undefined,
    ...forever,
    queryFn: async (): Promise<Model> => (await getVersionModel({ path: { n: n as number }, throwOnError: true })).data,
  })

/** What changed going from version `from` to `to`. */
export const useVersionDiff = (from: number | undefined, to: number | undefined) =>
  useQuery({
    queryKey: ["versions", "diff", from, to] as const,
    enabled: from !== undefined && to !== undefined,
    ...forever,
    queryFn: async () =>
      (await getVersionDiff({ path: { from: from as number, to: to as number }, throwOnError: true })).data,
  })
