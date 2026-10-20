import type { ChangeInput, Element, OwnerRole, Ownership, Relation } from "@archmap/api-client"

import type { ElementFormValues } from "./ElementFields"
import type { RelationFormValues } from "./RelationDialog"

/** Change input that adds an element. Only maintainers' forms carry the protected flag. */
export function addElementInput(id: string, v: ElementFormValues, canProtect: boolean): ChangeInput {
  const patch: Record<string, unknown> = {
    kind: v.kind,
    name: v.name,
    description: v.description,
    technology: v.technology,
    tags: v.tags,
    parent_id: v.parentId,
    layer: v.layer,
    projects: v.projects,
    replicas: v.replicas,
    properties: v.properties,
    type: v.type,
    environment: v.environment,
    lifecycle: v.lifecycle,
    criticality: v.criticality,
    runbook_url: v.runbookUrl,
    docs_url: v.docsUrl,
    last_reviewed_at: v.lastReviewedAt,
  }
  if (canProtect && v.protected) patch.protected = true
  return { op: "add", target_type: "element", target_id: id, patch }
}

const sameTags = (a: readonly string[], b: readonly string[]) => a.length === b.length && a.every((t, i) => t === b[i])

const sameProps = (a: Readonly<Record<string, string>>, b: Readonly<Record<string, string>>) => {
  const ka = Object.keys(a)
  return ka.length === Object.keys(b).length && ka.every((k) => a[k] === b[k])
}

/** Only the fields that actually changed, or null when nothing did. */
export function elementPatch(existing: Element, v: ElementFormValues, canProtect: boolean): ChangeInput | null {
  const patch: Record<string, unknown> = {}
  if (v.kind !== existing.kind) patch.kind = v.kind
  if (v.name !== existing.name) patch.name = v.name
  if (v.description !== existing.description) patch.description = v.description
  if (v.technology !== existing.technology) patch.technology = v.technology
  if (!sameTags(v.tags, existing.tags)) patch.tags = v.tags
  if (v.parentId !== existing.parent_id) patch.parent_id = v.parentId
  if (v.layer !== existing.layer) patch.layer = v.layer
  if (!sameTags(v.projects, existing.projects)) patch.projects = v.projects
  if (v.replicas !== existing.replicas) patch.replicas = v.replicas
  if (!sameProps(v.properties, existing.properties)) patch.properties = v.properties
  if (v.type !== existing.type) patch.type = v.type
  if (v.environment !== existing.environment) patch.environment = v.environment
  if (v.lifecycle !== existing.lifecycle) patch.lifecycle = v.lifecycle
  if (v.criticality !== existing.criticality) patch.criticality = v.criticality
  if (v.runbookUrl !== existing.runbook_url) patch.runbook_url = v.runbookUrl
  if (v.docsUrl !== existing.docs_url) patch.docs_url = v.docsUrl
  if (v.lastReviewedAt !== existing.last_reviewed_at) patch.last_reviewed_at = v.lastReviewedAt
  if (canProtect && v.protected !== existing.protected) patch.protected = v.protected
  if (Object.keys(patch).length === 0) return null
  return { op: "update", target_type: "element", target_id: existing.id, patch }
}

export function addRelationInput(id: string, sourceId: string, targetId: string, v: RelationFormValues): ChangeInput {
  return {
    op: "add",
    target_type: "relation",
    target_id: id,
    patch: {
      source_id: sourceId,
      target_id: targetId,
      kind: v.kind,
      technology: v.technology,
      description: v.description,
    },
  }
}

export function relationPatch(existing: Relation, v: RelationFormValues): ChangeInput | null {
  const patch: Record<string, unknown> = {}
  if (v.kind !== existing.kind) patch.kind = v.kind
  if (v.technology !== existing.technology) patch.technology = v.technology
  if (v.description !== existing.description) patch.description = v.description
  if (Object.keys(patch).length === 0) return null
  return { op: "update", target_type: "relation", target_id: existing.id, patch }
}

/**
 * Relation changes that make an element deployed on exactly `nodeIds`: adds the
 * missing "deployed on" relations and deletes the ones that are no longer wanted.
 */
export function deploymentInputs(
  elementId: string,
  relations: readonly Relation[],
  nodeIds: readonly string[],
  newId: () => string = () => crypto.randomUUID()
): ChangeInput[] {
  const wanted = new Set(nodeIds)
  const existing = relations.filter((r) => r.kind === "deployed_on" && r.source_id === elementId)
  const have = new Set(existing.map((r) => r.target_id))
  const out: ChangeInput[] = []
  for (const r of existing) if (!wanted.has(r.target_id)) out.push(deleteRelationInput(r.id))
  for (const node of wanted) {
    if (!have.has(node)) {
      out.push(addRelationInput(newId(), elementId, node, { kind: "deployed_on", technology: "", description: "" }))
    }
  }
  return out
}

export const deleteElementInput = (id: string, cascade: boolean): ChangeInput => ({
  op: "delete",
  target_type: "element",
  target_id: id,
  cascade,
})

export const deleteRelationInput = (id: string): ChangeInput => ({
  op: "delete",
  target_type: "relation",
  target_id: id,
})

/**
 * Changes that make an element's own owner per role match `wanted` ("" = no own
 * assignment, so the role is inherited). Existing assignments are updated in place;
 * new ones omit the id, which the API derives from element and role (ADR-063).
 */
export function ownershipInputs(
  elementId: string,
  own: readonly Ownership[],
  wanted: Readonly<Record<OwnerRole, string>>
): ChangeInput[] {
  const out: ChangeInput[] = []
  for (const role of Object.keys(wanted) as OwnerRole[]) {
    const cur = own.find((o) => o.role === role)
    const want = wanted[role]
    if (!cur && want) {
      out.push({ op: "add", target_type: "ownership", patch: { element_id: elementId, owner_id: want, role } })
    } else if (cur && !want) {
      out.push({ op: "delete", target_type: "ownership", target_id: cur.id })
    } else if (cur && want && cur.owner_id !== want) {
      out.push({ op: "update", target_type: "ownership", target_id: cur.id, patch: { owner_id: want } })
    }
  }
  return out
}
