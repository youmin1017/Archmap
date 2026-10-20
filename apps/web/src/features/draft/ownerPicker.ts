import type { ChangeInput, Directory, Owner, OwnerKind } from "@archmap/api-client"

import { principalOwnerId } from "@/lib/uuidv5"

/** One pickable owner. `value` is the owner id; for users and groups it is derived (ADR-069). */
export type OwnerChoice = {
  value: string
  label: string
  kind: OwnerKind
  /** The user or group this owner stands for. */
  principalId?: string
  /** Extra text to tell same-name entries apart (a user's email). */
  detail?: string
  /** A disabled user or deleted group: kept when already assigned, not offered for new ones. */
  inactive: boolean
}

export type OwnerSection = { label: string; choices: OwnerChoice[] }

const byLabel = (a: OwnerChoice, b: OwnerChoice) => a.label.localeCompare(b.label)

/**
 * The owner picker's sections: Users and Groups from the directory (whether or not
 * the model has an owner for them yet) and the model's named teams and people.
 * Inactive principals are only listed when `keep` (the current values) contains them.
 */
export function ownerSections(
  owners: readonly Owner[],
  directory: Directory | undefined,
  keep: readonly string[] = []
): OwnerSection[] {
  const kept = new Set(keep)
  const users = new Map<string, OwnerChoice>()
  const groups = new Map<string, OwnerChoice>()
  for (const u of directory?.users ?? []) {
    const value = principalOwnerId(u.id)
    users.set(value, {
      value,
      label: u.name,
      kind: "user",
      principalId: u.id,
      detail: u.email,
      inactive: u.disabled,
    })
  }
  for (const g of directory?.groups ?? []) {
    const value = principalOwnerId(g.id)
    groups.set(value, { value, label: g.name, kind: "group", principalId: g.id, inactive: false })
  }
  const named: OwnerChoice[] = []
  for (const o of owners) {
    if (o.kind === "user" || o.kind === "group") {
      const into = o.kind === "user" ? users : groups
      // An owner for a user or group the directory no longer lists (or not loaded yet).
      if (!into.has(o.id)) {
        into.set(o.id, {
          value: o.id,
          label: o.name,
          kind: o.kind,
          principalId: o.principal_id ?? undefined,
          inactive: directory !== undefined,
        })
      }
    } else {
      named.push({ value: o.id, label: o.name, kind: o.kind, inactive: false })
    }
  }
  const visible = (cs: Iterable<OwnerChoice>) => [...cs].filter((c) => !c.inactive || kept.has(c.value)).sort(byLabel)
  return [
    { label: "Users", choices: visible(users.values()) },
    { label: "Groups", choices: visible(groups.values()) },
    { label: "Teams", choices: named.sort(byLabel) },
  ].filter((s) => s.choices.length > 0)
}

/**
 * Owner adds needed before assigning `wanted` owner ids: a user or group picked for
 * the first time has no owner entity in the model yet. The API derives its id from
 * the principal, so the add carries no target id (ADR-069).
 */
export function principalOwnerInputs(
  wanted: Iterable<string>,
  owners: readonly Owner[],
  sections: readonly OwnerSection[]
): ChangeInput[] {
  const have = new Set(owners.map((o) => o.id))
  const choices = new Map(sections.flatMap((s) => s.choices).map((c) => [c.value, c]))
  const out: ChangeInput[] = []
  const added = new Set<string>()
  for (const id of wanted) {
    const c = choices.get(id)
    if (!id || have.has(id) || added.has(id) || !c?.principalId) continue
    added.add(id)
    out.push({
      op: "add",
      target_type: "owner",
      patch: { kind: c.kind, name: "", contact: "", oncall_url: "", principal_id: c.principalId },
    })
  }
  return out
}
