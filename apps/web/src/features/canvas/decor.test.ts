import { describe, expect, it } from "vitest"

import { edgeDiffStatus, type DiffStatus } from "./decor"

const m = (e: [string, DiffStatus][]) => new Map(e)

describe("edgeDiffStatus", () => {
  it("is undefined without an overlay or without touched relations", () => {
    expect(edgeDiffStatus(["a"], undefined)).toBeUndefined()
    expect(edgeDiffStatus(["a"], m([]))).toBeUndefined()
    expect(edgeDiffStatus(["a", "b"], m([["x", "added"]]))).toBeUndefined()
  })

  it("keeps a uniform status", () => {
    expect(
      edgeDiffStatus(
        ["a", "b"],
        m([
          ["a", "removed"],
          ["b", "removed"],
        ])
      )
    ).toBe("removed")
    expect(edgeDiffStatus(["a"], m([["a", "added"]]))).toBe("added")
  })

  it("reports mixed or partially changed aggregates as modified", () => {
    expect(
      edgeDiffStatus(
        ["a", "b"],
        m([
          ["a", "added"],
          ["b", "removed"],
        ])
      )
    ).toBe("modified")
    expect(edgeDiffStatus(["a", "b"], m([["a", "added"]]))).toBe("modified")
    expect(edgeDiffStatus(["a"], m([["a", "modified"]]))).toBe("modified")
  })
})
