import { memo } from "react"
import { BaseEdge, EdgeLabelRenderer, getBezierPath, type Edge, type EdgeProps } from "@xyflow/react"
import type { RelationKind } from "@archmap/api-client"

import type { DiffStatus } from "./decor"

import { cn } from "@/lib/utils"

export type AggregatedEdgeData = {
  /** Number of model relations this edge stands for (> 1 when collapsed nodes aggregate). */
  count: number
  kinds: Partial<Record<RelationKind, number>>
  dimmed: boolean
  diff?: DiffStatus
  /** Relations this edge stands for (used to select them in the property panel). */
  relationIds: string[]
  [key: string]: unknown
}

export type AggregatedFlowEdge = Edge<AggregatedEdgeData, "aggregated">

const diffStroke: Partial<Record<string, string>> = {
  added: "oklch(0.65 0.17 150)",
  modified: "oklch(0.75 0.15 80)",
  removed: "var(--destructive)",
}

function AggregatedEdgeImpl({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  markerEnd,
  data,
  selected,
}: EdgeProps<AggregatedFlowEdge>) {
  const [path, labelX, labelY] = getBezierPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition })
  const count = data?.count ?? 1
  // A pair that only has deployment relations is drawn dashed so infrastructure reads differently from usage.
  const deploymentOnly = data ? Object.keys(data.kinds).every((k) => k === "deployed_on") : false

  return (
    <>
      <BaseEdge
        id={id}
        path={path}
        markerEnd={markerEnd}
        style={{
          stroke: selected ? "var(--primary)" : (diffStroke[data?.diff ?? ""] ?? "var(--muted-foreground)"),
          strokeWidth: selected ? 2 : 1.5,
          strokeDasharray: deploymentOnly || data?.diff === "removed" ? "6 4" : undefined,
          opacity: data?.dimmed ? 0.2 : 1,
        }}
      />
      {count > 1 && (
        <EdgeLabelRenderer>
          <span
            className={cn(
              "nodrag nopan pointer-events-none absolute rounded-full border bg-background px-1.5 py-0.5 text-[11px] leading-none font-medium text-muted-foreground tabular-nums",
              data?.dimmed && "opacity-30"
            )}
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
            aria-label={`${count} relations`}
          >
            {count}
          </span>
        </EdgeLabelRenderer>
      )}
    </>
  )
}

export const AggregatedEdge = memo(AggregatedEdgeImpl)
