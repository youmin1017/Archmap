import { describe, expect, it } from "vitest"
import type { Element, Relation } from "@archmap/api-client"

import { canBeDeployed, hostedIds, hostIds, hostNameIndex } from "./deployment"

const rel = (id: string, kind: Relation["kind"], source: string, target: string) =>
  ({ id, kind, source_id: source, target_id: target }) as Relation
const el = (id: string, name: string) => ({ id, name }) as Element

const relations = [
  rel("1", "deployed_on", "api", "host"),
  rel("2", "uses", "api", "db"),
  rel("3", "deployed_on", "api", "k8s"),
]

describe("deployment helpers", () => {
  it("knows which kinds can be deployed", () => {
    expect(canBeDeployed("container")).toBe(true)
    expect(canBeDeployed("component")).toBe(true)
    expect(canBeDeployed("node")).toBe(false)
    expect(canBeDeployed("person")).toBe(false)
  })

  it("lists hosts and hosted elements, ignoring other relation kinds", () => {
    expect(hostIds(relations, "api")).toEqual(["host", "k8s"])
    expect(hostedIds(relations, "host")).toEqual(["api"])
    expect(hostedIds(relations, "db")).toEqual([])
  })

  it("indexes host names per element", () => {
    const byId = new Map([el("host", "Docker Host"), el("k8s", "shop-prod")].map((e) => [e.id, e]))
    expect(hostNameIndex(byId, relations).get("api")).toEqual(["Docker Host", "shop-prod"])
    expect(hostNameIndex(byId, relations).has("db")).toBe(false)
  })
})
