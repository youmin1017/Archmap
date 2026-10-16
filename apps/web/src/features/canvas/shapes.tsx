import { Server } from "lucide-react"
import type { Element } from "@archmap/api-client"

import { cn } from "@/lib/utils"
import { kindColor } from "./kind"

/** How a card is outlined: normal, selected, or marked by the diff overlay. */
export type ShapeTone = "default" | "selected" | "added" | "modified" | "removed"

const toneStroke: Record<ShapeTone, string> = {
  default: "var(--kind)",
  selected: "var(--primary)",
  added: "#10b981",
  modified: "#f59e0b",
  removed: "#ef4444",
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
    strokeWidth: tone === "default" ? 1.25 : 2,
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
      <path d={topFull} {...common} style={{ ...common.style, fill: "var(--kind)", fillOpacity: 0.14 }} />
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
            className="absolute inset-0 rounded-xl border border-(--kind)/40 bg-card"
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
      className="shrink-0 rounded-full bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground tabular-nums"
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
      className="inline-flex max-w-[55%] min-w-0 shrink items-center gap-1 rounded-full bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground"
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
