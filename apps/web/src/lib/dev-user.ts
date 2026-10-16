import { client } from "@archmap/api-client"

// Development stand-in for session auth: the API (AUTH_DEV_HEADER=true) identifies callers by
// an X-Dev-User header. The top bar's user switcher stores the choice in localStorage and this
// interceptor sends it; without a choice the Vite proxy falls back to $DEV_USER.
const KEY = "archmap.devUser"

export const devUsers = ["viewer@archmap.local", "editor@archmap.local", "maintainer@archmap.local"] as const

export function getDevUser(): string | null {
  try {
    return localStorage.getItem(KEY)
  } catch {
    return null
  }
}

export function setDevUser(email: string) {
  try {
    localStorage.setItem(KEY, email)
  } catch {
    // storage unavailable: the choice lasts only until reload via the proxy default
  }
}

let installed = false

export function installDevUserHeader() {
  if (installed || !import.meta.env.DEV) return
  installed = true
  client.interceptors.request.use((request) => {
    const user = getDevUser()
    if (user) request.headers.set("X-Dev-User", user)
    return request
  })
}
