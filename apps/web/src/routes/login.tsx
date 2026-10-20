import { useEffect, useState } from "react"
import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { ChevronDown, Network } from "lucide-react"
import type { AuthProvider } from "@archmap/api-client"

import { StatePanel } from "@/components/state-panel"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"
import { loginErrors, loginModel, safeReturnTo } from "@/features/auth/client"
import { useAuthProviders, useLogin, useSession } from "@/features/auth/queries"
import { errMessage } from "@/features/draft/queries"

type LoginSearch = { return_to?: string; error?: string }

export const Route = createFileRoute("/login")({
  validateSearch: (s: Record<string, unknown>): LoginSearch => ({
    return_to: typeof s.return_to === "string" ? s.return_to : undefined,
    error: typeof s.error === "string" ? s.error : undefined,
  }),
  head: () => ({ meta: [{ title: "Sign in · Archmap" }] }),
  component: LoginPage,
})

const heading = "text-[11px] font-medium tracking-wide text-muted-foreground uppercase"

const devAccounts = [
  { email: "viewer@archmap.local", role: "Viewer", can: "Browse only" },
  { email: "editor@archmap.local", role: "Editor", can: "Create and edit drafts" },
  { email: "maintainer@archmap.local", role: "Maintainer", can: "Review, merge, manage users" },
]

function LoginPage() {
  const search = Route.useSearch()
  const navigate = useNavigate()
  const session = useSession()
  const providers = useAuthProviders()
  const returnTo = safeReturnTo(search.return_to)

  // Already signed in (or just signed in): continue where the user wanted to go.
  useEffect(() => {
    if (session.data) void navigate({ to: returnTo, replace: true })
  }, [session.data, returnTo, navigate])

  const model = providers.data ? loginModel(providers.data, import.meta.env.DEV) : null
  const redirectError = search.error ? (loginErrors[search.error] ?? loginErrors.error) : null

  return (
    <main className="grid min-h-svh place-items-center bg-muted/30 p-4">
      <div className="flex w-full max-w-sm flex-col gap-6">
        <div className="flex flex-col items-center gap-3 text-center">
          <span className="grid size-10 place-items-center rounded-lg bg-linear-to-br from-primary to-[oklch(0.55_0.2_300)] text-primary-foreground shadow-sm">
            <Network className="size-5" aria-hidden />
          </span>
          <h1 className="font-heading text-xl font-semibold tracking-tight">Sign in to Archmap</h1>
        </div>

        {redirectError && (
          <p
            role="alert"
            className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive"
          >
            {redirectError}
          </p>
        )}

        {providers.isPending && <StatePanel tone="loading">Loading sign-in options…</StatePanel>}
        {providers.isError && (
          <StatePanel tone="error" title="Could not reach the API" onRetry={() => void providers.refetch()}>
            {errMessage(providers.error)}
          </StatePanel>
        )}
        {model && <SignInOptions model={model} />}
      </div>
    </main>
  )
}

function SignInOptions({ model }: { model: ReturnType<typeof loginModel> }) {
  const search = Route.useSearch()
  const [adminOpen, setAdminOpen] = useState(false)
  const returnTo = safeReturnTo(search.return_to)
  const nothing = !model.password && model.redirects.length === 0 && !model.restricted && !model.dev

  return (
    <>
      {(model.password || model.redirects.length > 0) && (
        <div className="flex flex-col gap-4 rounded-lg border bg-card p-5 shadow-xs">
          {model.redirects.map((p) => (
            <Button
              key={p.id}
              variant="outline"
              className="w-full"
              onClick={() =>
                window.location.assign(
                  `/api/v1/auth/${encodeURIComponent(p.id)}/start?return_to=${encodeURIComponent(returnTo)}`
                )
              }
            >
              Continue with {p.name}
            </Button>
          ))}
          {model.password && model.redirects.length > 0 && (
            <div className="flex items-center gap-3 text-xs text-muted-foreground" aria-hidden>
              <span className="h-px flex-1 bg-border" /> or <span className="h-px flex-1 bg-border" />
            </div>
          )}
          {model.password && <PasswordForm provider={model.password} />}
        </div>
      )}

      {model.restricted && (
        <div className="flex flex-col gap-3">
          <button
            type="button"
            aria-expanded={adminOpen}
            aria-controls="admin-sign-in"
            onClick={() => setAdminOpen((o) => !o)}
            className="inline-flex items-center justify-center gap-1 self-center rounded-md px-2 text-xs text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            {model.restricted.name}
            <ChevronDown className={cn("size-3.5 transition-transform", adminOpen && "rotate-180")} aria-hidden />
          </button>
          {adminOpen && (
            <div id="admin-sign-in" className="rounded-lg border bg-card p-5">
              <PasswordForm provider={model.restricted} />
            </div>
          )}
        </div>
      )}

      {model.dev && <DevSignIn provider={model.dev} />}

      {nothing && <StatePanel title="No sign-in method is configured">Ask an administrator.</StatePanel>}
    </>
  )
}

function PasswordForm({ provider }: { provider: AuthProvider }) {
  const signIn = useLogin()
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [touched, setTouched] = useState({ email: false, password: false })
  const emailOk = /\S+@\S+/.test(email.trim())
  const canSubmit = emailOk && password !== "" && !signIn.isPending
  const id = (f: string) => `${provider.id}-${f}`

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        setTouched({ email: true, password: true })
        if (!canSubmit) return
        signIn.mutate({ provider: provider.id, username: email.trim(), password })
      }}
      noValidate
    >
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={id("email")}>Email</Label>
        <Input
          id={id("email")}
          type="email"
          autoComplete="username"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          onBlur={(e) => e.target.value && setTouched((t) => ({ ...t, email: true }))}
          aria-invalid={touched.email && !emailOk}
          aria-describedby={touched.email && !emailOk ? id("email-error") : undefined}
          autoFocus
        />
        {touched.email && !emailOk && (
          <p id={id("email-error")} className="text-xs text-destructive">
            Enter the email you sign in with.
          </p>
        )}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={id("password")}>Password</Label>
        <Input
          id={id("password")}
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          onBlur={(e) => e.target.value && setTouched((t) => ({ ...t, password: true }))}
          aria-invalid={touched.password && password === ""}
        />
      </div>
      {signIn.isError && (
        <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {errMessage(signIn.error)}
        </p>
      )}
      <Button type="submit" className="w-full" disabled={signIn.isPending}>
        {signIn.isPending ? "Signing in…" : "Sign in"}
      </Button>
    </form>
  )
}

/** Development builds only: one click signs in as a seeded account. */
function DevSignIn({ provider }: { provider: AuthProvider }) {
  const signIn = useLogin()
  return (
    <section aria-labelledby="dev-sign-in" className="flex flex-col gap-2">
      <h2 id="dev-sign-in" className={cn(heading, "flex items-center gap-2")}>
        <span className="rounded-sm bg-amber-500/15 px-1 font-mono text-[10px] font-semibold text-amber-700 dark:text-amber-400">
          dev
        </span>
        Sign in as a seeded account
      </h2>
      <div className="grid grid-cols-3 gap-2">
        {devAccounts.map((a) => (
          <button
            key={a.email}
            type="button"
            disabled={signIn.isPending}
            onClick={() => signIn.mutate({ provider: provider.id, username: a.email })}
            className="flex flex-col items-start gap-0.5 rounded-lg border bg-card px-3 py-2 text-left transition-colors hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:opacity-60"
          >
            <span className="text-sm font-medium">{a.role}</span>
            <span className="text-xs text-muted-foreground">{a.can}</span>
          </button>
        ))}
      </div>
      {signIn.isError && (
        <p role="alert" className="text-xs text-destructive">
          {errMessage(signIn.error)}
        </p>
      )}
    </section>
  )
}
