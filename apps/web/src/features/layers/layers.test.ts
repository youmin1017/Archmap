import { describe, expect, it } from "vitest"
import type { Element } from "@archmap/api-client"

import { sampleElements } from "../canvas/fixtures"
import {
  MAX_LAYERS,
  MAX_LAYER_NAME,
  UNASSIGNED,
  kindAllowedOnLayer,
  layerAllowed,
  layerViolation,
  nestWithin,
  violationMessage,
  groupByLayer,
  laneModes,
  layerNameError,
  onLayer,
  planeHue,
  removeLayerFrom,
  renameLayerIn,
  shiftLayer,
  toggleHidden,
} from "./layers"

const withLayer = (names: Record<string, string>): Element[] =>
  sampleElements.map((e) => ({ ...e, layer: names[e.name] ?? "" }))

describe("groupByLayer", () => {
  const elements = withLayer({
    Customer: "User",
    API: "services",
    Worker: "Services ",
    "Physical Host": "Physical",
    Shop: "Mystery",
  })

  it("keeps the view's layer order and matches names case-insensitively", () => {
    const planes = groupByLayer(elements, ["User", "Services", "Kubernetes", "Physical"])
    expect(planes.map((p) => p.name)).toEqual(["User", "Services", "Kubernetes", "Physical", UNASSIGNED])
    expect(planes[0].elements.map((e) => e.name)).toEqual(["Customer"])
    expect(planes[1].elements.map((e) => e.name)).toEqual(["API", "Worker"]) // both spellings land together
    expect(planes[2].elements).toEqual([]) // empty layers still get a layer
  })

  it("collects empty and unknown layers on a trailing Unassigned layer", () => {
    const planes = groupByLayer(elements, ["User", "Services"])
    const last = planes.at(-1)!
    expect(last).toMatchObject({ name: UNASSIGNED, unassigned: true })
    expect(last.elements.map((e) => e.name)).toContain("Shop") // unknown layer
    expect(last.elements.map((e) => e.name)).toContain("Payment Provider") // no layer at all
    expect(planes.filter((p) => p.unassigned)).toHaveLength(1)
  })

  it("has no Unassigned layer when every element is placed", () => {
    const all = sampleElements.map((e) => ({ ...e, layer: "L" }))
    expect(groupByLayer(all, ["L"]).some((p) => p.unassigned)).toBe(false)
  })

  it("sorts a layer by kind, then name", () => {
    const planes = groupByLayer(
      sampleElements.map((e) => ({ ...e, layer: "All" })),
      ["All"]
    )
    const order = ["person", "system", "container", "component", "node"]
    const kinds = planes[0].elements.map((e) => order.indexOf(e.kind))
    expect(kinds).toEqual([...kinds].sort((a, b) => a - b))
  })
})

describe("laneModes", () => {
  const planes = groupByLayer(withLayer({ Customer: "User", API: "Services", "Physical Host": "Physical" }), [
    "User",
    "Services",
    "Physical",
  ])
  const modes = (hidden: string[], focus: string | null) =>
    Object.fromEntries(laneModes(planes, new Set(hidden), focus))

  it("shows every layer by default", () => {
    expect(modes([], null)).toMatchObject({ User: "open", Services: "open", Physical: "open" })
  })

  it("hides switched-off layers", () => {
    expect(modes(["Services"], null)).toMatchObject({ User: "open", Services: "hidden", Physical: "open" })
  })

  it("collapses the other visible layers around the focused one", () => {
    expect(modes([], "Services")).toMatchObject({ User: "collapsed", Services: "open", Physical: "collapsed" })
    // hidden stays hidden rather than collapsing
    expect(modes(["Physical"], "Services")).toMatchObject({ User: "collapsed", Services: "open", Physical: "hidden" })
  })

  it("ignores a focus on a hidden or unknown layer", () => {
    expect(modes(["Services"], "Services")).toMatchObject({ User: "open", Services: "hidden", Physical: "open" })
    expect(modes([], "Nope")).toMatchObject({ User: "open", Services: "open", Physical: "open" })
  })
})

describe("toggleHidden and planeHue", () => {
  it("toggles without mutating", () => {
    const start = new Set(["A"])
    const next = toggleHidden(start, "B")
    expect([...next].sort()).toEqual(["A", "B"])
    expect([...start]).toEqual(["A"])
    expect([...toggleHidden(next, "A")]).toEqual(["B"])
  })

  it("gives neighbouring layers different tints", () => {
    expect(planeHue(0)).not.toBe(planeHue(1))
    expect(planeHue(0)).toBeGreaterThanOrEqual(0)
    expect(planeHue(50)).toBeLessThan(360)
  })
})

describe("layer list editing", () => {
  const layers = ["User", "Services", "Physical"]

  it("validates names", () => {
    expect(layerNameError(layers, "  ")).toMatch(/name/)
    expect(layerNameError(layers, "services")).toMatch(/exists/)
    expect(layerNameError(layers, "Unassigned")).toMatch(/reserved/)
    expect(layerNameError(layers, "x".repeat(MAX_LAYER_NAME + 1))).toMatch(/characters/)
    expect(layerNameError(layers, "Data")).toBeNull()
    expect(
      layerNameError(
        Array.from({ length: MAX_LAYERS }, (_, i) => `L${i}`),
        "Data"
      )
    ).toMatch(/At most/)
  })

  it("lets a layer keep its name or change its case when renaming", () => {
    expect(layerNameError(layers, "SERVICES", "Services")).toBeNull()
    expect(layerNameError(layers, "user", "Services")).toMatch(/exists/)
  })

  it("renames, removes and shifts without mutating", () => {
    expect(renameLayerIn(layers, "services", " Backend ")).toEqual(["User", "Backend", "Physical"])
    expect(removeLayerFrom(layers, "SERVICES")).toEqual(["User", "Physical"])
    expect(shiftLayer(layers, "Services", -1)).toEqual(["Services", "User", "Physical"])
    expect(shiftLayer(layers, "Physical", 1)).toEqual(layers)
    expect(shiftLayer(layers, "User", -1)).toEqual(layers)
    expect(layers).toEqual(["User", "Services", "Physical"])
  })

  it("finds the elements of a layer", () => {
    const els = withLayer({ API: "services", Worker: "Services " })
    expect(
      onLayer(els, "Services")
        .map((e) => e.name)
        .sort()
    ).toEqual(["API", "Worker"])
  })
})

describe("layer rules", () => {
  const rules = [
    { element_class: "database", layers: ["Services", "Data"] },
    { element_class: "node", layers: ["Physical"] },
  ]
  const kind = (k: Element["kind"]) => ({ kind: k })

  it("restricts the kinds named by a rule", () => {
    expect(layerAllowed(rules, kind("database"), "data")).toBe(true)
    expect(layerAllowed(rules, kind("database"), "Physical")).toBe(false)
    expect(layerAllowed(rules, kind("container"), "Physical")).toBe(true)
    expect(layerAllowed(rules, kind("node"), "Services")).toBe(false)
    expect(layerAllowed(rules, kind("database"), "")).toBe(true)
    expect(layerAllowed([], kind("database"), "Physical")).toBe(true)
  })

  it("explains the violated rule", () => {
    const r = layerViolation(rules, kind("database"), "Physical")
    expect(r && violationMessage(r)).toBe("Databases can only be placed on: Services, Data.")
  })

  it("checks kind rules for menus", () => {
    expect(kindAllowedOnLayer(rules, "node", "Services")).toBe(false)
    expect(kindAllowedOnLayer(rules, "node", "physical")).toBe(true)
    expect(kindAllowedOnLayer(rules, "container", "Services")).toBe(true)
  })
})

describe("nestWithin", () => {
  const el = (id: string, parent: string | null): Element => ({ ...sampleElements[0], id, name: id, parent_id: parent })

  it("nests children under a parent on the same layer and keeps order", () => {
    const tree = nestWithin([
      el("ns", "cluster"),
      el("cluster", null),
      el("svc-a", "ns"),
      el("svc-b", "ns"),
      el("solo", "elsewhere"),
    ])
    expect(tree.map((n) => n.element.id)).toEqual(["cluster", "solo"])
    expect(tree[0].children.map((n) => n.element.id)).toEqual(["ns"])
    expect(tree[0].children[0].children.map((n) => n.element.id)).toEqual(["svc-a", "svc-b"])
  })

  it("treats an element whose parent is on another layer as a top-level one", () => {
    expect(nestWithin([el("a", "gone")]).map((n) => n.element.id)).toEqual(["a"])
  })
})
