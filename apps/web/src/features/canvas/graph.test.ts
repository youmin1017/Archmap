import { describe, expect, it } from "vitest"

import {
  aggregateEdges,
  allProjects,
  allTags,
  ancestorIds,
  buildTree,
  descendantCount,
  expandableIds,
  filterMatches,
  isFilterActive,
  nearestVisible,
  visibleElements,
} from "./graph"
import { ids, sampleElements, sampleRelations } from "./fixtures"

const tree = buildTree(sampleElements)
const names = (els: { name: string }[]) => els.map((e) => e.name)

describe("tree", () => {
  it("indexes roots and children sorted by name", () => {
    expect(names([...tree.roots])).toEqual(["Customer", "Payment Provider", "Physical Host", "Shop"])
    expect(names([...(tree.children.get(ids.shop) ?? [])])).toEqual([
      "API",
      "Message Queue",
      "PostgreSQL",
      "Web Frontend",
      "Worker",
    ])
  })

  it("walks ancestors nearest-first and counts descendants", () => {
    expect(ancestorIds(tree, ids.nginx)).toEqual([ids.docker, ids.host])
    expect(ancestorIds(tree, ids.customer)).toEqual([])
    expect(descendantCount(tree, ids.host)).toBe(3)
    expect(descendantCount(tree, ids.api)).toBe(0)
    expect(new Set(expandableIds(tree))).toEqual(new Set([ids.shop, ids.host, ids.docker]))
  })

  it("treats elements with a missing parent as roots and survives cycles", () => {
    const orphan = { ...sampleElements[3], id: "x", parent_id: "does-not-exist" }
    expect(buildTree([orphan]).roots).toHaveLength(1)
    const a = { ...sampleElements[0], id: "a", parent_id: "b" }
    const b = { ...sampleElements[0], id: "b", parent_id: "a" }
    expect(ancestorIds(buildTree([a, b]), "a")).toEqual(["b"])
  })
})

describe("visibleElements", () => {
  it("shows only roots when nothing is expanded", () => {
    expect(names(visibleElements(tree, new Set()))).toEqual(["Customer", "Payment Provider", "Physical Host", "Shop"])
  })

  it("lists parents before their children", () => {
    const order = visibleElements(tree, new Set([ids.host, ids.docker, ids.shop])).map((e) => e.id)
    expect(order.indexOf(ids.host)).toBeLessThan(order.indexOf(ids.docker))
    expect(order.indexOf(ids.docker)).toBeLessThan(order.indexOf(ids.nginx))
    expect(order.indexOf(ids.shop)).toBeLessThan(order.indexOf(ids.api))
    expect(order).toHaveLength(12)
  })

  it("does not show grandchildren of a collapsed element", () => {
    const shown = names(visibleElements(tree, new Set([ids.host]))) // docker not expanded
    expect(shown).toContain("Docker Host")
    expect(shown).not.toContain("Nginx")
  })
})

describe("aggregateEdges", () => {
  const edgesFor = (expanded: string[]) => {
    const visible = new Set(visibleElements(tree, new Set(expanded)).map((e) => e.id))
    return aggregateEdges(tree, sampleRelations, visible)
  }

  it("re-routes relations of collapsed elements to the visible ancestor with a count", () => {
    const edges = edgesFor([]) // landscape: Customer, Payments, Host, Shop
    const byPair = Object.fromEntries(edges.map((e) => [`${e.source}>${e.target}`, e]))
    // customer -> web (inside Shop), api -> payments (inside Shop) -> shop.
    expect(byPair[`${ids.customer}>${ids.shop}`].count).toBe(1)
    expect(byPair[`${ids.shop}>${ids.payments}`].count).toBe(1)
    // 5 deployed_on relations from Shop containers land on the physical host.
    const deploy = byPair[`${ids.shop}>${ids.host}`]
    expect(deploy.count).toBe(5)
    expect(deploy.kinds).toEqual({ deployed_on: 5 })
    // Everything between Shop's own children is internal and not drawn.
    expect(edges.every((e) => e.source !== e.target)).toBe(true)
    expect(edges).toHaveLength(3)
  })

  it("splits edges as nodes expand", () => {
    const edges = edgesFor([ids.shop])
    const pair = (s: string, t: string) => edges.find((e) => e.source === s && e.target === t)
    expect(pair(ids.web, ids.api)?.count).toBe(1)
    expect(pair(ids.api, ids.pg)?.count).toBe(1)
    expect(pair(ids.customer, ids.web)?.count).toBe(1)
    expect(pair(ids.api, ids.payments)?.count).toBe(1)
    // deployed_on edges now start at the individual containers, still ending at the host.
    expect(pair(ids.web, ids.host)?.count).toBe(1)
    expect(pair(ids.worker, ids.host)?.count).toBe(1)
    expect(pair(ids.shop, ids.host)).toBeUndefined()
  })

  it("keeps parallel relations of different kinds in one counted edge", () => {
    const extra = [...sampleRelations, { ...sampleRelations[0], id: "extra", kind: "deployed_on" as const }]
    const visible = new Set(visibleElements(tree, new Set()).map((e) => e.id))
    const edge = aggregateEdges(tree, extra, visible).find((e) => e.source === ids.customer && e.target === ids.shop)
    expect(edge?.count).toBe(2)
    expect(edge?.kinds).toEqual({ uses: 1, deployed_on: 1 })
  })

  it("skips relations between a node and its own descendant, and dangling endpoints", () => {
    const rels = [
      { ...sampleRelations[0], id: "r1", source_id: ids.shop, target_id: ids.api },
      { ...sampleRelations[0], id: "r2", source_id: ids.shop, target_id: "missing" },
    ]
    const visible = new Set(visibleElements(tree, new Set([ids.shop])).map((e) => e.id))
    expect(aggregateEdges(tree, rels, visible)).toEqual([])
  })

  it("resolves nearest visible ancestors", () => {
    const visible = new Set([ids.host, ids.docker])
    expect(nearestVisible(tree, ids.nginx, visible)).toBe(ids.docker)
    expect(nearestVisible(tree, ids.docker, visible)).toBe(ids.docker)
    expect(nearestVisible(tree, ids.api, visible)).toBeNull()
  })
})

describe("filters", () => {
  it("matches by text, kind and tag, and keeps ancestors of matches", () => {
    const { matches, keep } = filterMatches(tree, { query: "nginx", kinds: [], tags: [], project: null })
    expect([...matches]).toEqual([ids.nginx])
    expect(keep).toEqual(new Set([ids.nginx, ids.docker, ids.host]))

    expect(filterMatches(tree, { query: "", kinds: ["person"], tags: [], project: null }).matches).toEqual(
      new Set([ids.customer])
    )
    expect(filterMatches(tree, { query: "", kinds: [], tags: ["backend"], project: null }).matches).toEqual(
      new Set([ids.api, ids.worker])
    )
    expect(
      filterMatches(tree, { query: "go", kinds: ["container"], tags: ["backend"], project: null }).matches
    ).toEqual(new Set([ids.api, ids.worker]))
    expect(filterMatches(tree, { query: "zzz", kinds: [], tags: [], project: null }).matches.size).toBe(0)
  })

  it("highlights a project (case-insensitive) and lists projects", () => {
    const els = sampleElements.map((e) => ({
      ...e,
      projects: e.id === ids.api ? ["Shop", "Payments"] : e.id === ids.worker ? ["payments"] : [],
    }))
    const t = buildTree(els)
    const f = { query: "", kinds: [], tags: [], project: "PAYMENTS" }
    expect(filterMatches(t, f).matches).toEqual(new Set([ids.api, ids.worker]))
    expect(isFilterActive(f)).toBe(true)
    expect(allProjects(els)).toEqual(["Payments", "Shop"])
  })

  it("knows when a filter is active and lists tags", () => {
    expect(isFilterActive({ query: "  ", kinds: [], tags: [], project: null })).toBe(false)
    expect(isFilterActive({ query: "a", kinds: [], tags: [], project: null })).toBe(true)
    expect(allTags(sampleElements)).toEqual(["backend", "database", "external", "frontend"])
  })
})
