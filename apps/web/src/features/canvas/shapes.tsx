import { Minus, Pencil, Plus, Server, type LucideIcon } from "lucide-react"
import type { Element } from "@archmap/api-client"

import { cn } from "@/lib/utils"
import { kindColor } from "./kind"

/** How a card is outlined: normal, selected, or marked by the diff overlay. */
export type ShapeTone = "default" | "selected" | "added" | "modified" | "removed"

const toneStroke: Record<ShapeTone, string> = {
  default: "color-mix(in oklch, var(--foreground) 22%, transparent)",
  selected: "var(--primary)",
  added: "#10b981",
  modified: "#f59e0b",
  removed: "#ef4444",
}

/**
 * Outline of an element card (not the database cylinder, which draws its own):
 * a neutral hairline border; selection and the diff overlay only recolor it.
 */
export function cardClass(tone: ShapeTone) {
  return cn(
    "rounded-lg border bg-card shadow-[0_1px_2px_0_oklch(0_0_0/0.05)] transition-[border-color,box-shadow,opacity]",
    tone === "default" && "border-border hover:border-foreground/20",
    tone === "selected" && "border-primary ring-3 ring-primary/15",
    tone === "added" && "border-emerald-500/70",
    tone === "modified" && "border-amber-500/70",
    tone === "removed" && "border-dashed border-red-500/60 opacity-60"
  )
}

const diffMark: Record<"added" | "modified" | "removed", { icon: LucideIcon; label: string; className: string }> = {
  added: { icon: Plus, label: "Added", className: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400" },
  modified: { icon: Pencil, label: "Modified", className: "bg-amber-500/15 text-amber-700 dark:text-amber-400" },
  removed: { icon: Minus, label: "Removed", className: "bg-red-500/15 text-red-700 dark:text-red-400" },
}

/** Small +/✎/− square showing the element's diff status (same marks as the review panel). */
export function DiffMark({ diff }: { diff?: "added" | "modified" | "removed" }) {
  if (!diff) return null
  const { icon: Icon, label, className } = diffMark[diff]
  return (
    <span className={cn("grid size-4 shrink-0 place-items-center rounded", className)} title={label}>
      <Icon className="size-2.5" strokeWidth={3} aria-hidden />
    </span>
  )
}

/** The element's kind icon on a faint tile of the kind color (the card's only kind accent). */
export function KindTile({ icon: Icon }: { icon: LucideIcon }) {
  return (
    <span className="grid size-5 shrink-0 place-items-center rounded-md bg-(--kind)/12 text-(--kind)">
      <Icon className="size-3.5" aria-hidden />
    </span>
  )
}

const cap = 12 // vertical radius of the cylinder's ellipses, in viewBox units (of 100)
const rim = 14 // y of the ellipses' centres at the top

const body = `M0.5 ${rim} A49.5 ${cap} 0 0 1 99.5 ${rim} V${100 - rim} A49.5 ${cap} 0 0 1 0.5 ${100 - rim} Z`
const topFull = `M0.5 ${rim} A49.5 ${cap} 0 0 1 99.5 ${rim} A49.5 ${cap} 0 0 1 0.5 ${rim} Z`

/**
 * A database cylinder that stretches to its card (the stroke keeps its width).
 * It needs `--kind` on an ancestor; the card's text sits on top of it.
 */
export function DatabaseShape({
  tone = "default",
  dashed = false,
  muted = false,
  className,
}: {
  tone?: ShapeTone
  dashed?: boolean
  /** Fainter outline, used for the stacked replicas behind a card. */
  muted?: boolean
  className?: string
}) {
  const stroke = toneStroke[tone]
  const common = {
    vectorEffect: "non-scaling-stroke",
    strokeWidth: tone === "default" ? 1 : 1.5,
    strokeDasharray: dashed ? "5 3" : undefined,
    style: { stroke, strokeOpacity: muted ? 0.5 : 1 },
  } as const
  return (
    <svg
      aria-hidden
      viewBox="0 0 100 100"
      preserveAspectRatio="none"
      className={cn("pointer-events-none absolute inset-0 size-full overflow-visible", className)}
    >
      <path d={body} {...common} style={{ ...common.style, fill: "var(--card)" }} />
      <path d={topFull} {...common} style={{ ...common.style, fill: "var(--kind)", fillOpacity: 0.1 }} />
    </svg>
  )
}

/** Up to two offset copies behind a card that stands for several replicas. */
export function ReplicaBackdrop({ element }: { element: Pick<Element, "kind" | "replicas"> }) {
  if (element.replicas <= 1) return null
  const copies = element.replicas >= 3 ? [10, 5] : [5]
  return (
    <div
      aria-hidden
      className="pointer-events-none absolute inset-0"
      style={{ ["--kind" as string]: kindColor(element.kind) }}
    >
      {copies.map((offset) =>
        element.kind === "database" ? (
          <div key={offset} className="absolute inset-0" style={{ transform: `translate(${offset}px, ${offset}px)` }}>
            <DatabaseShape muted />
          </div>
        ) : (
          <div
            key={offset}
            className="absolute inset-0 rounded-lg border bg-card"
            style={{ transform: `translate(${offset}px, ${offset}px)` }}
          />
        )
      )}
    </div>
  )
}

/** "x3" badge for elements with several replicas. */
export function ReplicaBadge({ replicas }: { replicas: number }) {
  if (replicas <= 1) return null
  return (
    <span
      className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground tabular-nums"
      title={`${replicas} replicas`}
    >
      ×{replicas}
    </span>
  )
}

/** Where an element runs: "on Docker Host" (+N when it runs on several nodes). */
export function HostBadge({ hosts }: { hosts: readonly string[] }) {
  if (hosts.length === 0) return null
  const first = hosts[0]
  return (
    <span
      className="inline-flex max-w-[55%] min-w-0 shrink items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground"
      title={`Deployed on ${hosts.join(", ")}`}
    >
      <Server className="size-3 shrink-0" aria-hidden />
      <span className="truncate">{hosts.length > 1 ? `${first} +${hosts.length - 1}` : first}</span>
    </span>
  )
}

/** "10.0.0.5:5432" from the ip and port properties, when set. */
export function endpointOf(properties: Readonly<Record<string, string>>): string | null {
  const ip = properties.ip
  if (!ip) return null
  return properties.port ? `${ip}:${properties.port}` : ip
}
