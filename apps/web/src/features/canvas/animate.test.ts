import { describe, expect, it } from "vitest"

import { easeOutCubic, lerpBoxes } from "./animate"
import type { Box } from "./layout"

const box = (x: number, y: number, width: number, height: number): Box => ({ x, y, width, height })

describe("lerpBoxes", () => {
  const from = new Map([["a", box(0, 0, 100, 50)]])
  const to = new Map([
    ["a", box(100, 200, 300, 150)],
    ["new", box(5, 5, 10, 10)],
  ])

  it("interpolates shared boxes and places new boxes at their target", () => {
    expect(lerpBoxes(from, to, 0).get("a")).toEqual(box(0, 0, 100, 50))
    expect(lerpBoxes(from, to, 0.5).get("a")).toEqual(box(50, 100, 200, 100))
    expect(lerpBoxes(from, to, 1).get("a")).toEqual(box(100, 200, 300, 150))
    expect(lerpBoxes(from, to, 0.25).get("new")).toEqual(box(5, 5, 10, 10))
  })

  it("drops boxes that are no longer in the target", () => {
    expect([...lerpBoxes(to, from, 0.5).keys()]).toEqual(["a"])
  })
})

describe("easeOutCubic", () => {
  it("runs from 0 to 1, fast first", () => {
    expect(easeOutCubic(0)).toBe(0)
    expect(easeOutCubic(1)).toBe(1)
    expect(easeOutCubic(0.5)).toBeGreaterThan(0.5)
  })
})
