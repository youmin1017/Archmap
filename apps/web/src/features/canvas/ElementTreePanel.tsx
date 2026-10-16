import { useMemo, useRef, useState, type KeyboardEvent } from "react"
import { ChevronDown, ChevronRight, Search, X } from "lucide-react"
import type { Element, ElementKind } from "@archmap/api-client"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { cn } from "@/lib/utils"
import { allProjects, allTags, ancestorIds, childrenOf, filterMatches, isFilterActive, type ElementTree } from "./graph"
import { kindMeta, kinds } from "./kind"
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

export function ElementTreePanel({ tree, elements }: { tree: ElementTree; elements: readonly Element[] }) {
  const filter = useCanvasStore((s) => s.filter)
  const selectedId = useCanvasStore((s) => s.selectedId)
  const reveal = useCanvasStore((s) => s.reveal)
  const setQuery = useCanvasStore((s) => s.setQuery)
  const setKinds = useCanvasStore((s) => s.setKinds)
  const setTags = useCanvasStore((s) => s.setTags)
  const setProject = useCanvasStore((s) => s.setProject)
  const clearFilter = useCanvasStore((s) => s.clearFilter)

  const [open, setOpen] = useState<ReadonlySet<string>>(() => new Set())
  const [focusId, setFocusId] = useState<string | null>(null)
  const listRef = useRef<HTMLUListElement>(null)

  const active = isFilterActive(filter)
  const { matches, keep } = useMemo(() => filterMatches(tree, filter), [tree, filter])
  const rows = useMemo(() => treeRows(tree, open, { active, matches, keep }), [tree, open, active, matches, keep])
  const tags = useMemo(() => allTags(elements), [elements])
  const projects = useMemo(() => allProjects(elements), [elements])

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
      default:
        return
    }
    e.preventDefault()
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex flex-col gap-2 px-3 pt-3">
        <div className="relative">
          <Search
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden
          />
          <Input
            type="search"
            value={filter.query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search elements"
            aria-label="Search elements"
            className="pl-8"
          />
        </div>
        <ToggleGroup
          multiple
          value={[...filter.kinds]}
          onValueChange={(v) => setKinds(v as ElementKind[])}
          aria-label="Filter by kind"
          variant="outline"
          size="sm"
          className="flex-wrap"
        >
          {kinds.map((k) => {
            const Icon = kindMeta[k].icon
            return (
              <ToggleGroupItem key={k} value={k} aria-label={kindMeta[k].label} title={kindMeta[k].label}>
                <Icon aria-hidden />
              </ToggleGroupItem>
            )
          })}
        </ToggleGroup>
        {projects.length > 0 && (
          <div className="flex flex-col gap-1" role="group" aria-label="Highlight a project">
            <span className="text-xs font-medium text-muted-foreground">Project</span>
            <div className="flex flex-wrap gap-1">
              {projects.map((p) => {
                const on = filter.project?.toLowerCase() === p.toLowerCase()
                return (
                  <button
                    key={p}
                    type="button"
                    aria-pressed={on}
                    onClick={() => setProject(on ? null : p)}
                    className="rounded-4xl focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
                  >
                    <Badge variant={on ? "default" : "secondary"}>{p}</Badge>
                  </button>
                )
              })}
            </div>
          </div>
        )}
        {tags.length > 0 && (
          <div className="flex flex-wrap gap-1" role="group" aria-label="Filter by tag">
            {tags.map((t) => {
              const on = filter.tags.includes(t)
              return (
                <button
                  key={t}
                  type="button"
                  aria-pressed={on}
                  onClick={() => setTags(on ? filter.tags.filter((x) => x !== t) : [...filter.tags, t])}
                  className="rounded-4xl focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
                >
                  <Badge variant={on ? "default" : "outline"}>{t}</Badge>
                </button>
              )
            })}
          </div>
        )}
        {active && (
          <Button variant="ghost" size="xs" onClick={clearFilter} className="self-start">
            <X aria-hidden /> Clear filters ({matches.size} match{matches.size === 1 ? "" : "es"})
          </Button>
        )}
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
                "flex cursor-pointer items-center gap-1.5 rounded-lg py-1 pr-2 text-sm outline-none select-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring",
                selected && "bg-accent font-medium",
                !row.match && "text-muted-foreground"
              )}
            >
              {row.hasChildren ? (
                <button
                  type="button"
                  tabIndex={-1}
                  aria-label={`${row.open ? "Collapse" : "Expand"} ${e.name} in tree`}
                  className="grid size-5 shrink-0 place-items-center rounded hover:bg-muted"
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
            </li>
          )
        })}
      </ul>
    </div>
  )
}
