import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  createGroup,
  createUser,
  deleteGroup,
  getDirectory,
  listGroups,
  setGroupMembers,
  updateGroup,
  disableUser,
  enableUser,
  linkUserIdentity,
  listUserIdentities,
  listUsers,
  resetUserPassword,
  setUserRole,
  unlinkUserIdentity,
  type CreateUserRequest,
  type Role,
} from "@archmap/api-client"
import { toast } from "sonner"

import "@/lib/api"
import { errMessage, keys } from "../draft/queries"

const usersKey = ["users"] as const
const identitiesKey = (id: string) => ["users", id, "identities"] as const

/** All users; the API answers 403 unless the caller is a maintainer. */
export const useUsers = (enabled = true) =>
  useQuery({
    queryKey: usersKey,
    enabled,
    retry: false,
    queryFn: async () => (await listUsers({ throwOnError: true })).data.users,
  })

export const useIdentities = (id: string | null) =>
  useQuery({
    queryKey: identitiesKey(id ?? ""),
    enabled: id !== null,
    queryFn: async () => (await listUserIdentities({ path: { id: id! }, throwOnError: true })).data.identities,
  })

export function useSetUserRole() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: { id: string; role: Role }) =>
      (await setUserRole({ path: { id: v.id }, body: { role: v.role }, throwOnError: true })).data,
    onSuccess: (u) => {
      toast.success(`${u.name} is now ${u.role}`)
      void qc.invalidateQueries({ queryKey: usersKey })
      void qc.invalidateQueries({ queryKey: directoryKey })
      // The caller's own role may have changed.
      void qc.invalidateQueries({ queryKey: keys.me })
      void qc.invalidateQueries({ queryKey: ["session"] })
    },
    onError: (e) => toast.error(errMessage(e)),
  })
}

/** Errors are shown inside the dialog, so no toast here. */
export function useCreateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: CreateUserRequest) => (await createUser({ body, throwOnError: true })).data,
    onSuccess: () => void qc.invalidateQueries({ queryKey: usersKey }),
  })
}

export function useSetDisabled() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: { id: string; disabled: boolean }) =>
      (v.disabled
        ? await disableUser({ path: { id: v.id }, throwOnError: true })
        : await enableUser({ path: { id: v.id }, throwOnError: true })
      ).data,
    onSuccess: (u) => {
      toast.success(u.disabled ? `${u.name} is disabled and signed out` : `${u.name} can sign in again`)
      void qc.invalidateQueries({ queryKey: usersKey })
      void qc.invalidateQueries({ queryKey: directoryKey })
    },
    onError: (e) => toast.error(errMessage(e)),
  })
}

export function useResetPassword() {
  return useMutation({
    mutationFn: async (id: string) => (await resetUserPassword({ path: { id }, body: {}, throwOnError: true })).data,
    onError: (e) => toast.error(errMessage(e)),
  })
}

export function useLinkIdentity(userId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: { provider: string; subject: string }) =>
      (await linkUserIdentity({ path: { id: userId }, body: v, throwOnError: true })).data,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: identitiesKey(userId) })
      void qc.invalidateQueries({ queryKey: usersKey })
      void qc.invalidateQueries({ queryKey: directoryKey })
    },
  })
}

export function useUnlinkIdentity(userId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (identityId: string) => {
      await unlinkUserIdentity({ path: { id: userId, identityId }, throwOnError: true })
    },
    onSuccess: () => {
      toast.success("Identity removed; its sessions were signed out")
      void qc.invalidateQueries({ queryKey: identitiesKey(userId) })
      void qc.invalidateQueries({ queryKey: usersKey })
      void qc.invalidateQueries({ queryKey: directoryKey })
    },
    onError: (e) => toast.error(errMessage(e)),
  })
}

// ---- groups and the owner directory (ADR-069, ADR-070) --------------------

const directoryKey = ["directory"] as const
const groupsKey = ["groups"] as const

/** Users and groups that can be picked as owners (any signed-in user). */
export const useDirectory = () =>
  useQuery({
    queryKey: directoryKey,
    queryFn: async () => (await getDirectory({ throwOnError: true })).data,
    staleTime: 60_000,
  })

export const useGroups = () =>
  useQuery({ queryKey: groupsKey, queryFn: async () => (await listGroups({ throwOnError: true })).data.groups })

function useGroupMutation<TVars, TResult>(fn: (v: TVars) => Promise<TResult>, success: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onError: (e) => toast.error(errMessage(e)),
    onSuccess: () => {
      if (success) toast.success(success)
      void qc.invalidateQueries({ queryKey: groupsKey })
      void qc.invalidateQueries({ queryKey: directoryKey })
    },
  })
}

export const useCreateGroup = () =>
  useGroupMutation(
    async (v: { name: string; description: string }) => (await createGroup({ body: v, throwOnError: true })).data,
    null
  )

export const useUpdateGroup = () =>
  useGroupMutation(
    async (v: { id: string; name: string; description: string }) =>
      (
        await updateGroup({
          path: { id: v.id },
          body: { name: v.name, description: v.description },
          throwOnError: true,
        })
      ).data,
    null
  )

export const useSetGroupMembers = () =>
  useGroupMutation(
    async (v: { id: string; userIds: string[] }) =>
      (await setGroupMembers({ path: { id: v.id }, body: { user_ids: v.userIds }, throwOnError: true })).data,
    "Members saved"
  )

export const useDeleteGroup = () =>
  useGroupMutation(async (id: string) => {
    await deleteGroup({ path: { id }, throwOnError: true })
  }, "Group deleted")
