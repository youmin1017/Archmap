import type { AuthProvider } from "@archmap/api-client"

// Session plumbing for the API client (ADR-055, ADR-056). The session itself is an
// HttpOnly cookie the browser sends on its own; this module only adds the CSRF token
// to unsafe requests, retries once when the token went stale, and reports 401s so
// the app can send the user to the login page.

export const CSRF_HEADER = "X-CSRF-Token"
export const CSRF_PROBLEM = "https://archmap.dev/problems/csrf"
export const PASSWORD_CHANGE_PROBLEM = "https://archmap.dev/problems/password-change-required"

const safeMethods = new Set(["GET", "HEAD", "OPTIONS"])

type Deps = {
  fetch: (request: Request) => Promise<Response>
  /** The current session's CSRF token, if known. */
  getToken: () => string | null
  /** Reloads the session and returns its CSRF token (null when signed out). */
  refreshToken: () => Promise<string | null>
  /** Called when an API request (other than /auth/*) answers 401. */
  onUnauthenticated: () => void
}

async function problemType(res: Response): Promise<string | undefined> {
  try {
    const body = (await res.clone().json()) as { type?: unknown }
    return typeof body.type === "string" ? body.type : undefined
  } catch {
    return undefined
  }
}

/** A fetch for the generated client that handles the CSRF token and expired sessions. */
export function createAuthFetch(deps: Deps) {
  return async (request: Request): Promise<Response> => {
    const unsafe = !safeMethods.has(request.method.toUpperCase())
    const spare = unsafe ? request.clone() : null
    const send = (req: Request, token: string | null) => {
      if (unsafe && token) req.headers.set(CSRF_HEADER, token)
      return deps.fetch(req)
    }

    let res = await send(request, deps.getToken())
    if (spare && res.status === 403 && (await problemType(res)) === CSRF_PROBLEM) {
      const token = await deps.refreshToken()
      if (token) res = await send(spare, token)
    }
    if (res.status === 401 && !new URL(request.url, "http://x").pathname.startsWith("/api/v1/auth/")) {
      deps.onUnauthenticated()
    }
    return res
  }
}

/** Keeps post-login navigation on this site (mirrors auth.SafeReturnTo on the server). */
export function safeReturnTo(value: unknown): string {
  if (typeof value !== "string" || !value.startsWith("/") || value.startsWith("//") || /[\\\r\n\t]/.test(value)) {
    return "/"
  }
  if (value === "/login" || value.startsWith("/login?")) return "/"
  return value
}

/** Messages for the error codes a redirect sign-in sends back to /login. */
export const loginErrors: Record<string, string> = {
  failed: "Sign-in did not complete. Start again.",
  denied:
    "Your sign-in was refused: no account is linked to it, the account is disabled, or it must use another method.",
  rate_limited: "Too many failed sign-in attempts. Wait a few minutes and try again.",
  unknown_provider: "That sign-in method is not available.",
  error: "Sign-in failed because of a server error.",
}

export type LoginModel = {
  /** The main email + password form (not shown when it is restricted to maintainers). */
  password: AuthProvider | null
  /** Identity providers, one button each. */
  redirects: AuthProvider[]
  /** Local password limited to maintainers: tucked away as "Administrator sign-in". */
  restricted: AuthProvider | null
  /** Development sign-in (no password); only offered by dev builds. */
  dev: AuthProvider | null
}

/** How the login page lays out the server's providers. */
export function loginModel(providers: AuthProvider[], devBuild: boolean): LoginModel {
  const passwordKind = providers.filter((p) => p.kind === "password")
  const dev = devBuild ? (passwordKind.find((p) => p.username_only) ?? null) : null
  const forms = passwordKind.filter((p) => !p.username_only)
  const redirects = providers.filter((p) => p.kind === "redirect")
  const open = forms.find((p) => !p.restricted) ?? null
  // A restricted provider is still the main form when nothing else lets people in.
  const restricted = forms.find((p) => p.restricted) ?? null
  const fallback = !open && redirects.length === 0 && !dev ? restricted : null
  return {
    password: open ?? fallback,
    redirects,
    restricted: fallback ? null : restricted,
    dev,
  }
}
