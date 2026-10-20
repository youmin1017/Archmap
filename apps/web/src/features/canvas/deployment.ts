import type { Element, ElementKind, Relation } from "@archmap/api-client"

/** Kinds that can be deployed on a node (lint: deployed_on goes container / component -> node). */
export const deployableKinds: readonly ElementKind[] = ["container", "component"]

export const canBeDeployed = (kind: ElementKind) => deployableKinds.includes(kind)

/** Ids of the nodes an element is deployed on, in relation order. */
export function hostIds(relations: readonly Relation[], id: string): string[] {
  return relations.filter((r) => r.kind === "deployed_on" && r.source_id === id).map((r) => r.target_id)
}

/** Ids of the elements deployed directly on a node. */
export function hostedIds(relations: readonly Relation[], id: string): string[] {
  return relations.filter((r) => r.kind === "deployed_on" && r.target_id === id).map((r) => r.source_id)
}

/** Element id -> names of the nodes it is deployed on (only elements that have any). */
export function hostNameIndex(
  byId: ReadonlyMap<string, Element>,
  relations: readonly Relation[]
): ReadonlyMap<string, readonly string[]> {
  const out = new Map<string, string[]>()
  for (const r of relations) {
    if (r.kind !== "deployed_on") continue
    const host = byId.get(r.target_id)
    if (!host) continue
    const list = out.get(r.source_id)
    if (list) list.push(host.name)
    else out.set(r.source_id, [host.name])
  }
  return out
}
