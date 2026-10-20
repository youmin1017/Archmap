import { describe, expect, it, vi } from "vitest"

import type { MenuEntry } from "../canvas/CanvasContextMenu"
import { joinGroups } from "../canvas/CanvasContextMenu"
import { ids, sampleElements, sampleRelations } from "../canvas/fixtures"
import {
  childKindsOf,
  edgeEntries,
  layerEntries,
  nodeEntries,
  paneEntries,
  rootKinds,
  stackEntries,
  type DraftMenuDeps,
} from "./menuEntries"

function makeDeps(ghosts: string[] = []): DraftMenuDeps {
  return {
    element: (id) => sampleElements.find((e) => e.id === id),
    relation: (id) => sampleRelations.find((r) => r.id === id),
    nameOf: (id) => sampleElements.find((e) => e.id === id)?.name ?? "?",
    isGhost: (id) => ghosts.includes(id),
    addElement: vi.fn(),
    setLayer: vi.fn(),
    editElement: vi.fn(),
    deleteElement: vi.fn(),
    startConnect: vi.fn(),
    editRelation: vi.fn(),
    deleteRelation: vi.fn(),
  }
}

const labels = (entries: MenuEntry[]) => entries.map((e) => (e.type === "separator" ? "---" : e.label))
/** Triggers a plain item (not a separator or submenu). */
function run(entry: MenuEntry | undefined) {
  if (entry && entry.type !== "separator" && entry.type !== "submenu") entry.onSelect()
}
const find = (entries: MenuEntry[], label: string) => entries.find((e) => e.type !== "separator" && e.label === label)

describe("child / root kinds", () => {
  it("follow the hierarchy hints", () => {
    expect(childKindsOf("system")).toEqual(["container"])
    expect(childKindsOf("container")).toEqual(["component"])
    expect(childKindsOf("node")).toEqual(["node"])
    expect(childKindsOf("person")).toEqual([])
    expect(rootKinds()).toEqual(["person", "system", "node"])
  })
})

describe("nodeEntries", () => {
  it("offers edit, add child, connect and delete", () => {
    const entries = nodeEntries(makeDeps(), ids.shop)
    expect(labels(entries)).toEqual(["Edit…", "Add child", "Connect to…", "---", "Delete…"])
    const addChild = find(entries, "Add child")
    expect(addChild?.type === "submenu" && labels(addChild.entries)).toEqual(["Container"])
    expect(find(entries, "Delete…")).toMatchObject({ destructive: true })
  })

  it("disables Add child for kinds that cannot have children", () => {
    const entries = nodeEntries(makeDeps(), ids.customer)
    expect(find(entries, "Add child")).toMatchObject({ disabled: true })
  })

  it("wires entries to the actions with the right target", () => {
    const deps = makeDeps()
    const entries = nodeEntries(deps, ids.shop)
    const addChild = find(entries, "Add child")
    if (addChild?.type === "submenu") run(addChild.entries[0])
    expect(deps.addElement).toHaveBeenCalledWith("container", ids.shop)
    run(find(entries, "Connect to…"))
    expect(deps.startConnect).toHaveBeenCalledWith(ids.shop)
    run(find(entries, "Delete…"))
    expect(deps.deleteElement).toHaveBeenCalledWith(ids.shop)
  })

  it("has no editing entries for removed (ghost) or unknown elements", () => {
    expect(nodeEntries(makeDeps([ids.shop]), ids.shop)).toEqual([])
    expect(nodeEntries(makeDeps(), "missing")).toEqual([])
  })
})

describe("edgeEntries", () => {
  const rel = (n: string) => sampleRelations.find((r) => r.id.endsWith(n.padStart(12, "0")))!

  it("single relation: edit and delete directly", () => {
    const deps = makeDeps()
    const entries = edgeEntries(deps, [rel("102").id])
    expect(labels(entries)).toEqual(["Edit relation…", "Delete relation"])
    run(entries[1])
    expect(deps.deleteRelation).toHaveBeenCalledWith(rel("102"))
  })

  it("aggregated edge: one submenu per relation", () => {
    const entries = edgeEntries(makeDeps(), [rel("103").id, rel("104").id])
    expect(labels(entries)).toEqual(["API → PostgreSQL · SQL", "API → Message Queue · AMQP"])
    expect(entries.every((e) => e.type === "submenu")).toBe(true)
  })

  it("is empty when the relations touch removed elements or do not exist", () => {
    expect(edgeEntries(makeDeps([ids.pg]), [rel("103").id])).toEqual([])
    expect(edgeEntries(makeDeps(), ["missing"])).toEqual([])
  })
})

describe("paneEntries and joinGroups", () => {
  it("adds only top-level kinds from the pane", () => {
    const deps = makeDeps()
    const add = paneEntries(deps)[0]
    expect(add.type === "submenu" && labels(add.entries)).toEqual(["Person", "System", "Node"])
    if (add.type === "submenu") run(add.entries[1])
    expect(deps.addElement).toHaveBeenCalledWith("system", null)
  })

  it("separates non-empty groups only", () => {
    const a: MenuEntry = { label: "a", onSelect: () => {} }
    const b: MenuEntry = { label: "b", onSelect: () => {} }
    expect(labels(joinGroups([a], [], [b]))).toEqual(["a", "---", "b"])
    expect(joinGroups([], [])).toEqual([])
  })
})

describe("layers", () => {
  const layers = ["User", "Services", "Physical"]

  it("adds Move to layer only when a layers view is active", () => {
    expect(labels(nodeEntries(makeDeps(), ids.api))).not.toContain("Move to layer")
    const entries = nodeEntries({ ...makeDeps(), layers }, ids.api)
    expect(labels(entries)).toEqual(["Edit…", "Add child", "Move to layer", "Connect to…", "---", "Delete…"])
  })

  it("lists every layer plus Unassigned, disables the current one and sets the chosen one", () => {
    const deps: DraftMenuDeps = { ...makeDeps(), layers }
    const api = sampleElements.find((e) => e.id === ids.api)!
    deps.element = (id) => (id === ids.api ? { ...api, layer: "services" } : sampleElements.find((e) => e.id === id)) // case-insensitive match
    const move = find(nodeEntries(deps, ids.api), "Move to layer")
    if (move?.type !== "submenu") throw new Error("expected a submenu")
    expect(labels(move.entries)).toEqual(["User", "Services", "Physical", "Unassigned"])
    expect(move.entries.map((e) => e.type !== "separator" && e.type !== "submenu" && e.disabled)).toEqual([
      false,
      true,
      false,
      false,
    ])
    run(move.entries[2])
    expect(deps.setLayer).toHaveBeenCalledWith(ids.api, "Physical")
    run(move.entries[3])
    expect(deps.setLayer).toHaveBeenCalledWith(ids.api, "")
  })

  it("adds an element straight into a layer plane", () => {
    const deps = makeDeps()
    const add = layerEntries(deps, "Services", false)[0]
    if (add.type !== "submenu") throw new Error("expected a submenu")
    expect(add.label).toBe("Add element to Services")
    expect(labels(add.entries)).toEqual(["Person", "System", "Container", "Component", "Node"])
    run(add.entries[2])
    expect(deps.addElement).toHaveBeenCalledWith("container", null, "Services")

    const loose = layerEntries(deps, "Unassigned", true)[0]
    if (loose.type !== "submenu") throw new Error("expected a submenu")
    expect(loose.label).toBe("Add element")
    run(loose.entries[0])
    expect(deps.addElement).toHaveBeenLastCalledWith("person", null, "")
  })

  it("offers layer management only when the user may edit the view", () => {
    const deps = makeDeps()
    expect(labels(layerEntries(deps, "Services", false))).toEqual(["Add element to Services"])
    expect(stackEntries(deps)).toEqual([])

    const ops = { add: vi.fn(), rename: vi.fn(), move: vi.fn(), remove: vi.fn() }
    const managed = { ...deps, layers: ["User", "Services", "Physical"], layerOps: ops }
    const entries = layerEntries(managed, "Services", false)
    expect(labels(entries)).toEqual([
      "Add element to Services",
      "---",
      "New layer…",
      "Rename layer…",
      "Move up",
      "Move down",
      "---",
      "Delete layer…",
    ])
    run(entries.find((e) => e.type !== "separator" && e.label === "Rename layer…"))
    expect(ops.rename).toHaveBeenCalledWith("Services")
    run(entries.find((e) => e.type !== "separator" && e.label === "Move down"))
    expect(ops.move).toHaveBeenCalledWith("Services", 1)
    run(entries.find((e) => e.type !== "separator" && e.label === "Delete layer…"))
    expect(ops.remove).toHaveBeenCalledWith("Services")
  })

  it("disables moves at the ends and skips rename / delete on Unassigned", () => {
    const ops = { add: vi.fn(), rename: vi.fn(), move: vi.fn(), remove: vi.fn() }
    const deps = { ...makeDeps(), layers: ["User", "Services"], layerOps: ops }
    const top = layerEntries(deps, "User", false).find((e) => e.type !== "separator" && e.label === "Move up")
    expect(top && top.type !== "separator" && top.type !== "submenu" ? top.disabled : undefined).toBe(true)
    expect(labels(layerEntries(deps, "Unassigned", true))).toEqual(["Add element", "---", "New layer…"])
    expect(labels(stackEntries(deps))).toEqual(["New layer…"])
  })
})
