import { Link, createFileRoute } from "@tanstack/react-router"
import { useState } from "react"

import { AppTopBar } from "@/components/app-top-bar"
import { StatePanel } from "@/components/state-panel"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { NewDraftButton } from "@/features/draft/NewDraftDialog"
import { useChangesets, useMe } from "@/features/draft/queries"
import { StatusBadge } from "@/features/draft/StatusBadge"
import type { ChangesetStatus } from "@archmap/api-client"

export const Route = createFileRoute("/changesets/")({ component: ChangesetList })

const filters: { value: ChangesetStatus | "all"; label: string }[] = [
  { value: "all", label: "All" },
  { value: "draft", label: "Drafts" },
  { value: "in_review", label: "In review" },
  { value: "merged", label: "Merged" },
]

const relative = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" })
/** "3 hours ago", "yesterday", ... (falls back to the date after a month). */
function ago(iso: string) {
  const s = (new Date(iso).getTime() - Date.now()) / 1000
  const steps: [Intl.RelativeTimeFormatUnit, number][] = [
    ["second", 60],
    ["minute", 60],
    ["hour", 24],
    ["day", 30],
  ]
  let v = s
  for (const [unit, size] of steps) {
    if (Math.abs(v) < size) return relative.format(Math.round(v), unit)
    v /= size
  }
  return new Date(iso).toLocaleDateString()
}

function ChangesetList() {
  const [filter, setFilter] = useState<ChangesetStatus | "all">("all")
  const list = useChangesets(filter === "all" ? undefined : filter)
  const me = useMe()

  return (
    <div className="flex h-svh flex-col">
      <AppTopBar />
      <main className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-5 overflow-y-auto px-6 py-8">
        <div className="flex flex-col gap-1">
          <h1 className="font-heading text-2xl font-semibold tracking-tight">Changesets</h1>
          <p className="text-sm text-muted-foreground">
            Every edit to the model is proposed in a draft, reviewed, then merged into main.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <ToggleGroup
            value={[filter]}
            onValueChange={(v) => v[0] && setFilter(v[0] as ChangesetStatus | "all")}
            variant="outline"
            size="sm"
            aria-label="Filter by status"
          >
            {filters.map((f) => (
              <ToggleGroupItem key={f.value} value={f.value}>
                {f.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <div className="ml-auto">
            <NewDraftButton />
          </div>
        </div>

        {list.isPending && <StatePanel tone="loading">Loading changesets…</StatePanel>}
        {list.isError && (
          <StatePanel tone="error" title="Could not load changesets" onRetry={() => void list.refetch()}>
            {String(list.error.message).slice(0, 200)}
          </StatePanel>
        )}
        {list.isSuccess && list.data.length === 0 && (
          <StatePanel title={`No changesets${filter === "all" ? " yet" : " with this status"}`}>
            Start a draft to propose a change.
          </StatePanel>
        )}
        {list.isSuccess && list.data.length > 0 && (
          <div className="overflow-hidden rounded-lg border bg-card">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/40 hover:bg-muted/40">
                  <TableHead>Title</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Author</TableHead>
                  <TableHead>Based on</TableHead>
                  <TableHead>Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.data.map((cs) => (
                  <TableRow key={cs.id} className="relative">
                    <TableCell>
                      {/* The whole row is clickable: the link's overlay covers it. */}
                      <Link
                        to="/changesets/$id"
                        params={{ id: cs.id }}
                        className="font-medium after:absolute after:inset-0 focus-visible:outline-none focus-visible:after:ring-2 focus-visible:after:ring-ring focus-visible:after:ring-inset"
                      >
                        {cs.title}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={cs.status} />
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {cs.author_id === me.data?.id ? "You" : cs.author_id.slice(-6)}
                    </TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground tabular-nums">
                      v{cs.base_version}
                    </TableCell>
                    <TableCell
                      className="text-muted-foreground"
                      title={new Date(cs.created_at).toLocaleString()}
                      suppressHydrationWarning
                    >
                      {ago(cs.created_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </main>
    </div>
  )
}
