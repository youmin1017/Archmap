import type { ChangeInput, Version } from "@archmap/api-client"

const patchName = (i: ChangeInput) => {
  const name = (i.patch as { name?: unknown } | undefined)?.name
  return typeof name === "string" ? name : undefined
}

const more = (n: number) => (n > 1 ? ` and ${n - 1} more` : "")

/**
 * A version summary for a direct edit (ADR-072), e.g. "Add Billing", "Edit API",
 * "Connect Web → API". Element changes win over relations, then ownership.
 */
export function directSummary(inputs: readonly ChangeInput[], nameOf: (id: string) => string): string {
  const of = (op: ChangeInput["op"], t: ChangeInput["target_type"]) =>
    inputs.filter((i) => i.op === op && i.target_type === t)
  const added = of("add", "element")
  if (added.length > 0) return `Add ${patchName(added[0]) ?? "element"}${more(added.length)}`
  const deleted = of("delete", "element")
  if (deleted.length > 0) {
    // A cascade delete lists the descendants first; the requested element is the last one.
    return `Delete ${nameOf(deleted[deleted.length - 1].target_id ?? "")}`
  }
  const edited = [...new Set(of("update", "element").map((i) => i.target_id ?? ""))]
  if (edited.length > 0) return `Edit ${nameOf(edited[0])}${more(edited.length)}`
  const connected = of("add", "relation")
  if (connected.length > 0) {
    const p = connected[0].patch as { source_id?: string; target_id?: string }
    return `Connect ${nameOf(p.source_id ?? "")} → ${nameOf(p.target_id ?? "")}${more(connected.length)}`
  }
  if (of("update", "relation").length > 0) return "Edit relation"
  if (of("delete", "relation").length > 0) return "Delete relation"
  return "Edit owners"
}

/**
 * The direct edit "Undo" reverses: the caller's newest direct edit that is not
 * itself an undo and has not been undone yet. The server refuses it when someone
 * changed the same things since.
 */
export function undoTarget(versions: readonly Version[], meId: string | undefined): Version | undefined {
  if (!meId) return undefined
  const undone = new Set(versions.flatMap((v) => (v.reverts ? [v.reverts] : [])))
  return versions.find((v) => v.kind === "direct" && v.merged_by === meId && !v.reverts && !undone.has(v.changeset_id))
}

/** A run of versions shown together: consecutive direct edits by one person, or a single version. */
export type VersionGroup = { key: string; versions: Version[] }

/** Groups consecutive direct edits by the same person (versions newest first). */
export function groupVersions(versions: readonly Version[]): VersionGroup[] {
  const out: VersionGroup[] = []
  for (const v of versions) {
    const last = out.at(-1)
    const prev = last?.versions.at(-1)
    if (last && prev && v.kind === "direct" && prev.kind === "direct" && v.merged_by === prev.merged_by) {
      last.versions.push(v)
    } else {
      out.push({ key: `v${v.number}`, versions: [v] })
    }
  }
  return out
}
