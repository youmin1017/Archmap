import { describe, expect, it } from "vitest"

import { ids, sampleElements, sampleRelations } from "../canvas/fixtures"
import type { Relation } from "@archmap/api-client"
import type { ElementFormValues } from "./ElementFields"
import {
  addElementInput,
  addRelationInput,
  deleteElementInput,
  deploymentInputs,
  elementPatch,
  ownershipInputs,
  relationPatch,
} from "./patches"

const api = sampleElements.find((e) => e.id === ids.api)!
const values = (over: Partial<ElementFormValues> = {}): ElementFormValues => ({
  kind: api.kind,
  name: api.name,
  description: api.description,
  technology: api.technology,
  tags: api.tags,
  parentId: api.parent_id,
  protected: api.protected,
  layer: api.layer,
  projects: api.projects,
  replicas: api.replicas,
  properties: api.properties,
  deployedOn: [],
  type: api.type,
  environment: api.environment,
  lifecycle: api.lifecycle,
  criticality: api.criticality,
  runbookUrl: api.runbook_url,
  docsUrl: api.docs_url,
  lastReviewedAt: api.last_reviewed_at,
  ownership: { owner: "", maintainer: "", oncall: "", escalation: "" },
  ...over,
})

describe("elementPatch", () => {
  it("records replicas and properties only when they changed", () => {
    expect(elementPatch(api, values({ replicas: 3 }), false)?.patch).toEqual({ replicas: 3 })
    expect(elementPatch(api, values({ properties: { ip: "10.0.0.5" } }), false)?.patch).toEqual({
      properties: { ip: "10.0.0.5" },
    })
  })

  it("records a changed project list", () => {
    expect(elementPatch(api, values({ projects: ["Shop"] }), false)?.patch).toEqual({ projects: ["Shop"] })
  })

  it("is null when nothing changed", () => {
    expect(elementPatch(api, values(), true)).toBeNull()
  })

  it("contains only changed fields", () => {
    const input = elementPatch(api, values({ name: "Gateway", tags: ["backend", "edge"] }), true)
    expect(input).toEqual({
      op: "update",
      target_type: "element",
      target_id: ids.api,
      patch: { name: "Gateway", tags: ["backend", "edge"] },
    })
  })

  it("can clear the parent and never sends the protected flag for non-maintainers", () => {
    expect(elementPatch(api, values({ parentId: null }), false)?.patch).toEqual({ parent_id: null })
    expect(elementPatch(api, values({ protected: !api.protected }), false)).toBeNull()
    expect(elementPatch(api, values({ protected: !api.protected }), true)?.patch).toEqual({ protected: !api.protected })
  })
})

describe("layer", () => {
  it("is part of an edit only when it changed, and can be cleared", () => {
    expect(elementPatch(api, values({ layer: "Kubernetes" }), false)?.patch).toEqual({ layer: "Kubernetes" })
    expect(elementPatch({ ...api, layer: "Services" }, values({ layer: "" }), false)?.patch).toEqual({ layer: "" })
    expect(elementPatch(api, values(), false)).toBeNull()
  })

  it("is sent when adding an element", () => {
    expect(addElementInput("x", values({ layer: "Physical" }), false).patch).toHaveProperty("layer", "Physical")
  })
})

describe("addElementInput", () => {
  it("carries the form fields and the client-generated id", () => {
    const input = addElementInput("new-id", values({ name: "Cache", parentId: ids.shop, protected: true }), false)
    expect(input).toMatchObject({ op: "add", target_type: "element", target_id: "new-id" })
    expect(input.patch).toMatchObject({ name: "Cache", parent_id: ids.shop, kind: "container" })
    expect(input.patch).not.toHaveProperty("protected")
  })

  it("sets protected only for maintainers", () => {
    expect(addElementInput("x", values({ protected: true }), true).patch).toHaveProperty("protected", true)
  })
})

describe("relations and deletes", () => {
  it("builds add / patch / delete inputs", () => {
    const add = addRelationInput("r-new", ids.worker, ids.api, { kind: "uses", technology: "gRPC", description: "" })
    expect(add.patch).toEqual({
      source_id: ids.worker,
      target_id: ids.api,
      kind: "uses",
      technology: "gRPC",
      description: "",
    })

    const rel = sampleRelations[0]
    expect(relationPatch(rel, { kind: rel.kind, technology: rel.technology, description: rel.description })).toBeNull()
    expect(relationPatch(rel, { kind: rel.kind, technology: "QUIC", description: rel.description })?.patch).toEqual({
      technology: "QUIC",
    })

    expect(deleteElementInput(ids.host, true)).toEqual({
      op: "delete",
      target_type: "element",
      target_id: ids.host,
      cascade: true,
    })
  })
})

describe("deploymentInputs", () => {
  const dep = (id: string, source: string, target: string) =>
    ({ id, kind: "deployed_on", source_id: source, target_id: target }) as Relation
  const uses = { id: "u", kind: "uses", source_id: "a", target_id: "n1" } as Relation
  let n = 0
  const newId = () => `new-${++n}`

  it("adds the missing relations", () => {
    const out = deploymentInputs("a", [], ["n1", "n2"], newId)
    expect(out.map((i) => [i.op, i.patch?.target_id])).toEqual([
      ["add", "n1"],
      ["add", "n2"],
    ])
    expect(out[0]?.patch).toMatchObject({ source_id: "a", kind: "deployed_on" })
  })

  it("removes relations to nodes that were dropped and keeps the others", () => {
    const out = deploymentInputs("a", [dep("r1", "a", "n1"), dep("r2", "a", "n2")], ["n2"], newId)
    expect(out).toEqual([{ op: "delete", target_type: "relation", target_id: "r1" }])
  })

  it("does nothing when the set is unchanged and ignores other relation kinds and elements", () => {
    const rels = [dep("r1", "a", "n1"), uses, dep("r3", "b", "n9")]
    expect(deploymentInputs("a", rels, ["n1", "n1"], newId)).toEqual([])
  })

  it("clears every deployment with an empty list", () => {
    expect(deploymentInputs("a", [dep("r1", "a", "n1")], [], newId)).toHaveLength(1)
  })
})

describe("operational fields and ownership", () => {
  it("records only the operational fields that changed", () => {
    expect(
      elementPatch(
        api,
        values({ environment: "prod", lastReviewedAt: "2026-10-01", runbookUrl: "https://r.example" }),
        false
      )?.patch
    ).toEqual({ environment: "prod", last_reviewed_at: "2026-10-01", runbook_url: "https://r.example" })
    expect(elementPatch(api, values({ lastReviewedAt: api.last_reviewed_at }), false)).toBeNull()
  })

  it("adds new elements with every operational field", () => {
    const add = addElementInput("x", values({ type: "database", lifecycle: "planned" }), false)
    expect(add.patch).toMatchObject({ type: "database", lifecycle: "planned", environment: "", last_reviewed_at: null })
  })

  it("turns the wanted owners per role into add, update and delete changes", () => {
    const own = [
      { id: "o1", element_id: ids.api, owner_id: "team-a", role: "owner" as const, version: 1 },
      { id: "o2", element_id: ids.api, owner_id: "team-b", role: "oncall" as const, version: 1 },
    ]
    const out = ownershipInputs(ids.api, own, { owner: "team-c", maintainer: "team-a", oncall: "", escalation: "" })
    expect(out).toEqual([
      { op: "update", target_type: "ownership", target_id: "o1", patch: { owner_id: "team-c" } },
      { op: "add", target_type: "ownership", patch: { element_id: ids.api, owner_id: "team-a", role: "maintainer" } },
      { op: "delete", target_type: "ownership", target_id: "o2" },
    ])
    expect(
      ownershipInputs(ids.api, own, { owner: "team-a", maintainer: "", oncall: "team-b", escalation: "" })
    ).toEqual([])
  })
})
