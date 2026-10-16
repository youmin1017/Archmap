import { describe, expect, it } from "vitest"
import type { ChangeInput, Model } from "@archmap/api-client"

import { ids, sampleElements, sampleRelations } from "../canvas/fixtures"
import { applyInputs } from "./optimistic"

const model: Model = { elements: sampleElements, relations: sampleRelations }
const NOW = "2030-01-01T00:00:00Z"
const apply = (...inputs: ChangeInput[]) => applyInputs(model, inputs, NOW)

describe("applyInputs", () => {
  it("adds an element with defaults and the given fields", () => {
    const out = apply({
      op: "add",
      target_type: "element",
      target_id: "new-1",
      patch: { kind: "component", name: "Pool", parent_id: ids.api },
    })
    const added = out.elements.find((e) => e.id === "new-1")
    expect(added).toMatchObject({
      kind: "component",
      name: "Pool",
      parent_id: ids.api,
      tags: [],
      protected: false,
      created_at: NOW,
    })
    expect(out.elements).toHaveLength(sampleElements.length + 1)
  })

  it("updates only the patched fields and leaves everything else", () => {
    const out = apply({ op: "update", target_type: "element", target_id: ids.api, patch: { name: "Gateway" } })
    const api = out.elements.find((e) => e.id === ids.api)
    expect(api).toMatchObject({ name: "Gateway", technology: "Go", updated_at: NOW })
    expect(out.elements.find((e) => e.id === ids.worker)?.name).toBe("Worker")
  })

  it("lets an update clear the parent (null)", () => {
    const out = apply({ op: "update", target_type: "element", target_id: ids.api, patch: { parent_id: null } })
    expect(out.elements.find((e) => e.id === ids.api)?.parent_id).toBeNull()
  })

  it("deletes an element together with the relations touching it", () => {
    const out = apply({ op: "delete", target_type: "element", target_id: ids.pg })
    expect(out.elements.some((e) => e.id === ids.pg)).toBe(false)
    expect(out.relations.some((r) => r.source_id === ids.pg || r.target_id === ids.pg)).toBe(false)
    expect(out.relations).toHaveLength(sampleRelations.length - 3) // api->pg, worker->pg, pg deployed_on
  })

  it("cascades to descendants only when asked", () => {
    const plain = apply({ op: "delete", target_type: "element", target_id: ids.host })
    expect(plain.elements.some((e) => e.id === ids.docker)).toBe(true)
    const cascade = apply({ op: "delete", target_type: "element", target_id: ids.host, cascade: true })
    for (const gone of [ids.host, ids.docker, ids.nginx, ids.traefik]) {
      expect(cascade.elements.some((e) => e.id === gone)).toBe(false)
    }
    expect(cascade.relations.some((r) => r.kind === "deployed_on" && r.target_id === ids.nginx)).toBe(false)
  })

  it("adds, updates and deletes relations", () => {
    const added = apply({
      op: "add",
      target_type: "relation",
      target_id: "rel-new",
      patch: { source_id: ids.worker, target_id: ids.api, kind: "uses", technology: "gRPC" },
    })
    expect(added.relations.find((r) => r.id === "rel-new")).toMatchObject({ source_id: ids.worker, technology: "gRPC" })

    const updated = applyInputs(
      added,
      [{ op: "update", target_type: "relation", target_id: "rel-new", patch: { technology: "REST" } }],
      NOW
    )
    expect(updated.relations.find((r) => r.id === "rel-new")?.technology).toBe("REST")

    const deleted = applyInputs(updated, [{ op: "delete", target_type: "relation", target_id: "rel-new" }], NOW)
    expect(deleted.relations.some((r) => r.id === "rel-new")).toBe(false)
  })

  it("applies a batch in order and does not mutate its input", () => {
    const before = JSON.stringify(model)
    const out = apply(
      { op: "add", target_type: "element", target_id: "a", patch: { name: "A", kind: "system" } },
      { op: "update", target_type: "element", target_id: "a", patch: { name: "A2" } }
    )
    expect(out.elements.find((e) => e.id === "a")?.name).toBe("A2")
    expect(JSON.stringify(model)).toBe(before)
  })

  it("skips operations it cannot apply", () => {
    const out = apply(
      { op: "update", target_type: "element", target_id: "missing", patch: { name: "x" } },
      { op: "add", target_type: "element" }
    )
    expect(out.elements).toEqual(sampleElements)
  })
})
