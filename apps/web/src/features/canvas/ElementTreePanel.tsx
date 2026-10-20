import { useMemo, useRef, useState, type KeyboardEvent } from "react"
import { ChevronDown, ChevronRight, Focus, Search, X } from "lucide-react"
import type { Element, ElementKind } from "@archmap/api-client"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"
import { environments, lifecycles } from "./element"
import {
  allProjects,
  allTags,
  ancestorIds,
  childrenOf,
  filterMatches,
  isFilterActive,
  UNOWNED,
  usedOwners,
  type ElementTree,
} from "./graph"
import { kindMeta, kinds } from "./kind"
import { useOwnerIndex } from "./owners"
import { useCanvasStore } from "./store"

type Row = { element: Element; level: number; hasChildren: boolean; open: boolean; match: boolean }

/** Flattens the tree into the rows that are currently shown. */
export function treeRows(
  tree: ElementTree,
  open: ReadonlySet<string>,
  filter: { active: boolean; matches: ReadonlySet<string>; keep: ReadonlySet<string> }
): Row[] {
  const rows: Row[] = []
  const walk = (e: Element, level: number) => {
    if (filter.active && !filter.keep.has(e.id)) return
    const kids = childrenOf(tree, e.id).filter((c) => !filter.active || filter.keep.has(c.id))
    // While filtering, every kept ancestor is open so matches are visible.
    const isOpen = filter.active ? kids.length > 0 : open.has(e.id)
    rows.push({
      element: e,
      level,
      hasChildren: kids.length > 0,
      open: isOpen,
      match: !filter.active || filter.matches.has(e.id),
    })
    if (isOpen) for (const c of kids) walk(c, level + 1)
  }
  for (const r of tree.roots) walk(r, 1)
  return rows
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <span className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">{children}</span>
}

const chip =
  "inline-flex h-6 items-center rounded-md border px-2 text-xs transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none aria-pressed:border-primary/40 aria-pressed:bg-primary/10 aria-pressed:text-foreground"

export function ElementTreePanel({
  tree,
  elements,
  inFocus,
  onFocus,
}: {
  tree: ElementTree
  elements: readonly Element[]
  /** Ids the focused canvas shows (focused subtree + context); others are muted. Undefined = not focused. */
  inFocus?: ReadonlySet<string>
  /** Focus the canvas on an element (drill-down, ADR-071). */
  onFocus?: (id: string) => void
}) {
  const filter = useCanvasStore((s) => s.filter)
  const selectedId = useCanvasStore((s) => s.selectedId)
  const reveal = useCanvasStore((s) => s.reveal)
  const setQuery = useCanvasStore((s) => s.setQuery)
  const setKinds = useCanvasStore((s) => s.setKinds)
  const setTags = useCanvasStore((s) => s.setTags)
  const setProject = useCanvasStore((s) => s.setProject)
  const clearFilter = useCanvasStore((s) => s.clearFilter)
  const setOwner = useCanvasStore((s) => s.setOwner)
  const setEnvironment = useCanvasStore((s) => s.setEnvironment)
  const setLifecycle = useCanvasStore((s) => s.setLifecycle)
  const owners = useOwnerIndex()

  const [open, setOpen] = useState<ReadonlySet<string>>(() => new Set())
  const [focusId, setFocusId] = useState<string | null>(null)
  const listRef = useRef<HTMLUListElement>(null)

  const active = isFilterActive(filter)
  const { matches, keep } = useMemo(() => filterMatches(tree, filter), [tree, filter])
  const rows = useMemo(() => treeRows(tree, open, { active, matches, keep }), [tree, open, active, matches, keep])
  const tags = useMemo(() => allTags(elements), [elements])
  const projects = useMemo(() => allProjects(elements), [elements])
  const ownerChoices = useMemo(() => {
    const used = usedOwners(elements)
    return [...owners.values()].filter((o) => used.has(o.id)).sort((a, b) => a.name.localeCompare(b.name))
  }, [elements, owners])
  const hasUnowned = elements.some((e) => e.kind !== "person" && !e.effective?.owners.some((o) => o.role === "owner"))
  const envChoices = environments.filter((env) => elements.some((e) => e.effective?.environment === env.value))
  const lifecycleChoices = lifecycles.filter(
    (l) => l.value !== "active" && elements.some((e) => e.lifecycle === l.value)
  )

  const tabbableId = rows.some((r) => r.element.id === focusId) ? focusId : (rows[0]?.element.id ?? null)

  function setOpenState(id: string, value: boolean) {
    setOpen((prev) => {
      const next = new Set(prev)
      if (value) next.add(id)
      else next.delete(id)
      return next
    })
  }

  function focusRow(id: string | undefined) {
    if (!id) return
    setFocusId(id)
    listRef.current?.querySelector<HTMLElement>(`[data-row-id="${CSS.escape(id)}"]`)?.focus()
  }

  // Keyboard model follows the WAI-ARIA tree pattern.
  function onKeyDown(e: KeyboardEvent, i: number) {
    const row = rows[i]
    switch (e.key) {
      case "ArrowDown":
        focusRow(rows[i + 1]?.element.id)
        break
      case "ArrowUp":
        focusRow(rows[i - 1]?.element.id)
        break
      case "Home":
        focusRow(rows[0]?.element.id)
        break
      case "End":
        focusRow(rows.at(-1)?.element.id)
        break
      case "ArrowRight":
        if (row.hasChildren && !row.open) setOpenState(row.element.id, true)
        else if (row.hasChildren) focusRow(rows[i + 1]?.element.id)
        break
      case "ArrowLeft":
        if (row.hasChildren && row.open && !active) setOpenState(row.element.id, false)
        else focusRow(row.element.parent_id ?? undefined)
        break
      case "Enter":
      case " ":
        reveal(row.element.id, ancestorIds(tree, row.element.id))
        break
      case "f":
        if (!onFocus || !row.hasChildren) return
        onFocus(row.element.id)
        break
      default:
        return
    }
    e.preventDefault()
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-col gap-3 border-b px-3 pt-3 pb-3">
        <div className="relative">
          <Search
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden
          />
          <Input
            type="search"
            value={filter.query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search elements…"
            aria-label="Search elements"
            className="h-8 bg-background pl-8"
          />
        </div>
        <ToggleGroup
          multiple
          value={[...filter.kinds]}
          onValueChange={(v) => setKinds(v as ElementKind[])}
          aria-label="Filter by kind"
          variant="outline"
          size="sm"
          className="w-full *:flex-1"
        >
          {kinds.map((k) => {
            const Icon = kindMeta[k].icon
            return (
              <ToggleGroupItem key={k} value={k} aria-label={kindMeta[k].label} title={kindMeta[k].label}>
                <Icon aria-hidden style={{ color: `var(--kind-${k})` }} />
              </ToggleGroupItem>
            )
          })}
        </ToggleGroup>
        {projects.length > 0 && (
          <div className="flex flex-col gap-1.5" role="group" aria-label="Highlight a project">
            <SectionLabel>Projects</SectionLabel>
            <div className="flex flex-wrap gap-1">
              {projects.map((p) => {
                const on = filter.project?.toLowerCase() === p.toLowerCase()
                return (
                  <button
                    key={p}
                    type="button"
                    aria-pressed={on}
                    onClick={() => setProject(on ? null : p)}
                    className={cn(chip, "font-medium", !on && "text-foreground hover:bg-muted")}
                  >
                    {p}
                  </button>
                )
              })}
            </div>
          </div>
        )}
        {(ownerChoices.length > 0 || hasUnowned) && (
          <ChipFilter
            label="Owner"
            options={[
              ...ownerChoices.map((o) => ({ value: o.id, label: o.name })),
              ...(hasUnowned ? [{ value: UNOWNED, label: "Unowned" }] : []),
            ]}
            value={filter.owner}
            onChange={setOwner}
          />
        )}
        {(envChoices.length > 0 || lifecycleChoices.length > 0) && (
          <div className="flex flex-wrap gap-x-4 gap-y-3">
            {envChoices.length > 0 && (
              <ChipFilter
                label="Environment"
                options={envChoices}
                value={filter.environment}
                onChange={(v) => setEnvironment(v as typeof filter.environment)}
              />
            )}
            {lifecycleChoices.length > 0 && (
              <ChipFilter
                label="Lifecycle"
                options={lifecycleChoices}
                value={filter.lifecycle}
                onChange={(v) => setLifecycle(v as typeof filter.lifecycle)}
              />
            )}
          </div>
        )}
        {tags.length > 0 && (
          <div className="flex flex-col gap-1.5" role="group" aria-label="Filter by tag">
            <SectionLabel>Tags</SectionLabel>
            <div className="flex flex-wrap gap-1">
              {tags.map((t) => {
                const on = filter.tags.includes(t)
                return (
                  <button
                    key={t}
                    type="button"
                    aria-pressed={on}
                    onClick={() => setTags(on ? filter.tags.filter((x) => x !== t) : [...filter.tags, t])}
                    className={cn(chip, !on && "text-muted-foreground hover:bg-muted hover:text-foreground")}
                  >
                    <span className="mr-0.5 opacity-50">#</span>
                    {t}
                  </button>
                )
              })}
            </div>
          </div>
        )}
        {active && (
          <Button variant="ghost" size="xs" onClick={clearFilter} className="-ml-1 self-start text-muted-foreground">
            <X aria-hidden /> Clear filters ({matches.size} match{matches.size === 1 ? "" : "es"})
          </Button>
        )}
      </div>

      <div className="flex items-center justify-between px-3 pt-3 pb-1">
        <SectionLabel>Elements</SectionLabel>
        <span className="text-[11px] text-muted-foreground tabular-nums">{elements.length}</span>
      </div>
      <ul ref={listRef} role="tree" aria-label="Model elements" className="min-h-0 flex-1 overflow-y-auto px-1.5 pb-3">
        {rows.length === 0 && (
          <li className="px-3 py-6 text-center text-sm text-muted-foreground">No elements match.</li>
        )}
        {rows.map((row, i) => {
          const { element: e } = row
          const Icon = kindMeta[e.kind].icon
          const selected = e.id === selectedId
          return (
            <li
              key={e.id}
              role="treeitem"
              aria-level={row.level}
              aria-selected={selected}
              aria-expanded={row.hasChildren ? row.open : undefined}
              data-row-id={e.id}
              tabIndex={e.id === tabbableId ? 0 : -1}
              onFocus={() => setFocusId(e.id)}
              onKeyDown={(ev) => onKeyDown(ev, i)}
              onClick={() => reveal(e.id, ancestorIds(tree, e.id))}
              style={{ paddingLeft: `${(row.level - 1) * 14 + 6}px` }}
              className={cn(
                "group/row flex h-7 cursor-pointer items-center gap-1.5 rounded-md pr-1 text-sm outline-none select-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring",
                selected && "bg-primary/10 font-medium text-foreground hover:bg-primary/15",
                (!row.match || (inFocus && !inFocus.has(e.id))) && "text-muted-foreground"
              )}
            >
              {row.hasChildren ? (
                <button
                  type="button"
                  tabIndex={-1}
                  aria-label={`${row.open ? "Collapse" : "Expand"} ${e.name} in tree`}
                  className="grid size-5 shrink-0 place-items-center rounded text-muted-foreground hover:bg-background hover:text-foreground"
                  onClick={(ev) => {
                    ev.stopPropagation()
                    setOpenState(e.id, !row.open)
                  }}
                >
                  {row.open ? (
                    <ChevronDown className="size-3.5" aria-hidden />
                  ) : (
                    <ChevronRight className="size-3.5" aria-hidden />
                  )}
                </button>
              ) : (
                <span className="size-5 shrink-0" aria-hidden />
              )}
              <Icon className="size-4 shrink-0" style={{ color: `var(--kind-${e.kind})` }} aria-hidden />
              <span className="truncate">{e.name}</span>
              {onFocus && row.hasChildren && (
                <button
                  type="button"
                  tabIndex={-1}
                  aria-label={`Focus ${e.name}`}
                  title="Focus (F)"
                  className="ml-auto grid size-5 shrink-0 place-items-center rounded text-muted-foreground opacity-0 group-hover/row:opacity-100 group-focus-visible/row:opacity-100 hover:bg-background hover:text-foreground"
                  onClick={(ev) => {
                    ev.stopPropagation()
                    onFocus(e.id)
                  }}
                >
                  <Focus className="size-3.5" aria-hidden />
                </button>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

/** One-of-many filter chips (pressing the active chip clears it). */
function ChipFilter({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: readonly { value: string; label: string }[]
  value: string | null
  onChange: (value: string | null) => void
}) {
  return (
    <div className="flex flex-col gap-1.5" role="group" aria-label={`Filter by ${label.toLowerCase()}`}>
      <SectionLabel>{label}</SectionLabel>
      <div className="flex flex-wrap gap-1">
        {options.map((o) => {
          const on = value === o.value
          return (
            <button
              key={o.value}
              type="button"
              aria-pressed={on}
              onClick={() => onChange(on ? null : o.value)}
              className={cn(chip, !on && "text-muted-foreground hover:bg-muted hover:text-foreground")}
            >
              {o.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}
