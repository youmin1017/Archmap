import { Link, createFileRoute } from "@tanstack/react-router"
import { useState } from "react"

import { AppTopBar } from "@/components/app-top-bar"
import { Badge } from "@/components/ui/badge"
import { StatePanel } from "@/components/state-panel"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { NewDraftButton } from "@/features/draft/NewDraftDialog"
import { useChangesets, useMe } from "@/features/draft/queries"
import { statusLabel } from "@/features/draft/workflow"
import type { ChangesetStatus } from "@archmap/api-client"

export const Route = createFileRoute("/changesets/")({ component: ChangesetList })

const filters: { value: ChangesetStatus | "all"; label: string }[] = [
  { value: "all", label: "All" },
  { value: "draft", label: "Drafts" },
  { value: "in_review", label: "In review" },
  { value: "merged", label: "Merged" },
]

export const statusTone: Record<ChangesetStatus, string> = {
  draft: "bg-secondary text-secondary-foreground",
  in_review: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
  merged: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400",
  rejected: "bg-red-500/15 text-red-700 dark:text-red-400",
  abandoned: "bg-muted text-muted-foreground",
}

function ChangesetList() {
  const [filter, setFilter] = useState<ChangesetStatus | "all">("all")
  const list = useChangesets(filter === "all" ? undefined : filter)
  const me = useMe()

  return (
    <div className="flex h-svh flex-col">
      <AppTopBar context={<Badge variant="secondary">Changesets</Badge>} />
      <main className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-4 overflow-y-auto p-6">
        <div className="flex items-center gap-3">
          <h1 className="font-heading text-xl font-semibold">Changesets</h1>
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
          <div className="rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Title</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Author</TableHead>
                  <TableHead>Based on</TableHead>
                  <TableHead>Created</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.data.map((cs) => (
                  <TableRow key={cs.id}>
                    <TableCell>
                      <Link to="/changesets/$id" params={{ id: cs.id }} className="font-medium hover:underline">
                        {cs.title}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <span className={`rounded-full px-2 py-0.5 text-xs ${statusTone[cs.status]}`}>
                        {statusLabel[cs.status]}
                      </span>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {cs.author_id === me.data?.id ? "You" : cs.author_id.slice(-6)}
                    </TableCell>
                    <TableCell className="text-muted-foreground tabular-nums">v{cs.base_version}</TableCell>
                    <TableCell className="text-muted-foreground">{new Date(cs.created_at).toLocaleString()}</TableCell>
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
