import { useMemo, useState } from "react"
import { CircleAlert, GitMerge, Info, TriangleAlert } from "lucide-react"
import type { Change, Conflict, Diff, LintReport, Relation, Resolution } from "@archmap/api-client"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { AnimatedList, AnimatedListItem } from "@/components/ui/animated-list"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"
import { ancestorIds, type ElementTree } from "../canvas/graph"
import { useCanvasStore } from "../canvas/store"
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
      <TabsList className="mx-3 mt-3 w-auto">
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
        const target = f.target_type === "element" ? f.target_id : (relById.get(f.target_id)?.source_id ?? "")
        const Icon = f.severity === "error" ? CircleAlert : TriangleAlert
        return (
          <li key={`${f.rule}-${f.target_id}-${i}`}>
            <button
              type="button"
              onClick={() => goto(target)}
              className="flex w-full items-start gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            >
              <Icon
                className={cn("mt-0.5 size-4 shrink-0", f.severity === "error" ? "text-destructive" : "text-amber-500")}
                aria-label={f.severity}
              />
              <span className="flex flex-col">
                <span>{f.message}</span>
                <span className="text-xs text-muted-foreground">{f.rule}</span>
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
  if (!diff) return <p className="text-sm text-muted-foreground">Computing…</p>
  const { elements, relations } = diff
  const total =
    elements.added.length +
    elements.removed.length +
    elements.modified.length +
    relations.added.length +
    relations.removed.length +
    relations.modified.length
  const name = (id: string) => tree.byId.get(id)?.name ?? "unknown"

  return (
    <div className="flex flex-col gap-3">
      {showStale && diff.stale && (
        <p className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-2 text-xs">
          Main moved from v{diff.base_version} to v{diff.main_version} since this draft started. Merging rebases it onto
          the current main; conflicts, if any, appear under Conflicts.
        </p>
      )}
      {total === 0 && <p className="text-sm text-muted-foreground">{emptyText}</p>}

      <DiffSection title="Elements added" status="added" count={elements.added.length}>
        {elements.added.map((e) => (
          <DiffRow key={e.id} label={e.name} meta={e.kind} onClick={() => goto(e.id)} />
        ))}
      </DiffSection>
      <DiffSection title="Elements removed" status="removed" count={elements.removed.length}>
        {elements.removed.map((e) => (
          <DiffRow key={e.id} label={e.name} meta={e.kind} />
        ))}
      </DiffSection>
      <DiffSection title="Elements modified" status="modified" count={elements.modified.length}>
        {elements.modified.map((m) => (
          <DiffRow key={m.after.id} label={m.after.name} meta={m.after.kind} onClick={() => goto(m.after.id)}>
            {m.fields.map((f) => (
              <span key={f.field} className="block text-xs text-muted-foreground">
                {f.field}: <s>{formatValue(f.before)}</s> →{" "}
                <b className="font-medium text-foreground">{formatValue(f.after)}</b>
              </span>
            ))}
          </DiffRow>
        ))}
      </DiffSection>
      <DiffSection title="Relations added" status="added" count={relations.added.length}>
        {relations.added.map((r) => (
          <DiffRow key={r.id} label={`${name(r.source_id)} → ${name(r.target_id)}`} meta={r.kind} />
        ))}
      </DiffSection>
      <DiffSection title="Relations removed" status="removed" count={relations.removed.length}>
        {relations.removed.map((r) => (
          <DiffRow key={r.id} label={`${name(r.source_id)} → ${name(r.target_id)}`} meta={r.kind} />
        ))}
      </DiffSection>
      <DiffSection title="Relations modified" status="modified" count={relations.modified.length}>
        {relations.modified.map((m) => (
          <DiffRow
            key={m.after.id}
            label={`${name(m.after.source_id)} → ${name(m.after.target_id)}`}
            meta={m.after.kind}
          >
            {m.fields.map((f) => (
              <span key={f.field} className="block text-xs text-muted-foreground">
                {f.field}: <s>{formatValue(f.before)}</s> →{" "}
                <b className="font-medium text-foreground">{formatValue(f.after)}</b>
              </span>
            ))}
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
    <section aria-label={title} className="flex flex-col gap-1">
      <h3 className="flex items-center gap-2 text-sm font-medium">
        {title}
        <span className={cn("rounded-full px-1.5 text-[11px] leading-4 tabular-nums", statusStyle[status])}>
          {count}
        </span>
      </h3>
      <ul className="flex flex-col gap-0.5">{children}</ul>
    </section>
  )
}

function DiffRow({
  label,
  meta,
  onClick,
  children,
}: {
  label: string
  meta: string
  onClick?: () => void
  children?: React.ReactNode
}) {
  const body = (
    <>
      <span className="flex items-center gap-2 text-sm">
        <span className="truncate">{label}</span>
        <span className="ml-auto text-xs text-muted-foreground">{meta}</span>
      </span>
      {children}
    </>
  )
  return (
    <li>
      {onClick ? (
        <button
          type="button"
          onClick={onClick}
          className="flex w-full flex-col rounded-lg px-2 py-1 text-left hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          {body}
        </button>
      ) : (
        <div className="flex flex-col px-2 py-1">{body}</div>
      )}
    </li>
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
            <li key={k} className="flex flex-col gap-2 rounded-xl border p-3">
              <div className="flex items-center gap-2 text-sm font-medium">
                <CircleAlert className="size-4 text-destructive" aria-hidden />
                <span className="truncate">{c.label}</span>
                <Badge variant="outline" className="ml-auto">
                  {c.field === "*" ? c.kind.replaceAll("_", " ") : c.field}
                </Badge>
              </div>
              <p className="text-xs text-muted-foreground">{c.message}</p>
              <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
                <dt className="text-muted-foreground">Base</dt>
                <dd className="truncate">{formatValue(c.base)}</dd>
                <dt className="text-muted-foreground">Mine</dt>
                <dd className="truncate">{formatValue(c.mine)}</dd>
                <dt className="text-muted-foreground">Theirs</dt>
                <dd className="truncate">{formatValue(c.theirs)}</dd>
              </dl>
              <fieldset className="flex gap-4 text-sm" disabled={!canResolve}>
                <legend className="sr-only">Resolution for {c.label}</legend>
                {(["mine", "theirs"] as const).map((choice) => (
                  <label key={choice} className="flex items-center gap-1.5">
                    <input
                      type="radio"
                      name={k}
                      value={choice}
                      checked={choices[k] === choice}
                      onChange={() => setChoices((prev) => ({ ...prev, [k]: choice }))}
                    />
                    Keep {choice}
                  </label>
                ))}
              </fieldset>
            </li>
          )
        })}
      </ul>
      <Button
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
          <div className={cn("rounded-lg border px-2.5 py-1.5 text-sm", b.undone && "opacity-50")}>
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
