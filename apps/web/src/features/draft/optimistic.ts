import type { ChangeInput, Element, Model, Relation } from "@archmap/api-client"

/**
 * UI-level optimistic preview of a batch of changes on a cached draft model.
 * This is NOT validation or business logic (the API is the authority and the
 * result is replaced by a refetch): it only makes the canvas respond instantly.
 * Anything it cannot apply is skipped, and a server rejection rolls it back.
 */
export function applyInputs(model: Model, inputs: readonly ChangeInput[], now = new Date().toISOString()): Model {
  let elements = [...model.elements]
  let relations = [...model.relations]

  for (const input of inputs) {
    const id = input.target_id
    const patch = input.patch ?? {}

    if (input.target_type === "element") {
      if (input.op === "add" && id) {
        elements.push({
          id,
          parent_id: null,
          kind: "container",
          name: "",
          description: "",
          technology: "",
          tags: [],
          protected: false,
          layer: "",
          projects: [],
          replicas: 1,
          properties: {},
          version: 1,
          created_at: now,
          updated_at: now,
          ...(patch as Partial<Element>),
        })
      } else if (input.op === "update" && id) {
        elements = elements.map((e) => (e.id === id ? { ...e, ...(patch as Partial<Element>), updated_at: now } : e))
      } else if (input.op === "delete" && id) {
        const doomed = new Set([id])
        if (input.cascade) {
          // Descendants go with the element (repeat until no new child is found).
          for (let grew = true; grew;) {
            grew = false
            for (const e of elements) {
              if (e.parent_id && doomed.has(e.parent_id) && !doomed.has(e.id)) {
                doomed.add(e.id)
                grew = true
              }
            }
          }
        }
        elements = elements.filter((e) => !doomed.has(e.id))
        relations = relations.filter((r) => !doomed.has(r.source_id) && !doomed.has(r.target_id))
      }
    } else {
      if (input.op === "add" && id) {
        relations.push({
          id,
          source_id: "",
          target_id: "",
          kind: "uses",
          description: "",
          technology: "",
          version: 1,
          created_at: now,
          updated_at: now,
          ...(patch as Partial<Relation>),
        })
      } else if (input.op === "update" && id) {
        relations = relations.map((r) => (r.id === id ? { ...r, ...(patch as Partial<Relation>), updated_at: now } : r))
      } else if (input.op === "delete" && id) {
        relations = relations.filter((r) => r.id !== id)
      }
    }
  }
  return { elements, relations }
}
