import { describe, expect, it } from "vitest"

import { expandAllIds, EXPAND_ALL_LIMIT } from "./CanvasView"
import { ids, sampleElements, sampleRelations } from "./fixtures"
import {
  aggregateEdges,
  ancestorIds,
  buildTree,
  focusScope,
  focusTree,
  nearestVisible,
  pathTo,
  visibleElements,
  type ElementTree,
} from "./graph"

const tree = buildTree(sampleElements)
const names = (xs: Iterable<string>) => [...xs].map((id) => tree.byId.get(id)?.name).sort()

describe("focusScope", () => {
  it("keeps the subtree and lifts outside neighbours to their highest ancestor outside the focus", () => {
    const s = focusScope(tree, sampleRelations, ids.shop)
    expect(names(s.inside)).toEqual(["API", "Message Queue", "PostgreSQL", "Shop", "Web Frontend", "Worker"])
    // Customer uses the web frontend, the API uses Payment Provider, containers run on Physical Host's nodes.
    expect(names(s.context)).toEqual(["Customer", "Payment Provider", "Physical Host"])
  })

  it("stops lifting at the focus's own ancestors (siblings stay siblings)", () => {
    const s = focusScope(tree, sampleRelations, ids.api)
    expect(names(s.inside)).toEqual(["API"])
    expect(names(s.context)).toEqual([
      "Message Queue",
      "Payment Provider",
      "Physical Host",
      "PostgreSQL",
      "Web Frontend",
    ])
  })

  it("ignores relations to the focus's ancestors and inside the subtree", () => {
    const toParent = [{ ...sampleRelations[0], id: "r", source_id: ids.api, target_id: ids.shop }]
    expect(focusScope(tree, toParent, ids.api).context.size).toBe(0)
    expect(focusScope(tree, sampleRelations.slice(1, 6), ids.shop).context.size).toBe(0)
  })
})

describe("focusTree", () => {
  const scope = focusScope(tree, sampleRelations, ids.shop)
  const ft = focusTree(tree, scope, ids.shop)

  it("roots the focus first, then the context by name; context cards cannot expand", () => {
    expect(ft.roots.map((e) => e.name)).toEqual(["Shop", "Customer", "Payment Provider", "Physical Host"])
    expect(ft.children.has(ids.shop)).toBe(true)
    expect(ft.children.has(ids.host)).toBe(false)
  })

  it("routes relations deep inside a context element to its card", () => {
    const visible = new Set(visibleElements(ft, new Set([ids.shop])).map((e) => e.id))
    const edges = aggregateEdges(ft, sampleRelations, visible)
    expect(edges.find((e) => e.source === ids.web && e.target === ids.host)?.kinds).toEqual({ deployed_on: 1 })
    expect(edges.some((e) => e.source === ids.customer && e.target === ids.web)).toBe(true)
  })
})

describe("aggregateEdges", () => {
  // The memoised nearest-visible lookup must give the same answer as walking each chain.
  function reference(t: ElementTree, visible: ReadonlySet<string>) {
    const out = new Map<string, number>()
    for (const r of sampleRelations) {
      const s = nearestVisible(t, r.source_id, visible)
      const d = nearestVisible(t, r.target_id, visible)
      if (!s || !d || s === d || ancestorIds(t, s).includes(d) || ancestorIds(t, d).includes(s)) continue
      out.set(`${s}->${d}`, (out.get(`${s}->${d}`) ?? 0) + 1)
    }
    return out
  }

  it.each([[[]], [[ids.shop]], [[ids.shop, ids.host]], [[ids.shop, ids.host, ids.docker]]])(
    "matches the per-relation walk (expanded %#)",
    (expanded) => {
      const visible = new Set(visibleElements(tree, new Set(expanded)).map((e) => e.id))
      const got = new Map(aggregateEdges(tree, sampleRelations, visible).map((e) => [e.id, e.count]))
      expect(got).toEqual(reference(tree, visible))
    }
  )
})

describe("pathTo", () => {
  it("lists the path from the top level down to the element", () => {
    expect(pathTo(tree, ids.nginx).map((e) => e.name)).toEqual(["Physical Host", "Docker Host", "Nginx"])
    expect(pathTo(tree, ids.shop).map((e) => e.name)).toEqual(["Shop"])
  })
})

describe("expandAllIds", () => {
  it("expands everything in a small model", () => {
    expect(new Set(expandAllIds(tree, new Set()))).toEqual(new Set([ids.shop, ids.host, ids.docker]))
  })

  it("opens one more level at a time in a large model", () => {
    const many = Array.from({ length: EXPAND_ALL_LIMIT }, (_, i) => ({
      ...sampleElements[3],
      id: `extra-${i}`,
      parent_id: ids.shop,
    }))
    const big = buildTree([...sampleElements, ...many])
    expect(new Set(expandAllIds(big, new Set()))).toEqual(new Set([ids.shop, ids.host]))
    expect(new Set(expandAllIds(big, new Set([ids.shop, ids.host])))).toEqual(new Set([ids.shop, ids.host, ids.docker]))
  })
})
