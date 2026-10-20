import { describe, expect, it } from "vitest"
import type { ChangeInput, Version } from "@archmap/api-client"

import { directSummary, groupVersions, undoTarget } from "./direct"

const names: Record<string, string> = { a: "API", w: "Web", d: "DB" }
const nameOf = (id: string) => names[id] ?? "unknown"
const el = (op: ChangeInput["op"], id: string, patch?: Record<string, unknown>): ChangeInput => ({
  op,
  target_type: "element",
  target_id: id,
  patch,
})

describe("directSummary", () => {
  it.each<[string, ChangeInput[], string]>([
    ["add", [el("add", "n", { name: "Billing" }), el("add", "m", { name: "Invoices" })], "Add Billing and 1 more"],
    ["cascade delete names the requested element", [el("delete", "d"), el("delete", "a")], "Delete API"],
    [
      "edit with ownership",
      [el("update", "a", { name: "x" }), { op: "add", target_type: "ownership", patch: {} }],
      "Edit API",
    ],
    ["two elements", [el("update", "a"), el("update", "w"), el("update", "a")], "Edit API and 1 more"],
    [
      "connect",
      [{ op: "add", target_type: "relation", target_id: "r", patch: { source_id: "w", target_id: "a" } }],
      "Connect Web → API",
    ],
    ["owners only", [{ op: "add", target_type: "owner", patch: {} }], "Edit owners"],
  ])("%s", (_name, inputs, want) => {
    expect(directSummary(inputs, nameOf)).toBe(want)
  })
})

const v = (number: number, over: Partial<Version> = {}): Version => ({
  number,
  changeset_id: `cs${number}`,
  merged_by: "me",
  merged_at: "2030-01-01T00:00:00Z",
  summary: `v${number}`,
  kind: "direct",
  reverts: null,
  ...over,
})

describe("undoTarget", () => {
  it("picks my newest direct edit that is neither an undo nor undone", () => {
    const versions = [v(5, { reverts: "cs4" }), v(4), v(3, { merged_by: "other" }), v(2), v(1, { kind: "draft" })]
    expect(undoTarget(versions, "me")?.number).toBe(2)
    expect(undoTarget(versions, "other")?.number).toBe(3)
    expect(undoTarget([v(1, { kind: "draft" })], "me")).toBeUndefined()
    expect(undoTarget(versions, undefined)).toBeUndefined()
  })
})

describe("groupVersions", () => {
  it("groups consecutive direct edits by the same person", () => {
    const groups = groupVersions([v(6), v(5), v(4, { merged_by: "other" }), v(3, { kind: "draft" }), v(2), v(1)])
    expect(groups.map((g) => g.versions.map((x) => x.number))).toEqual([[6, 5], [4], [3], [2, 1]])
  })
})
