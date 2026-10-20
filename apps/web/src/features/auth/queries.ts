import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import {
  changeMyPassword,
  getSession,
  listAuthProviders,
  login,
  logout,
  type LoginRequest,
  type Session,
} from "@archmap/api-client"
import { toast } from "sonner"

import { setCsrfToken } from "@/lib/api"
import { errMessage } from "../draft/queries"

export const sessionKey = ["session"] as const
const providersKey = ["auth", "providers"] as const

/** Loads the session; null when signed out (401). Keeps the CSRF token for the API client. */
export async function fetchSession(): Promise<Session | null> {
  const res = await getSession()
  if (res.response?.status === 401) {
    setCsrfToken(null)
    return null
  }
  if (res.error !== undefined) throw new Error(errMessage(res.error))
  setCsrfToken(res.data.csrf_token)
  return res.data
}

export const useSession = () =>
  useQuery({ queryKey: sessionKey, queryFn: fetchSession, retry: false, staleTime: 60_000 })

export const useAuthProviders = () =>
  useQuery({
    queryKey: providersKey,
    queryFn: async () => (await listAuthProviders({ throwOnError: true })).data.providers,
    staleTime: 5 * 60_000,
  })

/** Forget everything cached for the previous user and adopt the new session. */
function adopt(qc: QueryClient, session: Session | null) {
  setCsrfToken(session?.csrf_token ?? null)
  qc.removeQueries({ predicate: (q) => q.queryKey[0] !== sessionKey[0] && q.queryKey[0] !== providersKey[0] })
  qc.setQueryData(sessionKey, session)
}

export function useLogin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: LoginRequest) => (await login({ body, throwOnError: true })).data,
    onSuccess: (session) => adopt(qc, session),
  })
}

export function useLogout() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      await logout({ throwOnError: true })
    },
    onSuccess: () => adopt(qc, null),
    onError: (e) => toast.error(errMessage(e)),
  })
}

export function useChangePassword() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: { current: string; next: string }) => {
      await changeMyPassword({ body: { current_password: v.current, new_password: v.next }, throwOnError: true })
    },
    onSuccess: () => {
      toast.success("Password changed. Other sessions were signed out.")
      void qc.invalidateQueries({ queryKey: sessionKey })
      void qc.invalidateQueries({ queryKey: ["me"] })
    },
  })
}
