import type { ChangesetStatus } from "@archmap/api-client"

import { cn } from "@/lib/utils"
import { statusLabel } from "./workflow"

const tone: Record<ChangesetStatus, string> = {
  draft: "bg-sky-500/10 text-sky-700 dark:text-sky-400",
  in_review: "bg-amber-500/15 text-amber-700 dark:text-amber-400",
  merged: "bg-violet-500/15 text-violet-700 dark:text-violet-400",
  rejected: "bg-red-500/15 text-red-700 dark:text-red-400",
  abandoned: "bg-muted text-muted-foreground",
}

/** Changeset status as a small colored pill with a dot. */
export function StatusBadge({ status, className }: { status: ChangesetStatus; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex h-5 shrink-0 items-center gap-1.5 rounded-full px-2 text-xs font-medium whitespace-nowrap",
        tone[status],
        className
      )}
    >
      <span className="size-1.5 rounded-full bg-current" aria-hidden />
      {statusLabel[status]}
    </span>
  )
}
