import { client, getSession } from "@archmap/api-client"

import { createAuthFetch } from "@/features/auth/client"

// Same-origin: the Vite dev server proxies /api to the Go API (see vite.config.ts), so the
// HttpOnly session cookie is sent automatically. The CSRF token lives here in memory.
let csrfToken: string | null = null
const unauthenticated = new Set<() => void>()

export function setCsrfToken(token: string | null) {
  csrfToken = token
}

/** Subscribe to "an API call answered 401" (the session ended); returns the unsubscribe. */
export function onUnauthenticated(fn: () => void): () => void {
  unauthenticated.add(fn)
  return () => unauthenticated.delete(fn)
}

const authFetch = createAuthFetch({
  fetch: (request) => fetch(request),
  getToken: () => csrfToken,
  refreshToken: async () => {
    const res = await getSession()
    csrfToken = res.data?.csrf_token ?? null
    return csrfToken
  },
  onUnauthenticated: () => unauthenticated.forEach((fn) => fn()),
})

client.setConfig({
  baseUrl: "",
  fetch: (input: RequestInfo | URL, init?: RequestInit) =>
    authFetch(input instanceof Request ? input : new Request(input, init)),
})
