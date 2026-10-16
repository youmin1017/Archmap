import { describe, expect, it } from "vitest"

import { resolveTechIcon } from "./resolve"

describe("resolveTechIcon", () => {
  it.each([
    ["postgres", "simple"],
    ["PostgreSQL", "simple"],
    ["k8s", "simple"],
    ["Node.js", "simple"],
    ["Traefik", "simple"],
    ["  GoLang ", "simple"],
    ["PostgreSQL 16", "simple"], // first matching word
    ["Go / Echo", "simple"],
    ["Spring Boot", "logos"], // not in simple-icons: colored fallback
    [".NET", "logos"],
  ] as const)("%s resolves from %s", (input, source) => {
    expect(resolveTechIcon(input)?.source).toBe(source)
  })

  it("is case-insensitive and stable across spellings", () => {
    expect(resolveTechIcon("K8S")?.data).toBe(resolveTechIcon("kubernetes")?.data)
    expect(resolveTechIcon("postgres")?.data).toBe(resolveTechIcon("PostgreSQL")?.data)
  })

  it.each(["", "   ", "my in-house thing", "JSON/HTTPS"])("falls back for %j", (input) => {
    expect(resolveTechIcon(input)).toBeUndefined()
  })

  it("flags dark colored logos for contrast handling", () => {
    expect(resolveTechIcon("IBM")?.lowContrastOnDark).toBe(true)
    expect(resolveTechIcon("Spring")?.lowContrastOnDark).toBe(false)
  })
})
