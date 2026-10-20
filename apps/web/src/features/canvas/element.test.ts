import { describe, expect, it } from "vitest"

import { ownerInput } from "../draft/OwnersDialog"
import { urlProblem } from "../draft/ElementFields"
import { daysSinceReview, effectiveOwner, isDatastore, today, typesForKind } from "./element"

describe("element vocabulary", () => {
  it("draws only datastore types as cylinders", () => {
    expect(isDatastore({ type: "database-server" })).toBe(true)
    expect(isDatastore({ type: "schema" })).toBe(true)
    expect(isDatastore({ type: "cache" })).toBe(false)
    expect(isDatastore({ type: "" })).toBe(false)
  })

  it("offers the types that fit a kind (mirrors the API's type-kind rule)", () => {
    expect(typesForKind("container").map((t) => t.value)).toEqual([
      "database-server",
      "database",
      "cache",
      "queue",
      "proxy",
    ])
    expect(typesForKind("component").map((t) => t.value)).toContain("schema")
    expect(typesForKind("node").map((t) => t.value)).toEqual(["proxy"])
    expect(typesForKind("person")).toEqual([])
  })

  it("tells own and inherited owners apart", () => {
    const eff = {
      environment: "" as const,
      environment_source_id: null,
      deployed_via_id: null,
      owners: [
        { role: "owner" as const, owner_id: "team-a", source_element_id: "parent" },
        { role: "oncall" as const, owner_id: "team-b", source_element_id: "me" },
      ],
    }
    expect(effectiveOwner(eff, "me")).toEqual({ ownerId: "team-a", sourceId: "parent", inherited: true })
    expect(effectiveOwner(eff, "me", "oncall")?.inherited).toBe(false)
    expect(effectiveOwner(eff, "me", "escalation")).toBeNull()
    expect(effectiveOwner(undefined, "me")).toBeNull()
  })

  it("counts review age in days", () => {
    const now = new Date("2026-10-19T12:00:00Z")
    expect(daysSinceReview("2026-10-09", now)).toBe(10)
    expect(daysSinceReview(null, now)).toBeNull()
    expect(today(new Date(2026, 0, 5))).toBe("2026-01-05")
  })

  it("checks links like the API", () => {
    expect(urlProblem("")).toBeNull()
    expect(urlProblem("https://runbooks.example/x")).toBeNull()
    expect(urlProblem("ftp://x.example")).not.toBeNull()
    expect(urlProblem("runbooks.example")).not.toBeNull()
  })
})

describe("ownerInput", () => {
  const atlas = { id: "o1", kind: "team" as const, name: "Atlas", contact: "", oncall_url: "", version: 1 }
  it("adds a new owner with its fields", () => {
    expect(ownerInput(null, { kind: "person", name: "Alex", contact: "a@x.example", oncall_url: "" }, "new")).toEqual({
      op: "add",
      target_type: "owner",
      target_id: "new",
      patch: { kind: "person", name: "Alex", contact: "a@x.example", oncall_url: "" },
    })
  })
  it("updates only what changed", () => {
    expect(ownerInput(atlas, { ...atlas, contact: "#atlas" }, "x")?.patch).toEqual({ contact: "#atlas" })
    expect(ownerInput(atlas, { ...atlas }, "x")).toBeNull()
  })
})
