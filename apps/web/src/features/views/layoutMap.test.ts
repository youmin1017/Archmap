import { describe, expect, it } from "vitest"
import type { ViewElement } from "@archmap/api-client"

import { layoutToState, stateToLayout } from "./layoutMap"

const entry = (id: string, over: Partial<ViewElement> = {}): ViewElement => ({
  element_id: id,
  collapsed: true,
  ...over,
})
const known = new Set(["a", "b", "c"])

describe("layoutToState", () => {
  it("restores positions and expanded elements, dropping unknown ids", () => {
    const out = layoutToState(
      [entry("a", { x: 1, y: 2 }), entry("b", { collapsed: false }), entry("gone", { x: 9, y: 9, collapsed: false })],
      ["c"],
      known
    )
    expect(out.positions).toEqual({ a: { x: 1, y: 2 } })
    expect(out.expanded).toEqual(["b"])
  })

  it("falls back to the server defaults when nothing was laid out yet", () => {
    expect(layoutToState([], ["c", "unknown"], known).expanded).toEqual(["c"])
    // Rows that only carry custom-view membership are not a saved layout.
    expect(layoutToState([entry("a", { included: true })], ["c"], known).expanded).toEqual(["c"])
  })

  it("ignores half-set coordinates", () => {
    expect(layoutToState([entry("a", { x: 1, y: null })], [], known).positions).toEqual({})
  })
})

describe("stateToLayout", () => {
  it("lists moved and expanded elements only", () => {
    const out = stateToLayout({ a: { x: 5, y: 6 } }, new Set(["b"]), known)
    expect(out).toEqual([
      { element_id: "a", x: 5, y: 6, w: null, h: null, collapsed: true },
      { element_id: "b", x: null, y: null, w: null, h: null, collapsed: false },
    ])
  })

  it("keeps saved entries of elements that are not shown here", () => {
    const saved = [entry("draft-only", { x: 3, y: 4 }), entry("c", { x: 7, y: 7 }), entry("empty-unknown")]
    const out = stateToLayout({ a: { x: 1, y: 1 } }, new Set(), known, saved)
    expect(out.map((e) => e.element_id)).toEqual(["a", "draft-only"]) // c is known (re-derived), empty rows dropped
    expect(out[1]).toMatchObject({ x: 3, y: 4, collapsed: true })
  })

  it("round-trips", () => {
    const state = { positions: { a: { x: 1, y: 2 } }, expanded: new Set(["b"]) }
    const back = layoutToState(stateToLayout(state.positions, state.expanded, known), [], known)
    expect(back.positions).toEqual(state.positions)
    expect(back.expanded).toEqual(["b"])
  })
})
