import { createFileRoute, Link } from "@tanstack/react-router"
import { useEffect, useMemo, useState } from "react"
import { ArrowUpRight, ChevronDown, ChevronRight, GitCommitHorizontal } from "lucide-react"
import type { Version } from "@archmap/api-client"

import { AppTopBar } from "@/components/app-top-bar"
import { StatePanel } from "@/components/state-panel"
import { Badge } from "@/components/ui/badge"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"
import { PropertyPanel } from "@/features/canvas/PropertyPanel"
import { useCanvasStore } from "@/features/canvas/store"
import { groupVersions } from "@/features/editing/direct"
import { useDirectory } from "@/features/users/queries"
import { buildTree } from "@/features/canvas/graph"
import { diffOverlay } from "@/features/draft/decor"
import { useMainVersion } from "@/features/draft/queries"
import { DiffView } from "@/features/draft/ReviewPanel"
import { useVersionDiff, useVersionModel } from "@/features/versions/queries"
import { OwnersProvider } from "@/features/canvas/owners"
import { ModelWorkspace } from "@/features/workspace/ModelWorkspace"

type Search = { v?: number; base?: number; focus?: string }
const asNumber = (x: unknown) => (typeof x === "number" && Number.isInteger(x) && x >= 1 ? x : undefined)

export const Route = createFileRoute("/versions")({
  component: VersionBrowser,
  validateSearch: (s: Record<string, unknown>): Search => ({
    v: asNumber(s.v),
    base: asNumber(s.base),
    focus: typeof s.focus === "string" ? s.focus : undefined,
  }),
})

const when = (iso: string) => new Date(iso).toLocaleString()
const short = (iso: string) =>
  new Date(iso).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })

/** Merged versions of main: browse any version read-only and see what each one changed. */
function VersionBrowser() {
  const versions = useMainVersion()
  const { v, base, focus } = Route.useSearch()
  // The canvas store is shared with main and drafts: start from a clean canvas here.
  useEffect(() => {
    const store = useCanvasStore.getState()
    store.loadLayout({}, [])
    store.select(null)
    store.clearFilter()
  }, [])
  const navigate = Route.useNavigate()
  const list = versions.data ?? []

  const latest: number | undefined = list.length > 0 ? list[0].number : undefined
  const selected = v !== undefined && list.some((x) => x.number === v) ? v : latest
  // Compare with the previous version unless another one was chosen.
  const compareWith =
    base !== undefined && base !== selected && list.some((x) => x.number === base) ? base : (selected ?? 1) - 1
  const hasBase = selected !== undefined && compareWith >= 1 && list.some((x) => x.number === compareWith)

  const model = useVersionModel(selected)
  const diff = useVersionDiff(hasBase ? compareWith : undefined, hasBase ? selected : undefined)
  const [tab, setTab] = useState("model")
  const [highlight, setHighlight] = useState(true)

  const overlay = useMemo(() => diffOverlay(highlight ? diff.data : undefined), [highlight, diff.data])
  const elements = useMemo(
    () => (model.data ? [...model.data.elements, ...overlay.removedElements] : []),
    [model.data, overlay.removedElements]
  )
  const relations = useMemo(
    () => (model.data ? [...model.data.relations, ...overlay.removedRelations] : []),
    [model.data, overlay.removedRelations]
  )
  // Names in the diff also cover what the version no longer has.
  const diffTree = useMemo(
    () => buildTree([...(model.data?.elements ?? []), ...(diff.data?.elements.removed ?? [])]),
    [model.data, diff.data]
  )

  // Consecutive direct edits by one person are folded into one row (ADR-072).
  const groups = useMemo(() => groupVersions(list), [list])
  const [openGroups, setOpenGroups] = useState<ReadonlySet<string>>(() => new Set())
  const toggleGroup = (key: string) =>
    setOpenGroups((cur) => {
      const next = new Set(cur)
      if (!next.delete(key)) next.add(key)
      return next
    })
  const directory = useDirectory()
  const userName = (id: string) => directory.data?.users.find((u) => u.id === id)?.name ?? "Someone"
  const renderItem = (x: Version) => {
    const on = x.number === selected
    return (
      <li key={x.number}>
        <Link
          to="/versions"
          search={{ v: x.number }}
          aria-current={on ? "true" : undefined}
          className={cn(
            "relative flex gap-3 rounded-lg py-2 pr-2 pl-2.5 text-sm transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
            on && "bg-primary/10 hover:bg-primary/15"
          )}
        >
          <span
            className={cn(
              "relative z-10 mt-1 grid size-3.5 shrink-0 place-items-center rounded-full border-2 bg-sidebar",
              on ? "border-primary" : "border-muted-foreground/40"
            )}
            aria-hidden
          >
            {on && <span className="size-1.5 rounded-full bg-primary" />}
          </span>
          <span className="flex min-w-0 flex-col gap-0.5">
            <span className="flex items-center gap-2">
              <span className="font-mono text-xs font-semibold tabular-nums">v{x.number}</span>
              {x.number === latest && (
                <span className="rounded-full bg-emerald-500/15 px-1.5 text-[10px] font-medium text-emerald-700 dark:text-emerald-400">
                  current
                </span>
              )}
              {x.kind === "direct" && (
                <span className="rounded bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground">
                  {x.reverts ? "undo" : "direct"}
                </span>
              )}
              <span className="ml-auto text-[11px] text-muted-foreground">{short(x.merged_at)}</span>
            </span>
            <span className={cn("line-clamp-2", on ? "text-foreground" : "text-muted-foreground")}>
              {x.summary || "No summary"}
            </span>
          </span>
        </Link>
      </li>
    )
  }

  const current = list.find((x) => x.number === selected)
  const others = list.filter((x) => x.number !== selected)

  return (
    <div className="flex h-svh flex-col">
      <AppTopBar
        context={
          <>
            {selected !== undefined && (
              <Badge variant="outline" className="font-mono">
                v{selected}
              </Badge>
            )}
          </>
        }
      />

      {versions.isPending && <StatePanel tone="loading">Loading versions…</StatePanel>}
      {versions.isError && (
        <StatePanel tone="error" title="Could not load versions" onRetry={() => void versions.refetch()}>
          {String(versions.error.message).slice(0, 200)}
        </StatePanel>
      )}
      {versions.isSuccess && list.length === 0 && (
        <StatePanel title="No versions yet">
          Main gets its first version when a changeset is merged (or `task seed` runs).
        </StatePanel>
      )}

      {versions.isSuccess && current && selected !== undefined && (
        <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[288px_minmax(0,1fr)]">
          <aside
            className="flex min-h-0 flex-col overflow-y-auto border-r bg-sidebar text-sidebar-foreground"
            aria-label="Versions"
          >
            <div className="flex items-center justify-between px-4 pt-4 pb-2">
              <span className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">History</span>
              <span className="text-[11px] text-muted-foreground tabular-nums">{list.length}</span>
            </div>
            {/* Timeline: a rail with one dot per merged version, newest first. */}
            <ol className="relative flex flex-col px-2 pb-3 before:absolute before:top-3 before:bottom-6 before:left-[1.4rem] before:w-px before:bg-border">
              {groups.map((g) => {
                if (g.versions.length === 1) return renderItem(g.versions[0])
                const first = g.versions[0]
                const last = g.versions[g.versions.length - 1]
                const expanded = openGroups.has(g.key) || g.versions.some((x) => x.number === selected)
                return (
                  <li key={g.key}>
                    <button
                      type="button"
                      aria-expanded={expanded}
                      onClick={() => toggleGroup(g.key)}
                      className="relative flex w-full gap-3 rounded-lg py-2 pr-2 pl-2.5 text-left text-sm text-muted-foreground transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                    >
                      <span
                        className="relative z-10 mt-0.5 grid size-3.5 shrink-0 place-items-center rounded-full border-2 border-dashed border-muted-foreground/40 bg-sidebar"
                        aria-hidden
                      />
                      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                        <span className="flex items-center gap-2">
                          <span className="font-mono text-xs font-semibold tabular-nums">
                            v{last.number}–v{first.number}
                          </span>
                          <span className="ml-auto text-[11px]">{short(first.merged_at)}</span>
                        </span>
                        <span className="flex items-center gap-1">
                          {expanded ? (
                            <ChevronDown className="size-3.5 shrink-0" aria-hidden />
                          ) : (
                            <ChevronRight className="size-3.5 shrink-0" aria-hidden />
                          )}
                          {g.versions.length} direct edits · {userName(first.merged_by)}
                        </span>
                      </span>
                    </button>
                    {expanded && <ol className="flex flex-col">{g.versions.map(renderItem)}</ol>}
                  </li>
                )
              })}
            </ol>
          </aside>

          <section className="flex min-h-0 min-w-0 flex-col" aria-label={`Version ${selected}`}>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-2.5">
              <span className="grid size-7 place-items-center rounded-lg border bg-card text-muted-foreground">
                <GitCommitHorizontal className="size-4" aria-hidden />
              </span>
              <div className="flex min-w-0 flex-col">
                <h1 className="font-heading text-[15px] leading-tight font-semibold tracking-tight">
                  Version {selected}
                  {current.summary && <span className="font-normal text-muted-foreground"> · {current.summary}</span>}
                </h1>
                <span className="text-xs text-muted-foreground">merged {when(current.merged_at)}</span>
              </div>
              <Link
                to="/changesets/$id"
                params={{ id: current.changeset_id }}
                className="inline-flex items-center gap-0.5 text-xs text-muted-foreground hover:text-foreground"
              >
                Changeset <ArrowUpRight className="size-3" aria-hidden />
              </Link>
              <div className="ml-auto flex items-center gap-2">
                <Label htmlFor="compare-with" className="text-sm text-muted-foreground">
                  Compare with
                </Label>
                <Select
                  value={hasBase ? String(compareWith) : null}
                  onValueChange={(n) => {
                    if (n) void navigate({ search: { v: selected, base: Number(n) } })
                  }}
                  items={others.map((x) => ({ value: String(x.number), label: `v${x.number}` }))}
                  disabled={others.length === 0}
                >
                  <SelectTrigger id="compare-with" size="sm" className="w-28">
                    <SelectValue placeholder="none" />
                  </SelectTrigger>
                  <SelectContent>
                    {others.map((x) => (
                      <SelectItem key={x.number} value={String(x.number)}>
                        v{x.number}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <Tabs value={tab} onValueChange={setTab} className="min-h-0 flex-1 gap-0">
              <div className="flex items-center gap-3 px-4 pt-3 pb-1">
                <TabsList className="w-auto">
                  <TabsTrigger value="model">Model</TabsTrigger>
                  <TabsTrigger value="changes">Changes</TabsTrigger>
                </TabsList>
                {tab === "model" && hasBase && (
                  <label className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Switch checked={highlight} onCheckedChange={setHighlight} aria-label="Highlight changes" />
                    Highlight changes since v{compareWith}
                  </label>
                )}
              </div>

              <TabsContent value="model" className="flex min-h-0 flex-col pt-2">
                {model.isPending && <StatePanel tone="loading">Loading version {selected}…</StatePanel>}
                {model.isError && (
                  <StatePanel tone="error" title="Could not load this version" onRetry={() => void model.refetch()}>
                    {String(model.error.message).slice(0, 200)}
                  </StatePanel>
                )}
                {model.isSuccess && model.data.elements.length === 0 && (
                  <StatePanel title="This version is empty">It has no elements.</StatePanel>
                )}
                {model.isSuccess && model.data.elements.length > 0 && (
                  <ModelWorkspace
                    viewKey={`v${selected}`}
                    elements={elements}
                    relations={relations}
                    owners={model.data.owners}
                    decor={{ diff: overlay.elements, relationDiff: overlay.relations }}
                    renderPanel={(tree) => <PropertyPanel tree={tree} relations={relations} />}
                    focusId={focus}
                    onFocusChange={(id) => void navigate({ search: (prev) => ({ ...prev, focus: id ?? undefined }) })}
                  />
                )}
              </TabsContent>

              <TabsContent value="changes" className="min-h-0 overflow-y-auto px-4 py-4">
                {!hasBase && (
                  <p className="text-sm text-muted-foreground">This is the first version; nothing to compare with.</p>
                )}
                {hasBase && diff.isError && (
                  <StatePanel tone="error" title="Could not compute the changes" onRetry={() => void diff.refetch()}>
                    {String(diff.error.message).slice(0, 200)}
                  </StatePanel>
                )}
                {hasBase && !diff.isError && (
                  <div className="flex max-w-3xl flex-col gap-4">
                    <p className="flex items-center gap-2 text-sm text-muted-foreground">
                      Comparing
                      <Badge variant="outline" className="font-mono">
                        v{compareWith}
                      </Badge>
                      →
                      <Badge variant="outline" className="font-mono">
                        v{selected}
                      </Badge>
                    </p>
                    <OwnersProvider owners={model.data?.owners ?? []}>
                      <DiffView
                        diff={diff.data}
                        tree={diffTree}
                        showStale={false}
                        emptyText="The two versions are identical."
                      />
                    </OwnersProvider>
                  </div>
                )}
              </TabsContent>
            </Tabs>
          </section>
        </div>
      )}
    </div>
  )
}
