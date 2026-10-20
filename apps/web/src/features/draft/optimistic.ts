import type { ChangeInput, Element, Model, Owner, Ownership, Relation } from "@archmap/api-client"

import { elementDefaults } from "../canvas/element"

/**
 * UI-level optimistic preview of a batch of changes on a cached draft model.
 * This is NOT validation or business logic (the API is the authority and the
 * result is replaced by a refetch): it only makes the canvas respond instantly.
 * Anything it cannot apply is skipped, and a server rejection rolls it back.
 */
export function applyInputs(model: Model, inputs: readonly ChangeInput[], now = new Date().toISOString()): Model {
  let elements = [...model.elements]
  let relations = [...model.relations]
  let owners = [...model.owners]
  let ownerships = [...model.ownerships]

  for (const input of inputs) {
    const id = input.target_id
    const patch = input.patch ?? {}

    if (input.target_type === "element") {
      if (input.op === "add" && id) {
        elements.push({
          ...elementDefaults,
          id,
          kind: "container",
          name: "",
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
  // Owners and ownerships: previewed as-is; inherited values stay as the server computed
  // them until the refetch.
  for (const input of inputs) {
    const id = input.target_id
    const patch = input.patch ?? {}
    if (input.target_type === "owner" && id) {
      if (input.op === "add") {
        owners.push({
          id,
          kind: "team",
          name: "",
          contact: "",
          oncall_url: "",
          version: 1,
          ...(patch as Partial<Owner>),
        })
      } else if (input.op === "update") {
        owners = owners.map((o) => (o.id === id ? { ...o, ...(patch as Partial<Owner>) } : o))
      } else {
        owners = owners.filter((o) => o.id !== id)
        ownerships = ownerships.filter((o) => o.owner_id !== id)
      }
    } else if (input.target_type === "ownership") {
      const p = patch as Partial<Ownership>
      if (input.op === "add" && p.element_id && p.owner_id && p.role) {
        // The server derives the real id from element and role; this preview key is replaced on refetch.
        ownerships.push({
          id: id ?? `${p.element_id}:${p.role}`,
          element_id: p.element_id,
          owner_id: p.owner_id,
          role: p.role,
          version: 1,
        })
      } else if (input.op === "update" && id) {
        ownerships = ownerships.map((o) => (o.id === id ? { ...o, ...p } : o))
      } else if (input.op === "delete" && id) {
        ownerships = ownerships.filter((o) => o.id !== id)
      }
    }
  }
  return { elements, relations, owners, ownerships }
}
