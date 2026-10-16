import { beforeEach, describe, expect, it } from "vitest"

import { useCanvasStore } from "./store"

const state = () => useCanvasStore.getState()

describe("canvas store revision", () => {
  beforeEach(() => {
    state().loadLayout({}, [])
    useCanvasStore.setState({ revision: 0, selectedId: null })
  })

  it("counts layout changes the user makes", () => {
    state().toggle("a")
    state().expand(["b"])
    state().collapse("a")
    state().setExpanded(["c"])
    state().setPosition("a", { x: 1, y: 2 })
    state().resetPositions()
    state().reveal("d", ["e"])
    expect(state().revision).toBe(7)
  })

  it("does not count selection, filters or loading a saved layout", () => {
    state().select("a")
    state().selectRelations(["r"])
    state().setQuery("x")
    state().loadLayout({ a: { x: 1, y: 1 } }, ["a"])
    expect(state().revision).toBe(0)
    expect(state().positions).toEqual({ a: { x: 1, y: 1 } })
    expect([...state().expanded]).toEqual(["a"])
  })
})
