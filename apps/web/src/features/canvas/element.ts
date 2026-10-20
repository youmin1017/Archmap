import { Database, DatabaseZap, ListOrdered, Router, Table2, Zap, type LucideIcon } from "lucide-react"
import type {
  Criticality,
  Element,
  ElementEffective,
  ElementKind,
  ElementType,
  Environment,
  Lifecycle,
  Owner,
  OwnerRole,
} from "@archmap/api-client"

// Vocabulary of the M10 element fields (ADR-062..065). Presentation and UI hints only:
// the API validates values, computes inheritance (`effective`) and runs the lint rules.

export type TypeValue = Exclude<ElementType, "">

export const elementTypes: { value: TypeValue; label: string; icon: LucideIcon; kinds: readonly ElementKind[] }[] = [
  { value: "database-server", label: "Database server", icon: DatabaseZap, kinds: ["container"] },
  { value: "database", label: "Database", icon: Database, kinds: ["container", "component"] },
  { value: "schema", label: "Schema", icon: Table2, kinds: ["component"] },
  { value: "cache", label: "Cache", icon: Zap, kinds: ["container", "component"] },
  { value: "queue", label: "Queue", icon: ListOrdered, kinds: ["container", "component"] },
  { value: "proxy", label: "Proxy", icon: Router, kinds: ["container", "component", "node"] },
]

export const typeMeta = (t: ElementType) => elementTypes.find((x) => x.value === t)

/** Types offered for a kind (mirrors the API's type-kind lint rule). */
export const typesForKind = (kind: ElementKind) => elementTypes.filter((t) => t.kinds.includes(kind))

/** Datastores are drawn as cylinders (the only thing that decides it). */
export const isDatastore = (e: Pick<Element, "type">) =>
  e.type === "database-server" || e.type === "database" || e.type === "schema"

export const environments: { value: Exclude<Environment, "">; label: string }[] = [
  { value: "prod", label: "Prod" },
  { value: "staging", label: "Staging" },
  { value: "dev", label: "Dev" },
]

export const lifecycles: { value: Lifecycle; label: string }[] = [
  { value: "active", label: "Active" },
  { value: "planned", label: "Planned" },
  { value: "deprecated", label: "Deprecated" },
]

export const criticalities: { value: Exclude<Criticality, "">; label: string }[] = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "critical", label: "Critical" },
]

export const ownerRoles: { value: OwnerRole; label: string; hint: string }[] = [
  { value: "owner", label: "Owner", hint: "Accountable for it" },
  { value: "maintainer", label: "Maintainer", hint: "Builds and changes it" },
  { value: "oncall", label: "On-call", hint: "Paged when it breaks" },
  { value: "escalation", label: "Escalation", hint: "When on-call needs help" },
]

export const labelOf = <T extends string>(list: readonly { value: T; label: string }[], v: T | "") =>
  list.find((x) => x.value === v)?.label ?? ""

/** Fields of a new element before the user fills anything in (also the optimistic preview default). */
export const elementDefaults = {
  parent_id: null,
  description: "",
  technology: "",
  tags: [] as string[],
  protected: false,
  layer: "",
  projects: [] as string[],
  replicas: 1,
  properties: {} as Record<string, string>,
  type: "" as ElementType,
  environment: "" as Environment,
  lifecycle: "active" as Lifecycle,
  criticality: "" as Criticality,
  runbook_url: "",
  docs_url: "",
  last_reviewed_at: null as string | null,
  version: 1,
} satisfies Partial<Element>

/** The effective owner of a role and whether it is inherited (null when there is none). */
export function effectiveOwner(
  effective: ElementEffective | undefined,
  elementId: string,
  role: OwnerRole = "owner"
): { ownerId: string; sourceId: string; inherited: boolean } | null {
  const o = effective?.owners.find((x) => x.role === role)
  return o ? { ownerId: o.owner_id, sourceId: o.source_element_id, inherited: o.source_element_id !== elementId } : null
}

/** Owners by id, for showing names. */
export const ownerIndex = (owners: readonly Owner[]) => new Map(owners.map((o) => [o.id, o]))

/** Days since a review date (YYYY-MM-DD), or null when never reviewed. */
export function daysSinceReview(date: string | null, now = new Date()): number | null {
  if (!date) return null
  const t = Date.parse(`${date}T00:00:00Z`)
  return Number.isNaN(t) ? null : Math.floor((now.getTime() - t) / 86_400_000)
}

/** The API's default stale-review threshold (LINT_REVIEW_MAX_AGE); used for a hint only. */
export const REVIEW_MAX_DAYS = 180

/** Today as YYYY-MM-DD in the user's time zone. */
export function today(now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
}
