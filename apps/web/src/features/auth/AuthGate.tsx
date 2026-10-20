import { useEffect } from "react"
import { useNavigate, useRouterState } from "@tanstack/react-router"
import { useQueryClient } from "@tanstack/react-query"

import { StatePanel } from "@/components/state-panel"
import { onUnauthenticated, setCsrfToken } from "@/lib/api"
import { errMessage } from "../draft/queries"
import { ChangePasswordDialog } from "./ChangePasswordDialog"
import { sessionKey, useSession } from "./queries"

/**
 * Renders the app only for a signed-in user; everyone else goes to /login (which
 * remembers where they were). The check runs in the browser: the API is the
 * authority and answers 401 without a session, which also ends up here.
 */
export function AuthGate({ children }: { children: React.ReactNode }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { pathname, href } = useRouterState({
    select: (s) => ({ pathname: s.location.pathname, href: s.location.href }),
  })
  const session = useSession()
  const isLogin = pathname === "/login"
  const signedOut = session.isSuccess && session.data === null

  // Any API call answering 401 means the session ended (expired, signed out elsewhere, disabled).
  useEffect(
    () =>
      onUnauthenticated(() => {
        setCsrfToken(null)
        qc.setQueryData(sessionKey, null)
      }),
    [qc]
  )

  useEffect(() => {
    if (!isLogin && signedOut) void navigate({ to: "/login", search: { return_to: href }, replace: true })
  }, [isLogin, signedOut, href, navigate])

  if (isLogin) return children
  if (session.isError) {
    return (
      <div className="grid h-svh">
        <StatePanel tone="error" title="Could not check your session" onRetry={() => void session.refetch()}>
          {errMessage(session.error)}
        </StatePanel>
      </div>
    )
  }
  if (!session.data) {
    return (
      <div className="grid h-svh">
        <StatePanel tone="loading">Checking your session…</StatePanel>
      </div>
    )
  }
  return (
    <>
      {children}
      <ChangePasswordDialog open={session.data.me.password_must_change} forced />
    </>
  )
}
