import { describe, expect, it } from "vitest"

import { MAX_LAYERS, addLayer, moveLayer } from "./LayersEditor"

describe("addLayer", () => {
  it("appends trimmed names", () => {
    expect(addLayer(["User"], "  Services ")).toEqual(["User", "Services"])
  })

  it("ignores blanks, duplicates in any case, and anything over the limit", () => {
    expect(addLayer(["User"], "   ")).toEqual(["User"])
    expect(addLayer(["User"], "user")).toEqual(["User"])
    const full = Array.from({ length: MAX_LAYERS }, (_, i) => `L${i}`)
    expect(addLayer(full, "extra")).toEqual(full)
  })

  it("does not mutate its input", () => {
    const input = ["A"]
    addLayer(input, "B")
    expect(input).toEqual(["A"])
  })
})

describe("moveLayer", () => {
  it("moves a layer up or down", () => {
    expect(moveLayer(["A", "B", "C"], 1, -1)).toEqual(["B", "A", "C"])
    expect(moveLayer(["A", "B", "C"], 0, 1)).toEqual(["B", "A", "C"])
    expect(moveLayer(["A", "B", "C"], 0, 2)).toEqual(["B", "C", "A"])
  })

  it("leaves the list alone when it cannot move", () => {
    expect(moveLayer(["A", "B"], 0, -1)).toEqual(["A", "B"])
    expect(moveLayer(["A", "B"], 1, 1)).toEqual(["A", "B"])
    expect(moveLayer(["A", "B"], 5, -1)).toEqual(["A", "B"])
  })
})
