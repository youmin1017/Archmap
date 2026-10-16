import { ArrowDown, ArrowUp, Cable, FolderPlus, Layers, Pencil, Plus, Trash2 } from "lucide-react"
import type { Element, ElementKind, LayerRule, Relation } from "@archmap/api-client"

import type { MenuEntry } from "../canvas/CanvasContextMenu"
import { kindMeta, kinds } from "../canvas/kind"
import { kindAllowedOnLayer, layerAllowed } from "../layers/layers"
import { allowedParentKinds, requiresParent } from "./kindRules"

/** What the draft editing menus can do; DraftPage wires these to its dialogs and mutations. */
export type DraftMenuDeps = {
  element: (id: string) => Element | undefined
  relation: (id: string) => Relation | undefined
  nameOf: (id: string) => string
  /** Elements the draft removed are drawn as ghosts and cannot be edited. */
  isGhost: (id: string) => boolean
  /** `layer` preselects the layer of the new element (layered views). */
  addElement: (kind: ElementKind, parentId: string | null, layer?: string) => void
  /** Layer names of the active layers view (top to bottom); enables "Move to layer". */
  layers?: readonly string[]
  /** Layer rules: disallowed layers are disabled in menus. */
  layerRules?: readonly LayerRule[]
  setLayer: (id: string, layer: string) => void
  /** Managing the layers themselves (create / rename / reorder / delete); only for views the user may edit. */
  layerOps?: {
    add: () => void
    rename: (name: string) => void
    move: (name: string, delta: -1 | 1) => void
    remove: (name: string) => void
  }
  editElement: (e: Element) => void
  deleteElement: (id: string) => void
  startConnect: (id: string) => void
  editRelation: (r: Relation) => void
  deleteRelation: (r: Relation) => void
}

const kindEntries = (
  candidates: readonly ElementKind[],
  pick: (k: ElementKind) => void,
  allowed: (k: ElementKind) => boolean = () => true
): MenuEntry[] =>
  candidates.map((k) => ({
    label: kindMeta[k].label,
    icon: kindMeta[k].icon,
    disabled: !allowed(k),
    onSelect: () => pick(k),
  }))

/** Kinds that may be created directly under a parent of the given kind. */
export const childKindsOf = (parent: ElementKind): ElementKind[] =>
  kinds.filter((k) => allowedParentKinds[k].includes(parent))

/** Kinds that may exist at the top level. */
export const rootKinds = (): ElementKind[] => kinds.filter((k) => !requiresParent(k))

export function nodeEntries(deps: DraftMenuDeps, id: string): MenuEntry[] {
  const el = deps.element(id)
  if (!el || deps.isGhost(id)) return []
  const childKinds = childKindsOf(el.kind)
  return [
    { label: "Edit…", icon: Pencil, onSelect: () => deps.editElement(el) },
    {
      type: "submenu",
      label: "Add child",
      icon: FolderPlus,
      disabled: childKinds.length === 0,
      entries: kindEntries(childKinds, (k) => deps.addElement(k, id)),
    },
    ...(deps.layers ? [moveToLayer(deps, el, deps.layers)] : []),
    { label: "Connect to…", icon: Cable, onSelect: () => deps.startConnect(id) },
    { type: "separator" },
    { label: "Delete…", icon: Trash2, destructive: true, onSelect: () => deps.deleteElement(id) },
  ]
}

export function edgeEntries(deps: DraftMenuDeps, relationIds: readonly string[]): MenuEntry[] {
  const rels = relationIds.map((id) => deps.relation(id)).filter((r): r is Relation => r !== undefined)
  const editable = rels.filter((r) => !deps.isGhost(r.source_id) && !deps.isGhost(r.target_id))
  const describe = (r: Relation) =>
    `${deps.nameOf(r.source_id)} → ${deps.nameOf(r.target_id)}${r.technology ? ` · ${r.technology}` : ""}`
  const perRelation = (r: Relation): MenuEntry[] => [
    { label: "Edit relation…", icon: Pencil, onSelect: () => deps.editRelation(r) },
    { label: "Delete relation", icon: Trash2, destructive: true, onSelect: () => deps.deleteRelation(r) },
  ]

  if (editable.length === 0) return []
  if (editable.length === 1) return perRelation(editable[0])
  // An aggregated edge stands for several relations: pick one first.
  return editable.map((r) => ({ type: "submenu", label: describe(r), entries: perRelation(r) }))
}

export function paneEntries(deps: Pick<DraftMenuDeps, "addElement">): MenuEntry[] {
  return [
    {
      type: "submenu",
      label: "Add element",
      icon: Plus,
      entries: kindEntries(rootKinds(), (k) => deps.addElement(k, null)),
    },
  ]
}

/** "Move to layer" submenu: every layer of the view plus Unassigned; the current one is disabled. */
function moveToLayer(deps: DraftMenuDeps, el: Element, layers: readonly string[]): MenuEntry {
  const current = el.layer.trim().toLowerCase()
  const choices = [...layers.map((l) => ({ label: l, value: l })), { label: "Unassigned", value: "" }]
  return {
    type: "submenu",
    label: "Move to layer",
    icon: Layers,
    entries: choices.map((c) => ({
      label: c.label,
      disabled: c.value.toLowerCase() === current || !layerAllowed(deps.layerRules ?? [], el, c.value),
      onSelect: () => deps.setLayer(el.id, c.value),
    })),
  }
}

/** Right-click on a layer plane: add an element into it, and manage the layer itself. */
export function layerEntries(
  deps: Pick<DraftMenuDeps, "addElement" | "layers" | "layerOps" | "layerRules">,
  layer: string,
  unassigned: boolean
): MenuEntry[] {
  const entries: MenuEntry[] = [
    {
      type: "submenu",
      label: unassigned ? "Add element" : `Add element to ${layer}`,
      icon: Plus,
      entries: kindEntries(
        kinds,
        (k) => deps.addElement(k, null, unassigned ? "" : layer),
        (k) => kindAllowedOnLayer(deps.layerRules ?? [], k, unassigned ? "" : layer)
      ),
    },
  ]
  const ops = deps.layerOps
  if (!ops) return entries
  entries.push({ type: "separator" }, { label: "New layer…", icon: Layers, onSelect: ops.add })
  if (!unassigned) {
    const index = (deps.layers ?? []).findIndex((l) => l.toLowerCase() === layer.toLowerCase())
    const last = (deps.layers?.length ?? 0) - 1
    entries.push(
      { label: "Rename layer…", icon: Pencil, onSelect: () => ops.rename(layer) },
      { label: "Move up", icon: ArrowUp, disabled: index <= 0, onSelect: () => ops.move(layer, -1) },
      { label: "Move down", icon: ArrowDown, disabled: index < 0 || index >= last, onSelect: () => ops.move(layer, 1) },
      { type: "separator" },
      { label: "Delete layer…", icon: Trash2, destructive: true, onSelect: () => ops.remove(layer) }
    )
  }
  return entries
}

/** Right-click on the empty area of a layers view. */
export function stackEntries(deps: Pick<DraftMenuDeps, "layerOps">): MenuEntry[] {
  return deps.layerOps ? [{ label: "New layer…", icon: Layers, onSelect: deps.layerOps.add }] : []
}
