import type { Change, Diff, Element, LintReport, Relation } from "@archmap/api-client"

import type { DiffStatus, LintSeverity } from "../canvas/decor"

export type DiffOverlay = {
  elements: Map<string, DiffStatus>
  relations: Map<string, DiffStatus>
  /** Elements / relations that exist in the base but not in the draft: drawn as ghosts. */
  removedElements: Element[]
  removedRelations: Relation[]
}

export function diffOverlay(diff: Diff | undefined): DiffOverlay {
  const out: DiffOverlay = { elements: new Map(), relations: new Map(), removedElements: [], removedRelations: [] }
  if (!diff) return out
  for (const e of diff.elements.added) out.elements.set(e.id, "added")
  for (const m of diff.elements.modified) out.elements.set(m.after.id, "modified")
  for (const e of diff.elements.removed) {
    out.elements.set(e.id, "removed")
    out.removedElements.push(e)
  }
  for (const r of diff.relations.added) out.relations.set(r.id, "added")
  for (const m of diff.relations.modified) out.relations.set(m.after.id, "modified")
  for (const r of diff.relations.removed) {
    out.relations.set(r.id, "removed")
    out.removedRelations.push(r)
  }
  return out
}

/** Worst lint severity per element; relation findings mark both of their endpoints. */
export function lintMarks(report: LintReport | undefined, relations: readonly Relation[]): Map<string, LintSeverity> {
  const marks = new Map<string, LintSeverity>()
  if (!report) return marks
  const mark = (id: string, sev: LintSeverity) => {
    if (marks.get(id) !== "error") marks.set(id, sev)
  }
  const byId = new Map(relations.map((r) => [r.id, r]))
  for (const f of report.findings) {
    if (f.target_type === "element") mark(f.target_id, f.severity)
    else {
      const r = byId.get(f.target_id)
      if (r) {
        mark(r.source_id, f.severity)
        mark(r.target_id, f.severity)
      }
    }
  }
  return marks
}

export type BatchSummary = { batchId: string; undone: boolean; lines: string[]; createdAt: string }

const verb = { add: "Added", update: "Changed", delete: "Deleted" } as const

/** Groups a change log into one entry per GUI edit (batch) with readable lines, newest first. */
export function describeBatches(changes: readonly Change[], nameOf: (id: string) => string): BatchSummary[] {
  const batches = new Map<string, Change[]>()
  for (const c of changes) {
    const list = batches.get(c.batch_id)
    if (list) list.push(c)
    else batches.set(c.batch_id, [c])
  }
  const out: BatchSummary[] = []
  for (const [batchId, list] of batches) {
    list.sort((a, b) => a.seq - b.seq)
    out.push({
      batchId,
      undone: list.every((c) => c.undone_at !== null),
      createdAt: list[0].created_at,
      lines: list.map((c) => describeChange(c, nameOf)),
    })
  }
  return out.reverse()
}

export function describeChange(c: Change, nameOf: (id: string) => string): string {
  const label = (c.patch.name as string | undefined) ?? (c.before?.name as string | undefined) ?? nameOf(c.target_id)
  if (c.target_type === "relation") {
    const src = (c.patch.source_id as string | undefined) ?? (c.before?.source_id as string | undefined)
    const dst = (c.patch.target_id as string | undefined) ?? (c.before?.target_id as string | undefined)
    const ends = src && dst ? `${nameOf(src)} → ${nameOf(dst)}` : "relation"
    return `${verb[c.op]} relation ${ends}`
  }
  if (c.op === "update") {
    const fields = Object.keys(c.patch).join(", ")
    return `Changed ${fields} of ${c.before?.name ?? nameOf(c.target_id)}`
  }
  return `${verb[c.op]} ${label}`
}

/** Compact text for a raw diff / conflict value (field value, entity fields, null = deleted). */
export function formatValue(v: unknown): string {
  if (v === null || v === undefined) return "(none)"
  if (typeof v === "string") return v === "" ? "(empty)" : v
  if (typeof v === "number" || typeof v === "boolean") return String(v)
  if (Array.isArray(v)) return v.length === 0 ? "(none)" : v.join(", ")
  if (typeof v === "object") {
    const o = v as Record<string, unknown>
    if (typeof o.name === "string") return o.name // entity fields: show its name
    return JSON.stringify(v)
  }
  return String(v)
}
