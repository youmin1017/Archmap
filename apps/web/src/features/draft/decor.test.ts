import { describe, expect, it } from "vitest"
import type { Change, Diff, LintReport } from "@archmap/api-client"

import { ids, sampleElements, sampleRelations } from "../canvas/fixtures"
import { describeBatches, describeChange, diffOverlay, formatValue, lintMarks } from "./decor"

const el = (id: string) => sampleElements.find((e) => e.id === id)!
const rel = (n: string) => sampleRelations.find((r) => r.id.endsWith(n.padStart(12, "0")))!
const nameOf = (id: string) => sampleElements.find((e) => e.id === id)?.name ?? "?"

describe("diffOverlay", () => {
  const diff: Diff = {
    base_version: 1,
    main_version: 1,
    stale: false,
    elements: {
      added: [el(ids.web)],
      removed: [el(ids.worker)],
      modified: [{ before: el(ids.api), after: el(ids.api), fields: [{ field: "name", before: "API", after: "X" }] }],
    },
    relations: { added: [], removed: [rel("106")], modified: [] },
  }

  it("maps ids to statuses and collects ghosts", () => {
    const o = diffOverlay(diff)
    expect(o.elements.get(ids.web)).toBe("added")
    expect(o.elements.get(ids.api)).toBe("modified")
    expect(o.elements.get(ids.worker)).toBe("removed")
    expect(o.relations.get("00000000-0000-7000-8000-000000000106")).toBe("removed")
    expect(o.removedElements.map((e) => e.id)).toEqual([ids.worker])
    expect(o.removedRelations).toHaveLength(1)
  })

  it("is empty without a diff", () => {
    expect(diffOverlay(undefined).elements.size).toBe(0)
  })
})

describe("lintMarks", () => {
  const report: LintReport = {
    errors: 1,
    warnings: 2,
    findings: [
      { rule: "hierarchy", severity: "error", message: "m", target_type: "element", target_id: ids.api },
      { rule: "orphan", severity: "warning", message: "m", target_type: "element", target_id: ids.api },
      { rule: "orphan", severity: "warning", message: "m", target_type: "element", target_id: ids.pg },
      { rule: "uses-technology", severity: "warning", message: "m", target_type: "relation", target_id: rel("103").id },
    ],
  }

  it("keeps the worst severity per element and marks relation endpoints", () => {
    const marks = lintMarks(report, sampleRelations)
    expect(marks.get(ids.api)).toBe("error") // error wins over warning, whatever the order
    expect(marks.get(ids.pg)).toBe("warning")
    expect(marks.has(ids.web)).toBe(false)
  })

  it("is empty without a report", () => {
    expect(lintMarks(undefined, []).size).toBe(0)
  })
})

describe("describeBatches", () => {
  const base = {
    changeset_id: "c",
    target_type: "element",
    before: null,
    undone_at: null,
    created_at: "2030-01-01T00:00:00Z",
  } as const
  const change = (over: Partial<Change>): Change => ({
    id: "x",
    seq: 1,
    batch_id: "b1",
    op: "add",
    target_id: ids.api,
    patch: {},
    ...base,
    ...over,
  })

  it("describes each operation", () => {
    expect(describeChange(change({ op: "add", patch: { name: "Cache" } }), nameOf)).toBe("Added Cache")
    expect(
      describeChange(
        change({ op: "update", patch: { name: "X", technology: "Rust" }, before: { name: "API" } }),
        nameOf
      )
    ).toBe("Changed name, technology of API")
    expect(describeChange(change({ op: "delete", before: { name: "Worker" } }), nameOf)).toBe("Deleted Worker")
    expect(
      describeChange(
        change({ op: "add", target_type: "relation", patch: { source_id: ids.web, target_id: ids.api } }),
        nameOf
      )
    ).toBe("Added relation Web Frontend → API")
  })

  it("groups by batch, newest first, and flags fully undone batches", () => {
    const out = describeBatches(
      [
        change({ batch_id: "b1", seq: 1, patch: { name: "A" } }),
        change({ batch_id: "b2", seq: 2, patch: { name: "B" }, undone_at: "2030-01-02T00:00:00Z" }),
        change({ batch_id: "b2", seq: 3, patch: { name: "C" }, undone_at: "2030-01-02T00:00:00Z" }),
      ],
      nameOf
    )
    expect(out.map((b) => b.batchId)).toEqual(["b2", "b1"])
    expect(out[0]).toMatchObject({ undone: true, lines: ["Added B", "Added C"] })
    expect(out[1]?.undone).toBe(false)
  })
})

describe("formatValue", () => {
  it.each([
    [null, "(none)"],
    [undefined, "(none)"],
    ["", "(empty)"],
    ["Go", "Go"],
    [3, "3"],
    [true, "true"],
    [[], "(none)"],
    [["a", "b"], "a, b"],
    [{ name: "API", kind: "container" }, "API"],
    [{ source_id: "x" }, '{"source_id":"x"}'],
  ])("%j -> %s", (input, out) => {
    expect(formatValue(input)).toBe(out)
  })
})
