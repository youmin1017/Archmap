import { describe, expect, it, vi } from "vitest"
import type { AuthProvider } from "@archmap/api-client"

import { CSRF_HEADER, CSRF_PROBLEM, createAuthFetch, loginModel, safeReturnTo } from "./client"

const problem = (status: number, type?: string) =>
  new Response(JSON.stringify({ status, title: "x", type }), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  })

function setup(responses: Response[], token: string | null = "t1") {
  const seen: Request[] = []
  const deps = {
    fetch: vi.fn(async (r: Request) => {
      seen.push(r)
      return responses.shift() ?? new Response(null, { status: 204 })
    }),
    getToken: () => token,
    refreshToken: vi.fn(async () => "t2"),
    onUnauthenticated: vi.fn(),
  }
  return { f: createAuthFetch(deps), deps, seen }
}

describe("createAuthFetch", () => {
  it("adds the CSRF token to unsafe requests only", async () => {
    const { f, seen } = setup([new Response("{}"), new Response("{}")])
    await f(new Request("http://app/api/v1/changesets", { method: "POST", body: "{}" }))
    await f(new Request("http://app/api/v1/changesets"))
    expect(seen[0].headers.get(CSRF_HEADER)).toBe("t1")
    expect(seen[1].headers.get(CSRF_HEADER)).toBeNull()
  })

  it("refreshes a stale token and retries once", async () => {
    const { f, deps, seen } = setup([problem(403, CSRF_PROBLEM), new Response("{}", { status: 201 })])
    const res = await f(new Request("http://app/api/v1/changesets", { method: "POST", body: '{"title":"x"}' }))
    expect(res.status).toBe(201)
    expect(deps.refreshToken).toHaveBeenCalledOnce()
    expect(seen).toHaveLength(2)
    expect(seen[1].headers.get(CSRF_HEADER)).toBe("t2")
    expect(await seen[1].text()).toBe('{"title":"x"}')
  })

  it("does not retry other 403s", async () => {
    const { f, deps } = setup([problem(403)])
    const res = await f(new Request("http://app/api/v1/users", { method: "POST", body: "{}" }))
    expect(res.status).toBe(403)
    expect(deps.refreshToken).not.toHaveBeenCalled()
  })

  it("reports 401s except from the auth endpoints", async () => {
    const { f, deps } = setup([problem(401), problem(401)])
    await f(new Request("http://app/api/v1/auth/session"))
    expect(deps.onUnauthenticated).not.toHaveBeenCalled()
    await f(new Request("http://app/api/v1/model/elements"))
    expect(deps.onUnauthenticated).toHaveBeenCalledOnce()
  })
})

describe("safeReturnTo", () => {
  it.each([
    ["/changesets/1?x=y", "/changesets/1?x=y"],
    ["/", "/"],
    ["", "/"],
    [undefined, "/"],
    ["//evil.example", "/"],
    ["/\\evil.example", "/"],
    ["https://evil.example", "/"],
    ["javascript:alert(1)", "/"],
    ["/login?return_to=/x", "/"],
  ])("%s -> %s", (input, want) => {
    expect(safeReturnTo(input)).toBe(want)
  })
})

const p = (id: string, kind: AuthProvider["kind"], extra: Partial<AuthProvider> = {}): AuthProvider => ({
  id,
  name: id,
  kind,
  restricted: false,
  username_only: false,
  ...extra,
})

describe("loginModel", () => {
  it("shows the password form and dev sign-in in development", () => {
    const m = loginModel([p("local", "password"), p("dev", "password", { username_only: true })], true)
    expect(m.password?.id).toBe("local")
    expect(m.dev?.id).toBe("dev")
    expect(m.restricted).toBeNull()
  })

  it("never offers dev sign-in in production builds", () => {
    expect(loginModel([p("local", "password"), p("dev", "password", { username_only: true })], false).dev).toBeNull()
  })

  it("tucks a restricted local password away behind SSO", () => {
    const m = loginModel([p("local", "password", { restricted: true }), p("corp", "redirect")], false)
    expect(m.password).toBeNull()
    expect(m.redirects.map((r) => r.id)).toEqual(["corp"])
    expect(m.restricted?.id).toBe("local")
  })

  it("keeps a restricted password as the main form when it is the only way in", () => {
    const m = loginModel([p("local", "password", { restricted: true })], false)
    expect(m.password?.id).toBe("local")
    expect(m.restricted).toBeNull()
  })
})
