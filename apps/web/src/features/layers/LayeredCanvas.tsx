import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { ChevronDown, ChevronRight, CircleAlert, Eye, EyeOff, Lock, TriangleAlert } from "lucide-react"
import type { Element, LayerRule, Relation } from "@archmap/api-client"

import { Button } from "@/components/ui/button"
import { TechBadge } from "@/components/tech-icon/tech-icon"
import { cn } from "@/lib/utils"
import { CanvasContextMenu, joinGroups, type MenuEntry, type MenuPosition } from "../canvas/CanvasContextMenu"
import type { Decor } from "../canvas/decor"
import { buildTree, filterMatches, isFilterActive } from "../canvas/graph"
import { kindColor, kindMeta } from "../canvas/kind"
import { hostNameIndex } from "../canvas/deployment"
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
} from "../canvas/shapes"
import { useCanvasStore } from "../canvas/store"
import { useLayerRules, useMe, useReplaceLayerRules } from "../draft/queries"
import { LayerRulesDialog } from "./LayerRulesDialog"
import {
  groupByLayer,
  laneModes,
  layerAllowed,
  nestWithin,
  planeHue,
  ruleClassLabel,
  toggleHidden,
  type GroupNode,
  type Plane,
} from "./layers"

type Props = {
  elements: readonly Element[]
  relations: readonly Relation[]
  /** Ordered layer names, top to bottom (from the view). */
  layers: readonly string[]
  decor?: Decor
  /** Called when a "Connect to…" menu action is completed by clicking the target card. */
  onConnect?: (sourceId: string, targetId: string) => void
  /** Drag & drop of a card onto a layer; the layer is "" for Unassigned. Omit to make cards not draggable. */
  onMoveToLayer?: (id: string, layer: string) => void
  menu?: {
    node?: (id: string) => MenuEntry[]
    /** Entries for a right-click on a layer. */
    layer?: (name: string, unassigned: boolean) => MenuEntry[]
    /** Entries for a right-click on the empty area around the layers. */
    stack?: () => MenuEntry[]
  }
}

type Line = { id: string; d: string; source: string; target: string; dashed: boolean }

/**
 * Layered view as flat plans stacked top to bottom: one band per layer, the
 * first layer on top. A layer list on the left switches layers on and off and
 * focuses one ("only this": the others collapse to a header bar), so the stack
 * can be browsed layer by layer. Relations are drawn between cards, also
 * across layers, and highlighted for the selected or hovered card.
 */
export function LayeredCanvas({ elements, relations, layers, decor, onConnect, onMoveToLayer, menu }: Props) {
  const planes = useMemo(() => groupByLayer(elements, layers), [elements, layers])
  const byId = useMemo(() => new Map(elements.map((e) => [e.id, e])), [elements])
  const layerOf = useMemo(() => {
    const m = new Map<string, string>()
    for (const p of planes) for (const e of p.elements) m.set(e.id, p.name)
    return m
  }, [planes])

  const selectedId = useCanvasStore((s) => s.selectedId)
  const select = useCanvasStore((s) => s.select)
  const filter = useCanvasStore((s) => s.filter)
  const connectFrom = useCanvasStore((s) => s.connectFrom)
  const setConnectFrom = useCanvasStore((s) => s.setConnectFrom)

  const keep = useMemo(
    () => (isFilterActive(filter) ? filterMatches(buildTree(elements), filter).matches : null),
    [elements, filter]
  )

  const [hidden, setHidden] = useState<ReadonlySet<string>>(() => new Set())
  const [focus, setFocus] = useState<string | null>(null)
  const [hoverId, setHoverId] = useState<string | null>(null)
  const [dragId, setDragId] = useState<string | null>(null)
  const [dropLayer, setDropLayer] = useState<string | null>(null)
  const [rulesOpen, setRulesOpen] = useState(false)
  const rules = useLayerRules().data ?? []
  const saveRules = useReplaceLayerRules()
  const isMaintainer = useMe().data?.role === "maintainer"
  // Whether the card being dragged may go on this layer (layer rules; the API enforces them too).
  const dropAllowed = (layer: string, unassigned: boolean) => {
    const dragged = dragId ? byId.get(dragId) : undefined
    return dragged !== undefined && layerAllowed(rules, dragged, unassigned ? "" : layer)
  }
  const [lines, setLines] = useState<Line[]>([])
  const [menuPos, setMenuPos] = useState<MenuPosition | null>(null)
  const [menuEntries, setMenuEntries] = useState<MenuEntry[]>([])
  const modes = useMemo(() => laneModes(planes, hidden, focus), [planes, hidden, focus])

  const contentRef = useRef<HTMLDivElement>(null)
  const cardRefs = useRef(new Map<string, HTMLElement>())
  const laneRefs = useRef(new Map<string, HTMLElement>())

  const hosts = useMemo(() => hostNameIndex(byId, relations), [byId, relations])

  // One link per pair of elements (parallel relations collapse; dashed when only deployed_on).
  const pairs = useMemo(() => {
    const map = new Map<string, { source: string; target: string; dashed: boolean }>()
    for (const r of relations) {
      if (r.source_id === r.target_id || !byId.has(r.source_id) || !byId.has(r.target_id)) continue
      const key = `${r.source_id}>${r.target_id}`
      const cur = map.get(key)
      map.set(key, {
        source: r.source_id,
        target: r.target_id,
        dashed: (cur?.dashed ?? true) && r.kind === "deployed_on",
      })
    }
    return [...map.entries()].map(([id, v]) => ({ id, ...v }))
  }, [relations, byId])

  // Where a link attaches: the card, or its layer bar when that layer is collapsed; nothing when hidden.
  const measure = useCallback(() => {
    const root = contentRef.current
    if (!root) return
    const box = root.getBoundingClientRect()
    const anchor = (id: string) => {
      const layer = layerOf.get(id)
      if (!layer) return null
      const mode = modes.get(layer)
      const el = mode === "open" ? cardRefs.current.get(id) : mode === "collapsed" ? laneRefs.current.get(layer) : null
      if (!el) return null
      const r = el.getBoundingClientRect()
      return { x: r.x + r.width / 2 - box.x, y: r.y + r.height / 2 - box.y }
    }
    const next: Line[] = []
    for (const p of pairs) {
      const a = anchor(p.source)
      const b = anchor(p.target)
      if (!a || !b) continue
      const mid = (a.y + b.y) / 2
      const bend = Math.abs(a.y - b.y) < 8 ? 40 : 0 // same row: arc so the link is visible
      next.push({
        id: p.id,
        source: p.source,
        target: p.target,
        dashed: p.dashed,
        d: `M ${a.x} ${a.y} C ${a.x} ${mid - bend}, ${b.x} ${mid - bend}, ${b.x} ${b.y}`,
      })
    }
    setLines(next)
  }, [pairs, layerOf, modes])

  useLayoutEffect(measure, [measure, planes, hidden, focus])
  useEffect(() => {
    const root = contentRef.current
    if (!root) return
    const observer = new ResizeObserver(measure)
    observer.observe(root)
    return () => observer.disconnect()
  }, [measure])

  // Connect mode (menu "Connect to…"): Escape cancels.
  useEffect(() => {
    if (!connectFrom) return
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setConnectFrom(null)
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [connectFrom, setConnectFrom])

  const openMenu = (e: React.MouseEvent, entries: MenuEntry[]) => {
    e.preventDefault()
    e.stopPropagation()
    if (entries.length === 0) return
    setMenuEntries(entries)
    setMenuPos({ x: e.clientX, y: e.clientY })
  }

  const activate = (id: string) => {
    if (connectFrom && connectFrom !== id) {
      onConnect?.(connectFrom, id)
      setConnectFrom(null)
      return
    }
    select(id)
  }

  const focusId = hoverId ?? selectedId
  const related = useMemo(() => {
    const set = new Set<string>()
    if (focusId) {
      set.add(focusId)
      for (const p of pairs) {
        if (p.source === focusId) set.add(p.target)
        if (p.target === focusId) set.add(p.source)
      }
    }
    return set
  }, [focusId, pairs])

  const renderNode = (n: GroupNode): React.ReactNode => {
    const el = n.element
    const card = (
      <Card
        key={el.id}
        element={el}
        selected={el.id === selectedId}
        draggable={onMoveToLayer !== undefined && decor?.diff?.get(el.id) !== "removed"}
        dragging={dragId === el.id}
        onDragStart={() => setDragId(el.id)}
        onDragEnd={() => {
          setDragId(null)
          setDropLayer(null)
        }}
        dimmed={(keep !== null && !keep.has(el.id)) || (focusId !== null && !related.has(el.id))}
        diff={decor?.diff?.get(el.id)}
        lint={decor?.lint?.get(el.id)}
        hosts={hosts.get(el.id) ?? noHosts}
        register={(node) => {
          if (node) cardRefs.current.set(el.id, node)
          else cardRefs.current.delete(el.id)
        }}
        onActivate={() => activate(el.id)}
        onHover={(over) => setHoverId(over ? el.id : null)}
        onContextMenu={(e) => {
          select(el.id)
          openMenu(e, joinGroups(menu?.node?.(el.id) ?? []))
        }}
      />
    )
    if (n.children.length === 0) return card
    // An element with elements of this layer inside it (e.g. a namespace) is drawn as a group.
    return (
      <div
        key={el.id}
        className="col-span-full grid gap-3 rounded-lg border border-dashed bg-background/40 p-2"
        style={{
          gridTemplateColumns: "repeat(auto-fill, minmax(184px, 1fr))",
          ["--kind" as string]: kindColor(el.kind),
        }}
      >
        {card}
        {n.children.map(renderNode)}
      </div>
    )
  }

  const visibleCount = [...modes.values()].filter((m) => m !== "hidden").length

  return (
    <>
      <div className="flex h-full min-h-0">
        <LayerList
          planes={planes}
          modes={modes}
          focus={focus}
          onToggleVisible={(name) => setHidden((h) => toggleHidden(h, name))}
          onFocus={(name) => setFocus((f) => (f === name ? null : name))}
          rules={rules}
          onEditRules={isMaintainer ? () => setRulesOpen(true) : undefined}
          onShowAll={() => {
            setHidden(new Set())
            setFocus(null)
          }}
        />

        <div
          className="relative min-w-0 flex-1 overflow-auto bg-background"
          onClick={(e) => {
            if (!(e.target instanceof HTMLElement && e.target.closest("[data-card]"))) select(null)
          }}
          onContextMenu={(e) => openMenu(e, joinGroups(menu?.stack?.() ?? []))}
        >
          <div ref={contentRef} className="relative flex min-h-full flex-col gap-3 p-4">
            {connectFrom && (
              <div
                role="status"
                className="sticky top-0 z-20 mx-auto -mb-1 w-fit rounded-full border bg-popover px-3 py-1.5 text-sm shadow-md"
              >
                Connecting from <b>{byId.get(connectFrom)?.name}</b>: click the target element · Esc to cancel
              </div>
            )}

            {visibleCount === 0 && (
              <p className="m-auto text-sm text-muted-foreground">All layers are hidden. Switch one on in the list.</p>
            )}

            {planes.map((plane, i) => {
              const mode = modes.get(plane.name) ?? "open"
              if (mode === "hidden") return null
              return (
                <Lane
                  key={plane.name}
                  plane={plane}
                  hue={planeHue(i)}
                  collapsed={mode === "collapsed"}
                  dropping={dragId !== null && dropLayer === plane.name}
                  blocked={dragId !== null && !dropAllowed(plane.name, plane.unassigned)}
                  onDragOver={
                    onMoveToLayer && dragId && dropAllowed(plane.name, plane.unassigned)
                      ? (e) => {
                          e.preventDefault()
                          e.dataTransfer.dropEffect = "move"
                          setDropLayer(plane.name)
                        }
                      : undefined
                  }
                  onDragLeave={(e) => {
                    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setDropLayer(null)
                  }}
                  onDrop={(e) => {
                    e.preventDefault()
                    const id = dragId
                    setDragId(null)
                    setDropLayer(null)
                    if (
                      id &&
                      onMoveToLayer &&
                      layerOf.get(id) !== plane.name &&
                      dropAllowed(plane.name, plane.unassigned)
                    )
                      onMoveToLayer(id, plane.unassigned ? "" : plane.name)
                  }}
                  registerLane={(node) => {
                    if (node) laneRefs.current.set(plane.name, node)
                    else laneRefs.current.delete(plane.name)
                  }}
                  onToggleCollapse={() => setFocus(mode === "collapsed" ? plane.name : null)}
                  onContextMenu={(e) => openMenu(e, joinGroups(menu?.layer?.(plane.name, plane.unassigned) ?? []))}
                >
                  {nestWithin(plane.elements).map(renderNode)}
                </Lane>
              )
            })}

            <svg className="pointer-events-none absolute inset-0 z-10 size-full" aria-hidden>
              <defs>
                <marker
                  id="layer-arrow"
                  viewBox="0 0 10 10"
                  refX="9"
                  refY="5"
                  markerWidth="7"
                  markerHeight="7"
                  orient="auto"
                >
                  <path d="M0 0 L10 5 L0 10 z" fill="context-stroke" />
                </marker>
              </defs>
              {lines.map((l) => {
                const active = focusId !== null && (l.source === focusId || l.target === focusId)
                return (
                  <path
                    key={l.id}
                    d={l.d}
                    fill="none"
                    stroke={active ? "var(--primary)" : "var(--muted-foreground)"}
                    strokeWidth={active ? 2 : 1.25}
                    strokeDasharray={l.dashed ? "6 4" : undefined}
                    opacity={active ? 0.95 : focusId !== null ? 0.1 : 0.4}
                    markerEnd="url(#layer-arrow)"
                  />
                )
              })}
            </svg>
          </div>
        </div>
      </div>
      <LayerRulesDialog
        open={rulesOpen}
        onOpenChange={setRulesOpen}
        rules={rules}
        layers={layers}
        pending={saveRules.isPending}
        onSave={(next) => saveRules.mutate(next, { onSuccess: () => setRulesOpen(false) })}
      />
      <CanvasContextMenu position={menuPos} entries={menuEntries} onClose={() => setMenuPos(null)} />
    </>
  )
}

/** Left list: one row per layer with a visibility switch; clicking the name focuses it. */
function LayerList({
  planes,
  modes,
  focus,
  onToggleVisible,
  onFocus,
  onShowAll,
  rules,
  onEditRules,
}: {
  rules: readonly LayerRule[]
  onEditRules?: () => void
  planes: readonly Plane[]
  modes: ReadonlyMap<string, "open" | "collapsed" | "hidden">
  focus: string | null
  onToggleVisible: (name: string) => void
  onFocus: (name: string) => void
  onShowAll: () => void
}) {
  const filtered = focus !== null || [...modes.values()].some((m) => m === "hidden")
  return (
    <nav
      aria-label="Layers"
      className="flex w-52 shrink-0 flex-col gap-1 overflow-y-auto border-r bg-sidebar p-3 text-sidebar-foreground"
    >
      <div className="flex items-center justify-between px-1 pb-1">
        <h2 className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">Layers</h2>
        {filtered && (
          <Button variant="ghost" size="xs" onClick={onShowAll}>
            Show all
          </Button>
        )}
      </div>
      <ol className="flex flex-col gap-0.5">
        {planes.map((p, i) => {
          const mode = modes.get(p.name) ?? "open"
          return (
            <li key={p.name} className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={mode === "hidden" ? `Show layer ${p.name}` : `Hide layer ${p.name}`}
                aria-pressed={mode !== "hidden"}
                onClick={() => onToggleVisible(p.name)}
              >
                {mode === "hidden" ? <EyeOff aria-hidden /> : <Eye aria-hidden />}
              </Button>
              <button
                type="button"
                aria-pressed={focus === p.name}
                title={focus === p.name ? "Show all layers again" : "Show only this layer"}
                onClick={() => onFocus(p.name)}
                className={cn(
                  "flex h-7 min-w-0 flex-1 items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
                  focus === p.name && "bg-primary/10 font-medium hover:bg-primary/15",
                  mode === "hidden" && "text-muted-foreground line-through"
                )}
              >
                <span
                  className="size-2 shrink-0 rounded-full"
                  style={{ background: `hsl(${planeHue(i)} 65% 58%)` }}
                  aria-hidden
                />
                <span className="truncate">{p.name}</span>
                <span className="ml-auto text-xs text-muted-foreground tabular-nums">{p.elements.length}</span>
              </button>
            </li>
          )
        })}
      </ol>
      <p className="px-1 pt-2 text-xs text-muted-foreground">
        Click a layer to show only it. The eye switches a layer on or off.
      </p>
      <div className="mt-2 flex flex-col gap-1 border-t px-1 pt-2">
        <div className="flex items-center justify-between">
          <h2 className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">Rules</h2>
          {onEditRules && (
            <Button variant="ghost" size="xs" onClick={onEditRules}>
              Edit
            </Button>
          )}
        </div>
        {rules.length === 0 ? (
          <p className="text-xs text-muted-foreground">Any element can be placed on any layer.</p>
        ) : (
          <ul className="flex flex-col gap-1 text-xs text-muted-foreground">
            {rules.map((r) => (
              <li key={r.element_class}>
                <span className="font-medium text-foreground">{ruleClassLabel(r.element_class)}</span>:{" "}
                {r.layers.join(", ")}
              </li>
            ))}
          </ul>
        )}
      </div>
    </nav>
  )
}

function Lane({
  plane,
  hue,
  collapsed,
  dropping,
  blocked,
  onDragOver,
  onDragLeave,
  onDrop,
  registerLane,
  onToggleCollapse,
  onContextMenu,
  children,
}: {
  plane: Plane
  hue: number
  collapsed: boolean
  /** A card is being dragged over this layer. */
  dropping: boolean
  /** A card is being dragged and the layer rules forbid it here. */
  blocked: boolean
  onDragOver?: (e: React.DragEvent) => void
  onDragLeave: (e: React.DragEvent) => void
  onDrop: (e: React.DragEvent) => void
  registerLane: (node: HTMLElement | null) => void
  onToggleCollapse: () => void
  onContextMenu: (e: React.MouseEvent) => void
  children: React.ReactNode
}) {
  const Chevron = collapsed ? ChevronRight : ChevronDown
  return (
    <section
      aria-label={`Layer ${plane.name}, ${plane.elements.length} element${plane.elements.length === 1 ? "" : "s"}`}
      className={cn(
        "rounded-lg border bg-muted/30 transition-[background-color,box-shadow,opacity] dark:bg-card/40",
        dropping && "bg-primary/5 ring-2 ring-primary/60",
        blocked && "opacity-40"
      )}
      onContextMenu={onContextMenu}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      <header ref={registerLane} className="flex items-center gap-2 px-3 py-1.5">
        <button
          type="button"
          onClick={onToggleCollapse}
          aria-expanded={!collapsed}
          aria-label={collapsed ? `Show layer ${plane.name} only` : `Show all layers`}
          className="grid size-5 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          <Chevron className="size-3.5" aria-hidden />
        </button>
        <span className="size-2 shrink-0 rounded-full" style={{ background: `hsl(${hue} 65% 58%)` }} aria-hidden />
        <h3 className="text-sm font-medium">{plane.name}</h3>
        <span className="text-xs text-muted-foreground tabular-nums">{plane.elements.length}</span>
      </header>
      {!collapsed && (
        <div
          className="grid gap-3 px-3 pt-1 pb-3"
          style={{ gridTemplateColumns: "repeat(auto-fill, minmax(184px, 1fr))" }}
        >
          {plane.elements.length === 0 ? (
            <p className="text-xs text-muted-foreground">Nothing on this layer yet.</p>
          ) : (
            children
          )}
        </div>
      )}
    </section>
  )
}

const noHosts: readonly string[] = []

function Card({
  element,
  selected,
  dimmed,
  diff,
  lint,
  hosts,
  register,
  draggable,
  dragging,
  onDragStart,
  onDragEnd,
  onActivate,
  onHover,
  onContextMenu,
}: {
  element: Element
  selected: boolean
  dimmed: boolean
  diff?: "added" | "modified" | "removed"
  lint?: "error" | "warning"
  hosts: readonly string[]
  register: (node: HTMLElement | null) => void
  draggable: boolean
  dragging: boolean
  onDragStart: () => void
  onDragEnd: () => void
  onActivate: () => void
  onHover: (over: boolean) => void
  onContextMenu: (e: React.MouseEvent) => void
}) {
  const { icon: KindIcon, label } = kindMeta[element.kind]
  const db = element.kind === "database"
  const tone: ShapeTone = selected ? "selected" : (diff ?? "default")
  const endpoint = endpointOf(element.properties)
  return (
    <div className="relative" style={{ ["--kind" as string]: kindColor(element.kind) }}>
      <ReplicaBackdrop element={element} />
      <div
        ref={register}
        data-card={element.id}
        role="button"
        tabIndex={0}
        draggable={draggable}
        onDragStart={(e) => {
          e.dataTransfer.setData("text/plain", element.id)
          e.dataTransfer.effectAllowed = "move"
          onDragStart()
        }}
        onDragEnd={onDragEnd}
        aria-pressed={selected}
        aria-label={`${label} ${element.name}${diff ? ` (${diff})` : ""}`}
        className={cn(
          "relative flex flex-col justify-center gap-1.5 overflow-hidden px-3 text-card-foreground outline-none",
          db ? "h-[96px] justify-end pt-8 pb-2" : cn("h-[76px]", cardClass(tone)),
          "cursor-pointer focus-visible:ring-2 focus-visible:ring-ring",
          db && diff === "removed" && "opacity-60",
          dimmed && "opacity-30",
          draggable && "cursor-grab active:cursor-grabbing",
          dragging && "opacity-40"
        )}
        onClick={(e) => {
          e.stopPropagation()
          onActivate()
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault()
            onActivate()
          }
        }}
        onMouseEnter={() => onHover(true)}
        onMouseLeave={() => onHover(false)}
        onFocus={() => onHover(true)}
        onBlur={() => onHover(false)}
        onContextMenu={onContextMenu}
      >
        {db && <DatabaseShape tone={tone} dashed={diff === "removed"} />}
        <div className="relative flex items-center gap-2">
          <KindTile icon={KindIcon} />
          <span className="truncate font-heading text-sm font-semibold">{element.name}</span>
          {element.protected && <Lock className="size-3 shrink-0 text-muted-foreground" aria-label="Protected" />}
          <DiffMark diff={diff} />
          {lint && (
            <span className="ml-auto" role="img" aria-label={lint === "error" ? "Lint error" : "Lint warning"}>
              {lint === "error" ? (
                <CircleAlert className="size-3.5 text-destructive" />
              ) : (
                <TriangleAlert className="size-3.5 text-amber-500" />
              )}
            </span>
          )}
        </div>
        <div className="relative flex items-center gap-1.5 overflow-hidden">
          <TechBadge technology={element.technology} />
          <ReplicaBadge replicas={element.replicas} />
          <HostBadge hosts={hosts} />
          {endpoint && <span className="truncate font-mono text-[11px] text-muted-foreground">{endpoint}</span>}
          {!element.technology && !endpoint && hosts.length === 0 && (
            <span className="truncate text-xs text-muted-foreground">{element.description || label}</span>
          )}
        </div>
      </div>
    </div>
  )
}
