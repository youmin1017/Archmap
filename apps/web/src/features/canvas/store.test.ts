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

describe("canvas store focus", () => {
  beforeEach(() => {
    state().loadLayout({ a: { x: 1, y: 1 } }, ["view"])
    state().setFocus(null)
    useCanvasStore.setState({ revision: 0, focusId: null, viewLayout: null, focusLayouts: {} })
  })

  it("keeps the view's layout aside while focused and does not count focus changes", () => {
    state().setFocus("f")
    expect([...state().expanded]).toEqual(["f"])
    expect(state().positions).toEqual({})
    state().toggle("g")
    state().setPosition("g", { x: 5, y: 5 })
    expect(state().revision).toBe(0)
    expect([...state().viewLayoutState().expanded]).toEqual(["view"])
    state().setFocus(null)
    expect([...state().expanded]).toEqual(["view"])
    expect(state().positions).toEqual({ a: { x: 1, y: 1 } })
  })

  it("remembers each focus's own expand state", () => {
    state().setFocus("f")
    state().toggle("g")
    state().setFocus("h")
    expect([...state().expanded]).toEqual(["h"])
    state().setFocus("f")
    expect(new Set(state().expanded)).toEqual(new Set(["f", "g"]))
  })

  it("loads a view's layout aside when already focused", () => {
    state().setFocus("f")
    state().loadLayout({}, ["other"])
    expect([...state().expanded]).toEqual(["f"])
    state().setFocus(null)
    expect([...state().expanded]).toEqual(["other"])
  })
})
