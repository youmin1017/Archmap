import { memo } from "react"
import { Handle, Position, type Node, type NodeProps } from "@xyflow/react"
import { ChevronDown, ChevronRight, CircleAlert, Lock, TriangleAlert } from "lucide-react"
import type { Element } from "@archmap/api-client"

import { TechBadge } from "@/components/tech-icon/tech-icon"
import { cn } from "@/lib/utils"
import { kindColor, kindMeta } from "./kind"
import {
  cardClass,
  DatabaseShape,
  DiffMark,
  endpointOf,
  HostBadge,
  KindTile,
  ReplicaBackdrop,
  ReplicaBadge,
  type ShapeTone,
} from "./shapes"
import type { DiffStatus, LintSeverity } from "./decor"
import { useCanvasStore } from "./store"

export type ElementNodeData = {
  element: Element
  /** Direct children (an element with children can expand). */
  childCount: number
  expanded: boolean
  /** A filter is active and this element (or a descendant) does not match. */
  dimmed: boolean
  /** Diff overlay status against the draft's base version. */
  diff?: DiffStatus
  lint?: LintSeverity
  /** Names of the nodes the element is deployed on. */
  hosts: readonly string[]
  editable: boolean
  [key: string]: unknown
}

export type ElementFlowNode = Node<ElementNodeData, "element">

function ExpandButton({ element, childCount, expanded }: { element: Element; childCount: number; expanded: boolean }) {
  const toggle = useCanvasStore((s) => s.toggle)
  const Chevron = expanded ? ChevronDown : ChevronRight
  return (
    <button
      type="button"
      // nodrag / nopan: clicking the control must not start a drag or pan.
      className="nodrag nopan inline-flex shrink-0 items-center gap-0.5 rounded px-1 py-0.5 text-[11px] text-muted-foreground tabular-nums transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
      aria-expanded={expanded}
      aria-label={`${expanded ? "Collapse" : "Expand"} ${element.name} (${childCount} ${childCount === 1 ? "child" : "children"})`}
      onClick={(e) => {
        e.stopPropagation()
        toggle(element.id)
      }}
    >
      <Chevron className="size-3" aria-hidden />
      {childCount}
    </button>
  )
}

function ElementNodeImpl({ data, selected }: NodeProps<ElementFlowNode>) {
  const { element, childCount, expanded, dimmed, diff, lint, hosts, editable } = data
  const { icon: KindIcon, label } = kindMeta[element.kind]
  const isContainer = expanded && childCount > 0
  const db = element.kind === "database"
  const tone: ShapeTone = selected ? "selected" : (diff ?? "default")
  const endpoint = endpointOf(element.properties)

  const handleClass = cn(
    "z-20 !border-0 transition-opacity",
    editable
      ? "!size-3 !bg-primary opacity-0 group-hover/node:opacity-100 hover:!opacity-100"
      : "!size-1.5 !bg-transparent"
  )

  return (
    <div className="group/node relative h-full w-full">
      <ReplicaBackdrop element={element} />
      {/* Handles sit outside the clipped card so they stay fully visible. */}
      <Handle type="target" position={Position.Left} isConnectable={editable} className={handleClass} />
      <Handle type="source" position={Position.Right} isConnectable={editable} className={handleClass} />
      <div
        role="group"
        aria-label={`${label} ${element.name}${diff ? ` (${diff})` : ""}`}
        style={{ "--kind": kindColor(element.kind) } as React.CSSProperties}
        className={cn(
          "relative h-full w-full overflow-hidden text-card-foreground duration-200",
          !db && cardClass(tone),
          // An expanded container is a quiet frame around its children.
          isContainer && "bg-muted/25 shadow-none dark:bg-card/40",
          db && diff === "removed" && "opacity-60",
          dimmed && "opacity-30"
        )}
      >
        {db && <DatabaseShape tone={tone} dashed={diff === "removed"} />}
        {lint && (
          <span
            className="absolute top-1 right-1 z-10"
            title={lint === "error" ? "Lint error" : "Lint warning"}
            role="img"
            aria-label={lint === "error" ? "Lint error" : "Lint warning"}
          >
            {lint === "error" ? (
              <CircleAlert className="size-3.5 text-destructive" />
            ) : (
              <TriangleAlert className="size-3.5 text-amber-500" />
            )}
          </span>
        )}

        {isContainer ? (
          <div className="flex h-11 items-center gap-2 border-b bg-card/60 px-3">
            <KindTile icon={KindIcon} />
            <span className="truncate font-heading text-sm font-semibold">{element.name}</span>
            {element.protected && <Lock className="size-3 shrink-0 text-muted-foreground" aria-label="Protected" />}
            <DiffMark diff={diff} />
            <TechBadge technology={element.technology} className="ml-auto" />
            <ExpandButton element={element} childCount={childCount} expanded />
          </div>
        ) : (
          <div
            className={cn("relative flex h-full flex-col justify-between gap-1 px-3 py-2.5", db && "justify-end pt-8")}
          >
            <div className="flex items-center gap-2">
              <KindTile icon={KindIcon} />
              <span className="truncate font-heading text-sm font-semibold">{element.name}</span>
              {element.protected && <Lock className="size-3 shrink-0 text-muted-foreground" aria-label="Protected" />}
              <DiffMark diff={diff} />
              {childCount > 0 && (
                <span className="ml-auto">
                  <ExpandButton element={element} childCount={childCount} expanded={false} />
                </span>
              )}
            </div>
            {!db && <p className="line-clamp-1 text-xs text-muted-foreground">{element.description || label}</p>}
            <div className="flex items-center gap-1.5 overflow-hidden">
              <TechBadge technology={element.technology} />
              <ReplicaBadge replicas={element.replicas} />
              <HostBadge hosts={hosts} />
              {endpoint && <span className="truncate font-mono text-[11px] text-muted-foreground">{endpoint}</span>}
              {element.tags.slice(0, hosts.length > 0 ? 1 : 2).map((t) => (
                <span
                  key={t}
                  className="truncate rounded bg-muted px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground"
                >
                  {t}
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export const ElementNode = memo(ElementNodeImpl)
