import { createFileRoute, Link } from "@tanstack/react-router"
import { useMemo, useState } from "react"
import { History } from "lucide-react"

import { AppTopBar } from "@/components/app-top-bar"
import { StatePanel } from "@/components/state-panel"
import { Badge } from "@/components/ui/badge"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"
import { PropertyPanel } from "@/features/canvas/PropertyPanel"
import { buildTree } from "@/features/canvas/graph"
import { diffOverlay } from "@/features/draft/decor"
import { useMainVersion } from "@/features/draft/queries"
import { DiffView } from "@/features/draft/ReviewPanel"
import { useVersionDiff, useVersionModel } from "@/features/versions/queries"
import { ModelWorkspace } from "@/features/workspace/ModelWorkspace"

type Search = { v?: number; base?: number }
const asNumber = (x: unknown) => (typeof x === "number" && Number.isInteger(x) && x >= 1 ? x : undefined)

export const Route = createFileRoute("/versions")({
  component: VersionBrowser,
  validateSearch: (s: Record<string, unknown>): Search => ({ v: asNumber(s.v), base: asNumber(s.base) }),
})

const when = (iso: string) => new Date(iso).toLocaleString()

/** Merged versions of main: browse any version read-only and see what each one changed. */
function VersionBrowser() {
  const versions = useMainVersion()
  const { v, base } = Route.useSearch()
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

  const current = list.find((x) => x.number === selected)
  const others = list.filter((x) => x.number !== selected)

  return (
    <div className="flex h-svh flex-col">
      <AppTopBar
        context={
          <Badge variant="secondary">
            <History data-icon="inline-start" aria-hidden /> Versions
          </Badge>
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
        <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[300px_minmax(0,1fr)]">
          <aside
            className="min-h-0 overflow-y-auto border-r bg-sidebar p-3 text-sidebar-foreground"
            aria-label="Versions"
          >
            <ol className="flex flex-col gap-1">
              {list.map((x) => (
                <li key={x.number}>
                  <Link
                    to="/versions"
                    search={{ v: x.number }}
                    aria-current={x.number === selected ? "true" : undefined}
                    className={cn(
                      "flex flex-col gap-0.5 rounded-xl border border-transparent px-3 py-2 text-sm hover:bg-accent",
                      x.number === selected && "border-border bg-accent"
                    )}
                  >
                    <span className="flex items-center gap-2">
                      <span className="font-heading font-semibold tabular-nums">v{x.number}</span>
                      {x.number === latest && <Badge variant="secondary">Current</Badge>}
                    </span>
                    <span className="line-clamp-2 text-muted-foreground">{x.summary || "No summary"}</span>
                    <span className="text-xs text-muted-foreground">{when(x.merged_at)}</span>
                  </Link>
                </li>
              ))}
            </ol>
          </aside>

          <section className="flex min-h-0 min-w-0 flex-col" aria-label={`Version ${selected}`}>
            <div className="flex flex-wrap items-center gap-3 border-b px-4 py-2">
              <h1 className="font-heading text-base font-semibold">Version {selected}</h1>
              <span className="text-sm text-muted-foreground">merged {when(current.merged_at)}</span>
              <Link
                to="/changesets/$id"
                params={{ id: current.changeset_id }}
                className="text-sm text-muted-foreground underline-offset-2 hover:underline"
              >
                Open changeset
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
              <div className="flex items-center gap-3 px-4 pt-3">
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
                    decor={{ diff: overlay.elements, relationDiff: overlay.relations }}
                    renderPanel={(tree) => <PropertyPanel tree={tree} relations={relations} />}
                  />
                )}
              </TabsContent>

              <TabsContent value="changes" className="min-h-0 overflow-y-auto p-4">
                {!hasBase && (
                  <p className="text-sm text-muted-foreground">This is the first version; nothing to compare with.</p>
                )}
                {hasBase && diff.isError && (
                  <StatePanel tone="error" title="Could not compute the changes" onRetry={() => void diff.refetch()}>
                    {String(diff.error.message).slice(0, 200)}
                  </StatePanel>
                )}
                {hasBase && !diff.isError && (
                  <div className="mx-auto flex max-w-2xl flex-col gap-3">
                    <p className="text-sm text-muted-foreground">
                      Going from v{compareWith} to v{selected}.
                    </p>
                    <DiffView
                      diff={diff.data}
                      tree={diffTree}
                      showStale={false}
                      emptyText="The two versions are identical."
                    />
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
