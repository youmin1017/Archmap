export type DiffStatus = "added" | "modified" | "removed"
export type LintSeverity = "error" | "warning"

/** Visual overlays computed outside the canvas (diff against base, lint findings). */
export type Decor = {
  /** Element id -> status in the diff overlay. */
  diff?: ReadonlyMap<string, DiffStatus>
  /** Relation id -> status; aggregated edges combine their relations' statuses. */
  relationDiff?: ReadonlyMap<string, DiffStatus>
  /** Element id -> worst lint severity. */
  lint?: ReadonlyMap<string, LintSeverity>
  /** Inside a draft: nodes show connection handles and accept new connections. */
  editable?: boolean
}

/** Combined status of the relations behind one edge. */
export function edgeDiffStatus(
  relationIds: readonly string[],
  relationDiff: ReadonlyMap<string, DiffStatus> | undefined
): DiffStatus | undefined {
  if (!relationDiff || relationDiff.size === 0) return undefined
  const statuses = relationIds.map((id) => relationDiff.get(id))
  if (statuses.every((s) => s === undefined)) return undefined
  if (statuses.every((s) => s === "removed")) return "removed"
  if (statuses.every((s) => s === "added")) return "added"
  return "modified"
}
