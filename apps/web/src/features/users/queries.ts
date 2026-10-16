import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { listUsers, setUserRole, type Role } from "@archmap/api-client"
import { toast } from "sonner"

import "@/lib/api"
import { errMessage, keys } from "../draft/queries"

const usersKey = ["users"] as const

/** All users; the API answers 403 unless the caller is a maintainer. */
export const useUsers = (enabled = true) =>
  useQuery({
    queryKey: usersKey,
    enabled,
    retry: false,
    queryFn: async () => (await listUsers({ throwOnError: true })).data.users,
  })

export function useSetUserRole() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: { id: string; role: Role }) =>
      (await setUserRole({ path: { id: v.id }, body: { role: v.role }, throwOnError: true })).data,
    onSuccess: (u) => {
      toast.success(`${u.name} is now ${u.role}`)
      void qc.invalidateQueries({ queryKey: usersKey })
      // The caller's own role may have changed.
      void qc.invalidateQueries({ queryKey: keys.me })
    },
    onError: (e) => toast.error(errMessage(e)),
  })
}
