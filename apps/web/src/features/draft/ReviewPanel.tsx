import { useMemo, useState } from "react"
import { CircleAlert, GitMerge, Info, Minus, Pencil, Plus, TriangleAlert } from "lucide-react"
import type { Change, Conflict, Diff, LintReport, Relation, Resolution } from "@archmap/api-client"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { AnimatedList, AnimatedListItem } from "@/components/ui/animated-list"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"
import { ancestorIds, type ElementTree } from "../canvas/graph"
import { useCanvasStore } from "../canvas/store"
import { useOwnerIndex } from "../canvas/owners"
import { describeBatches, formatValue } from "./decor"

type Props = {
  tree: ElementTree
  relations: readonly Relation[]
  lint?: LintReport
  diff?: Diff
  conflicts?: Conflict[]
  changes: readonly Change[]
  /** The author or a maintainer may record resolutions while the changeset is open. */
  canResolve: boolean
  resolving: boolean
  onResolve: (resolutions: Resolution[]) => void
  tab: string
  onTabChange: (tab: string) => void
}

export function ReviewPanel(props: Props) {
  const { lint, conflicts, changes } = props
  const conflictCount = conflicts?.length ?? 0
  return (
    <Tabs value={props.tab} onValueChange={props.onTabChange} className="h-full min-h-0 gap-0">
      <TabsList className="mx-3 mt-3 w-[calc(100%-1.5rem)] shrink-0">
        <TabsTrigger value="lint">
          Lint
          {lint && lint.errors + lint.warnings > 0 && (
            <CountBadge tone={lint.errors > 0 ? "error" : "warning"} n={lint.errors + lint.warnings} />
          )}
        </TabsTrigger>
        <TabsTrigger value="diff">Diff</TabsTrigger>
        <TabsTrigger value="conflicts">
          Conflicts
          {conflictCount > 0 && <CountBadge tone="error" n={conflictCount} />}
        </TabsTrigger>
        <TabsTrigger value="changes">History</TabsTrigger>
      </TabsList>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <TabsContent value="lint" className="p-3">
          <LintTab {...props} />
        </TabsContent>
        <TabsContent value="diff" className="p-3">
          <DiffTab {...props} />
        </TabsContent>
        <TabsContent value="conflicts" className="p-3">
          <ConflictsTab {...props} />
        </TabsContent>
        <TabsContent value="changes" className="p-3">
          <ChangesTab tree={props.tree} changes={changes} />
        </TabsContent>
      </div>
    </Tabs>
  )
}

function CountBadge({ n, tone }: { n: number; tone: "error" | "warning" }) {
  return (
    <span
      className={cn(
        "ml-1 rounded-full px-1.5 text-[11px] leading-4 font-medium tabular-nums",
        tone === "error" ? "bg-destructive/15 text-destructive" : "bg-amber-500/15 text-amber-600 dark:text-amber-400"
      )}
    >
      {n}
    </span>
  )
}

function useReveal(tree: ElementTree) {
  const reveal = useCanvasStore((s) => s.reveal)
  return (id: string) => {
    if (tree.byId.has(id)) reveal(id, ancestorIds(tree, id))
  }
}

function LintTab({ lint, tree, relations }: Props) {
  const goto = useReveal(tree)
  const relById = useMemo(() => new Map(relations.map((r) => [r.id, r])), [relations])
  if (!lint) return <p className="text-sm text-muted-foreground">Checking…</p>
  if (lint.findings.length === 0) {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <Info className="size-4" aria-hidden /> No lint findings. Errors would block the merge.
      </p>
    )
  }
  return (
    <ul className="flex flex-col gap-1.5" aria-label="Lint findings">
      {lint.findings.map((f, i) => {
        const target =
          f.target_type === "element"
            ? f.target_id
            : f.target_type === "relation"
              ? (relById.get(f.target_id)?.source_id ?? null)
              : null // owners and ownerships are not on the canvas
        const Icon = f.severity === "error" ? CircleAlert : TriangleAlert
        return (
          <li key={`${f.rule}-${f.target_id}-${i}`}>
            <button
              type="button"
              disabled={target === null}
              onClick={() => target && goto(target)}
              className="flex w-full items-start gap-2 rounded-lg border bg-card px-2.5 py-2 text-left text-sm transition-colors hover:border-ring/50 hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:hover:border-border disabled:hover:bg-card"
            >
              <Icon
                className={cn("mt-0.5 size-4 shrink-0", f.severity === "error" ? "text-destructive" : "text-amber-500")}
                aria-label={f.severity}
              />
              <span className="flex flex-col">
                <span>{f.message}</span>
                <span className="font-mono text-[11px] text-muted-foreground">{f.rule}</span>
              </span>
            </button>
          </li>
        )
      })}
    </ul>
  )
}

const statusStyle = {
  added: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400",
  removed: "bg-red-500/15 text-red-700 dark:text-red-400",
  modified: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
} as const
const statusIcon = { added: Plus, removed: Minus, modified: Pencil } as const

function DiffTab({ diff, tree }: Props) {
  return <DiffView diff={diff} tree={tree} />
}

/** Structured diff (added / removed / modified elements and relations); also used by the version browser. */
export function DiffView({
  diff,
  tree,
  emptyText = "No changes yet.",
  showStale = true,
}: {
  diff?: Diff
  tree: ElementTree
  emptyText?: string
  showStale?: boolean
}) {
  const goto = useReveal(tree)
  const known = useOwnerIndex()
  if (!diff) return <p className="text-sm text-muted-foreground">Computing…</p>
  const { elements, relations, owners, ownerships } = diff
  const total =
    elements.added.length +
    elements.removed.length +
    elements.modified.length +
    relations.added.length +
    relations.removed.length +
    relations.modified.length +
    owners.added.length +
    owners.removed.length +
    owners.modified.length +
    ownerships.added.length +
    ownerships.removed.length +
    ownerships.modified.length
  const name = (id: string) => tree.byId.get(id)?.name ?? "unknown"
  // Owners named by the diff itself (added / removed ones may not be in the current model).
  const ownerNames = new Map([...known.values()].map((o) => [o.id, o.name]))
  for (const o of [...owners.added, ...owners.removed, ...owners.modified.map((m) => m.after)])
    ownerNames.set(o.id, o.name)
  const ownerName = (id: string) => ownerNames.get(id) ?? "unknown owner"
  // Reference fields (parent_id, owner_id, ...) hold ids: show the name instead.
  const show = (v: unknown) =>
    typeof v === "string" && tree.byId.has(v)
      ? name(v)
      : typeof v === "string" && ownerNames.has(v)
        ? ownerName(v)
        : formatValue(v)
  const assignment = (o: { element_id: string; owner_id: string }) => `${name(o.element_id)}: ${ownerName(o.owner_id)}`

  return (
    <div className="flex flex-col gap-4">
      {showStale && diff.stale && (
        <p className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-2.5 text-xs">
          Main moved from v{diff.base_version} to v{diff.main_version} since this draft started. Merging rebases it onto
          the current main; conflicts, if any, appear under Conflicts.
        </p>
      )}
      {total === 0 && <p className="text-sm text-muted-foreground">{emptyText}</p>}

      <DiffSection title="Elements added" status="added" count={elements.added.length}>
        {elements.added.map((e) => (
          <DiffRow key={e.id} status="added" label={e.name} meta={e.kind} onClick={() => goto(e.id)} />
        ))}
      </DiffSection>
      <DiffSection title="Elements removed" status="removed" count={elements.removed.length}>
        {elements.removed.map((e) => (
          <DiffRow key={e.id} status="removed" label={e.name} meta={e.kind} />
        ))}
      </DiffSection>
      <DiffSection title="Elements modified" status="modified" count={elements.modified.length}>
        {elements.modified.map((m) => (
          <DiffRow
            key={m.after.id}
            status="modified"
            label={m.after.name}
            meta={m.after.kind}
            onClick={() => goto(m.after.id)}
          >
            <FieldChanges fields={m.fields} show={show} />
          </DiffRow>
        ))}
      </DiffSection>
      <DiffSection title="Relations added" status="added" count={relations.added.length}>
        {relations.added.map((r) => (
          <DiffRow key={r.id} status="added" label={`${name(r.source_id)} → ${name(r.target_id)}`} meta={r.kind} />
        ))}
      </DiffSection>
      <DiffSection title="Relations removed" status="removed" count={relations.removed.length}>
        {relations.removed.map((r) => (
          <DiffRow key={r.id} status="removed" label={`${name(r.source_id)} → ${name(r.target_id)}`} meta={r.kind} />
        ))}
      </DiffSection>
      <DiffSection title="Relations modified" status="modified" count={relations.modified.length}>
        {relations.modified.map((m) => (
          <DiffRow
            key={m.after.id}
            status="modified"
            label={`${name(m.after.source_id)} → ${name(m.after.target_id)}`}
            meta={m.after.kind}
          >
            <FieldChanges fields={m.fields} show={show} />
          </DiffRow>
        ))}
      </DiffSection>
      <DiffSection title="Owners added" status="added" count={owners.added.length}>
        {owners.added.map((o) => (
          <DiffRow key={o.id} status="added" label={o.name} meta={o.kind} />
        ))}
      </DiffSection>
      <DiffSection title="Owners removed" status="removed" count={owners.removed.length}>
        {owners.removed.map((o) => (
          <DiffRow key={o.id} status="removed" label={o.name} meta={o.kind} />
        ))}
      </DiffSection>
      <DiffSection title="Owners modified" status="modified" count={owners.modified.length}>
        {owners.modified.map((m) => (
          <DiffRow key={m.after.id} status="modified" label={m.after.name} meta={m.after.kind}>
            <FieldChanges fields={m.fields} show={show} />
          </DiffRow>
        ))}
      </DiffSection>
      <DiffSection title="Ownership added" status="added" count={ownerships.added.length}>
        {ownerships.added.map((o) => (
          <DiffRow key={o.id} status="added" label={assignment(o)} meta={o.role} onClick={() => goto(o.element_id)} />
        ))}
      </DiffSection>
      <DiffSection title="Ownership removed" status="removed" count={ownerships.removed.length}>
        {ownerships.removed.map((o) => (
          <DiffRow key={o.id} status="removed" label={assignment(o)} meta={o.role} />
        ))}
      </DiffSection>
      <DiffSection title="Ownership modified" status="modified" count={ownerships.modified.length}>
        {ownerships.modified.map((m) => (
          <DiffRow
            key={m.after.id}
            status="modified"
            label={name(m.after.element_id)}
            meta={m.after.role}
            onClick={() => goto(m.after.element_id)}
          >
            <FieldChanges fields={m.fields} show={show} />
          </DiffRow>
        ))}
      </DiffSection>
    </div>
  )
}

function DiffSection({
  title,
  status,
  count,
  children,
}: {
  title: string
  status: keyof typeof statusStyle
  count: number
  children: React.ReactNode
}) {
  if (count === 0) return null
  return (
    <section aria-label={title} className="flex flex-col gap-1.5">
      <h3 className="flex items-center gap-2 px-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
        {title}
        <span className={cn("rounded-full px-1.5 text-[11px] leading-4 tabular-nums", statusStyle[status])}>
          {count}
        </span>
      </h3>
      <ul className="flex flex-col gap-1">{children}</ul>
    </section>
  )
}

function DiffRow({
  status,
  label,
  meta,
  onClick,
  children,
}: {
  status: keyof typeof statusStyle
  label: string
  meta: string
  onClick?: () => void
  children?: React.ReactNode
}) {
  const Icon = statusIcon[status]
  const body = (
    <>
      <span className="flex items-center gap-2 text-sm">
        <span className={cn("grid size-4 shrink-0 place-items-center rounded", statusStyle[status])} aria-hidden>
          <Icon className="size-2.5" strokeWidth={3} />
        </span>
        <span className={cn("truncate font-medium", status === "removed" && "text-muted-foreground line-through")}>
          {label}
        </span>
        <span className="ml-auto shrink-0 font-mono text-[11px] text-muted-foreground">
          {meta.replaceAll("_", " ")}
        </span>
      </span>
      {children}
    </>
  )
  const box = "flex w-full flex-col gap-1.5 rounded-lg border bg-card px-2.5 py-2 text-left"
  return (
    <li>
      {onClick ? (
        <button
          type="button"
          onClick={onClick}
          className={cn(
            box,
            "transition-colors hover:border-ring/50 hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          )}
        >
          {body}
        </button>
      ) : (
        <div className={box}>{body}</div>
      )}
    </li>
  )
}

/** Field-level changes of a modified element or relation: name, old value (struck) and new value. */
function FieldChanges({
  fields,
  show,
}: {
  fields: readonly { field: string; before?: unknown; after?: unknown }[]
  show: (v: unknown) => string
}) {
  return (
    <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 pl-6 text-xs">
      {fields.map((f) => (
        <div key={f.field} className="contents">
          <dt className="font-mono text-muted-foreground">{f.field.replaceAll("_", " ")}</dt>
          <dd className="flex min-w-0 flex-wrap items-center gap-1">
            <span className="max-w-full truncate rounded bg-red-500/10 px-1 text-red-700 line-through decoration-red-500/50 dark:text-red-400">
              {show(f.before)}
            </span>
            <span className="text-muted-foreground" aria-label="changed to">
              →
            </span>
            <span className="max-w-full truncate rounded bg-emerald-500/10 px-1 text-emerald-700 dark:text-emerald-400">
              {show(f.after)}
            </span>
          </dd>
        </div>
      ))}
    </dl>
  )
}

const keyOf = (c: Conflict) => `${c.target_type}:${c.target_id}:${c.field}`

function ConflictsTab({ conflicts, canResolve, resolving, onResolve }: Props) {
  const [choices, setChoices] = useState<Record<string, "mine" | "theirs">>({})
  if (!conflicts) return <p className="text-sm text-muted-foreground">Checking…</p>
  if (conflicts.length === 0) {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <GitMerge className="size-4" aria-hidden /> No conflicts with the current main.
      </p>
    )
  }
  const allChosen = conflicts.every((c) => keyOf(c) in choices)
  return (
    <div className="flex flex-col gap-3">
      <p className="text-xs text-muted-foreground">
        Main changed the same things as this draft. Choose whose version wins for each conflict, then merge again.
      </p>
      <ul className="flex flex-col gap-3" aria-label="Conflicts">
        {conflicts.map((c) => {
          const k = keyOf(c)
          return (
            <li key={k} className="flex flex-col gap-2 rounded-lg border bg-card p-3">
              <div className="flex items-center gap-2 text-sm font-medium">
                <CircleAlert className="size-4 text-destructive" aria-hidden />
                <span className="truncate">{c.label}</span>
                <Badge variant="outline" className="ml-auto">
                  {c.field === "*" ? c.kind.replaceAll("_", " ") : c.field}
                </Badge>
              </div>
              <p className="text-xs text-muted-foreground">{c.message}</p>
              <p className="text-xs text-muted-foreground">
                Base: <span className="font-mono text-foreground">{formatValue(c.base)}</span>
              </p>
              {/* Each option shows the value it keeps, so choosing is one look and one click. */}
              <fieldset className="grid grid-cols-2 gap-1.5" disabled={!canResolve}>
                <legend className="sr-only">Resolution for {c.label}</legend>
                {(["mine", "theirs"] as const).map((choice) => {
                  const on = choices[k] === choice
                  return (
                    <label
                      key={choice}
                      className={cn(
                        "flex cursor-pointer flex-col gap-0.5 rounded-md border px-2.5 py-1.5 text-xs transition-colors has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring",
                        on ? "border-primary bg-primary/5" : "hover:bg-muted/60"
                      )}
                    >
                      <input
                        type="radio"
                        name={k}
                        value={choice}
                        checked={on}
                        onChange={() => setChoices((prev) => ({ ...prev, [k]: choice }))}
                        className="sr-only"
                      />
                      <span className="font-medium">{choice === "mine" ? "Keep mine" : "Keep theirs (main)"}</span>
                      <span className="truncate font-mono text-muted-foreground">
                        {formatValue(choice === "mine" ? c.mine : c.theirs)}
                      </span>
                    </label>
                  )
                })}
              </fieldset>
            </li>
          )
        })}
      </ul>
      <Button
        className="self-end"
        disabled={!canResolve || !allChosen || resolving}
        onClick={() =>
          onResolve(
            conflicts.map((c) => ({
              target_type: c.target_type,
              target_id: c.target_id,
              field: c.field,
              choice: choices[keyOf(c)],
            }))
          )
        }
      >
        Apply resolutions
      </Button>
    </div>
  )
}

function ChangesTab({ tree, changes }: { tree: ElementTree; changes: readonly Change[] }) {
  const batches = useMemo(() => describeBatches(changes, (id) => tree.byId.get(id)?.name ?? "element"), [changes, tree])
  if (batches.length === 0) return <p className="text-sm text-muted-foreground">No edits yet.</p>
  return (
    <AnimatedList delay={40} aria-label="Edit history" className="gap-1.5">
      {batches.map((b) => (
        <AnimatedListItem key={b.batchId}>
          <div className={cn("rounded-lg border bg-card px-2.5 py-1.5 text-sm", b.undone && "opacity-50")}>
            {b.lines.map((l, i) => (
              <p key={i} className={cn("truncate", b.undone && "line-through")}>
                {l}
              </p>
            ))}
            {b.undone && <p className="text-xs text-muted-foreground">undone (redo restores it)</p>}
          </div>
        </AnimatedListItem>
      ))}
    </AnimatedList>
  )
}
