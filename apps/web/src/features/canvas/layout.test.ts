import { describe, expect, it } from "vitest"

import { aggregateEdges, buildTree, visibleElements, type ElementTree } from "./graph"
import { HEADER_HEIGHT, LEAF_SIZE, computeLayout, type Box } from "./layout"
import { ids, sampleElements, sampleRelations } from "./fixtures"

const tree: ElementTree = buildTree(sampleElements)

async function layoutFor(expanded: string[]) {
  const set = new Set(expanded)
  const visible = visibleElements(tree, set)
  const edges = aggregateEdges(tree, sampleRelations, new Set(visible.map((e) => e.id)))
  return { boxes: await computeLayout(tree, set, edges), visible }
}

const overlaps = (a: Box, b: Box) =>
  a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height

describe("computeLayout", () => {
  it("gives every visible element a box; collapsed ones are fixed-size leaves", async () => {
    const { boxes, visible } = await layoutFor([])
    expect(boxes.size).toBe(visible.length)
    for (const b of boxes.values()) expect(b).toMatchObject(LEAF_SIZE)
  })

  it("does not overlap top-level nodes", async () => {
    const { boxes } = await layoutFor([ids.shop, ids.host, ids.docker])
    const top = [ids.customer, ids.payments, ids.shop, ids.host].map((id) => boxes.get(id)!)
    for (let i = 0; i < top.length; i++)
      for (let j = i + 1; j < top.length; j++) expect(overlaps(top[i], top[j])).toBe(false)
  })

  it("sizes an expanded container around its children, with header room", async () => {
    const { boxes } = await layoutFor([ids.shop])
    const shop = boxes.get(ids.shop)!
    expect(shop.width).toBeGreaterThan(LEAF_SIZE.width)
    expect(shop.height).toBeGreaterThan(LEAF_SIZE.height)
    // Children are positioned relative to the parent and stay inside it, below the header.
    for (const child of [ids.web, ids.api, ids.worker, ids.pg, ids.mq]) {
      const c = boxes.get(child)!
      expect(c.y).toBeGreaterThanOrEqual(HEADER_HEIGHT)
      expect(c.x).toBeGreaterThanOrEqual(0)
      expect(c.x + c.width).toBeLessThanOrEqual(shop.width)
      expect(c.y + c.height).toBeLessThanOrEqual(shop.height)
    }
  })

  it("lays out nested containers (host > docker > nginx / traefik)", async () => {
    const { boxes } = await layoutFor([ids.host, ids.docker])
    const host = boxes.get(ids.host)!
    const docker = boxes.get(ids.docker)!
    expect(docker.width).toBeLessThan(host.width)
    expect(boxes.get(ids.nginx)!.x + LEAF_SIZE.width).toBeLessThanOrEqual(docker.width)
    expect(overlaps(boxes.get(ids.nginx)!, boxes.get(ids.traefik)!)).toBe(false)
  })

  it("is deterministic", async () => {
    const a = await layoutFor([ids.shop, ids.host])
    const b = await layoutFor([ids.shop, ids.host])
    expect([...a.boxes]).toEqual([...b.boxes])
  })
})
