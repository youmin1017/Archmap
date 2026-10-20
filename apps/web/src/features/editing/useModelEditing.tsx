import { useEffect, useMemo, useState } from "react"
import { toast } from "sonner"
import { ChevronDown, Plus, UsersRound } from "lucide-react"
import type { ChangeInput, Element, ElementKind, Me, Model, Relation, View } from "@archmap/api-client"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { technologySuggestions } from "@/components/tech-icon/catalog"
import { hostIds } from "../canvas/deployment"
import { allProjects, ancestorIds, buildTree, descendantCount, type ElementTree } from "../canvas/graph"
import { kinds, kindMeta } from "../canvas/kind"
import type { PanelActions } from "../canvas/PropertyPanel"
import { useCanvasStore } from "../canvas/store"
import { ElementDialog } from "../draft/ElementDialog"
import { ElementEditor } from "../draft/ElementEditor"
import type { ElementFormValues } from "../draft/ElementFields"
import {
  childKindsOf,
  edgeEntries,
  layerEntries,
  nodeEntries,
  paneEntries,
  stackEntries,
  type DraftMenuDeps,
} from "../draft/menuEntries"
import { ownerSections, principalOwnerInputs } from "../draft/ownerPicker"
import { OwnersDialog } from "../draft/OwnersDialog"
import {
  addElementInput,
  addRelationInput,
  deleteElementInput,
  deleteRelationInput,
  deploymentInputs,
  elementPatch,
  ownershipInputs,
  relationPatch,
} from "../draft/patches"
import { useLayerRules, useUpdateView } from "../draft/queries"
import { RelationDialog, type RelationFormValues } from "../draft/RelationDialog"
import { layerViolation, onLayer, removeLayerFrom, renameLayerIn, shiftLayer, violationMessage } from "../layers/layers"
import { useDirectory } from "../users/queries"
import { LayerNameDialog } from "../views/LayerNameDialog"

type ElementDialogState = { kind?: ElementKind; parentId?: string | null; layer?: string } | null
type RelationDialogState = { source: string; target: string } | { relation: Relation } | null

export type EditingOptions = {
  /** Where edits go: a draft (recorded, merged later) or main (each save is a version). */
  target: "draft" | "main"
  /** The model being edited (the draft's effective model, or main). */
  model: Model | undefined
  /** What the canvas shows (a view's subset, possibly with removed-element ghosts). */
  elements: readonly Element[]
  relations: readonly Relation[]
  /** Layer names of the active layers view. */
  viewLayers?: readonly string[]
  activeView?: View
  me: Me | undefined
  editable: boolean
  canProtect: boolean
  /** Elements shown as removed by the diff overlay cannot be edited. */
  isGhost?: (id: string) => boolean
  /** Records one batch (one undo step / one version). */
  submit: (inputs: ChangeInput[], onSuccess?: () => void) => void
  pending: boolean
  /** Called when the in-place editor opens (e.g. to show the details tab). */
  onEditStart?: () => void
}

const notGhost = () => false

/**
 * Element / relation / owner / layer editing shared by the draft page and, under
 * the "direct" review policy, by main (ADR-072): dialogs, the in-place editor,
 * right-click menus and the Add menu. Only `submit` differs between the two.
 */
export function useModelEditing({
  target,
  model,
  elements,
  relations,
  viewLayers,
  activeView,
  me,
  editable,
  canProtect,
  isGhost = notGhost,
  submit,
  pending,
  onEditStart,
}: EditingOptions) {
  const directory = useDirectory()
  const updateView = useUpdateView()
  const layerRules = useLayerRules().data ?? []

  const [elementDialog, setElementDialog] = useState<ElementDialogState>(null)
  /** Element whose details are being edited in place (Edit / Save / Cancel in the panel). */
  const [editingId, setEditingId] = useState<string | null>(null)
  const [relationDialog, setRelationDialog] = useState<RelationDialogState>(null)
  const [deleteTarget, setDeleteTarget] = useState<Element | null>(null)
  const [layerDialog, setLayerDialog] = useState<{ renaming?: string } | null>(null)
  const [layerDelete, setLayerDelete] = useState<string | null>(null)
  const [ownersOpen, setOwnersOpen] = useState(false)

  const select = useCanvasStore((s) => s.select)
  const selectedId = useCanvasStore((s) => s.selectedId)
  const reveal = useCanvasStore((s) => s.reveal)

  // The edited model's own tree (no ghosts): what dialogs validate and offer.
  const liveTree = useMemo(() => buildTree(model?.elements ?? []), [model])
  const technologyOptions = useMemo(
    () => technologySuggestions((model?.elements ?? []).map((e) => e.technology)),
    [model]
  )
  const projectOptions = useMemo(() => allProjects(model?.elements ?? []), [model])
  const layerOptions = useMemo(
    () => [...new Set([...(viewLayers ?? []), ...(model?.elements ?? []).map((e) => e.layer).filter(Boolean)])],
    [viewLayers, model]
  )

  // Leaving the element (or editing becoming impossible) ends the in-place edit.
  useEffect(() => {
    setEditingId((cur) => (editable && cur === selectedId ? cur : null))
  }, [selectedId, editable])

  // The model's own relations (not a view's subset): what the deployment diff must compare against.
  const modelRelations = model?.relations ?? []
  const owners = model?.owners ?? []
  const ownOwnerships = (elementId: string) => (model?.ownerships ?? []).filter((o) => o.element_id === elementId)
  // Users and groups picked for the first time get their owner entity in the same batch.
  const newOwnerInputs = (values: ElementFormValues) =>
    principalOwnerInputs(Object.values(values.ownership), owners, ownerSections(owners, directory.data))

  /** Saves the in-place editor: the element's changed fields and its deployment, as one batch. */
  function saveElement(existing: Element, values: ElementFormValues) {
    const inputs = [
      elementPatch(existing, values, canProtect),
      ...deploymentInputs(existing.id, modelRelations, values.deployedOn),
      ...newOwnerInputs(values),
      ...ownershipInputs(existing.id, ownOwnerships(existing.id), values.ownership),
    ].filter((i) => i !== null)
    setEditingId(null)
    if (inputs.length > 0) submit(inputs)
  }

  function startEditing(e: Element) {
    if (isGhost(e.id)) return
    select(e.id)
    onEditStart?.()
    setEditingId(e.id)
  }

  function submitElement(values: ElementFormValues) {
    if (!elementDialog) return
    const newId = crypto.randomUUID()
    submit([
      addElementInput(newId, values, canProtect),
      ...deploymentInputs(newId, [], values.deployedOn),
      ...newOwnerInputs(values),
      ...ownershipInputs(newId, [], values.ownership),
    ])
    const parentChain = values.parentId ? [values.parentId, ...ancestorIds(liveTree, values.parentId)] : []
    reveal(newId, parentChain)
    setElementDialog(null)
  }

  function submitRelation(values: RelationFormValues) {
    const t = relationDialog
    if (!t) return
    setRelationDialog(null)
    if ("relation" in t) {
      const input = relationPatch(t.relation, values)
      if (input) submit([input])
      return
    }
    submit([addRelationInput(crypto.randomUUID(), t.source, t.target, values)])
  }

  function startConnect(source: string, targetId: string) {
    if (!editable || isGhost(source) || isGhost(targetId)) return
    setRelationDialog({ source, target: targetId })
  }

  function requestDelete(elId: string) {
    const el = liveTree.byId.get(elId)
    if (editable && el && !isGhost(elId)) setDeleteTarget(el)
  }

  const relationEnds = (r: Relation) => ({
    source: liveTree.byId.get(r.source_id)?.name ?? "unknown",
    target: liveTree.byId.get(r.target_id)?.name ?? "unknown",
  })

  // Layers belong to the view (not the model), so only whoever may edit the view can change them.
  const mayEditView =
    activeView !== undefined &&
    viewLayers !== undefined &&
    (me?.role === "maintainer" || (me?.role === "editor" && activeView.created_by === me.id))

  function setLayer(elId: string, layer: string) {
    const el = liveTree.byId.get(elId)
    if (!editable || !el || isGhost(elId) || el.layer === layer) return
    const violated = layerViolation(layerRules, el, layer)
    if (violated) {
      toast.error(violationMessage(violated))
      return
    }
    submit([{ op: "update", target_type: "element", target_id: elId, patch: { layer } }])
  }

  function saveLayers(next: string[], onDone?: () => void) {
    if (!activeView) return
    updateView.mutate(
      {
        id: activeView.id,
        body: {
          name: activeView.name,
          kind: "layers",
          description: activeView.description,
          scope_element_id: null,
          layers: next,
        },
      },
      { onSuccess: onDone }
    )
  }

  /** Rename keeps the elements on the layer: they get an update, then the view follows. */
  function renameLayer(from: string, to: string) {
    if (!viewLayers) return
    const next = renameLayerIn(viewLayers, from, to)
    const moved = onLayer(elements, from).filter((e) => !isGhost(e.id) && e.layer !== to)
    const done = () => setLayerDialog(null)
    if (moved.length === 0) {
      saveLayers(next, done)
      return
    }
    submit(
      moved.map((e) => ({
        op: "update" as const,
        target_type: "element" as const,
        target_id: e.id,
        patch: { layer: to },
      })),
      () => saveLayers(next, done)
    )
  }

  const layerOps: DraftMenuDeps["layerOps"] =
    editable && mayEditView
      ? {
          add: () => setLayerDialog({}),
          rename: (name) => setLayerDialog({ renaming: name }),
          move: (name, delta) => saveLayers(shiftLayer(viewLayers, name, delta)),
          remove: (name) => setLayerDelete(name),
        }
      : undefined

  // Right-click menus (only while editing): wired to the same dialogs and mutations as the toolbar.
  const menuDeps: DraftMenuDeps = {
    element: (elId) => liveTree.byId.get(elId),
    relation: (elId) => relations.find((r) => r.id === elId),
    nameOf: (elId) => liveTree.byId.get(elId)?.name ?? "unknown",
    isGhost,
    addElement: (kind, parentId, layer) => setElementDialog({ kind, parentId, layer }),
    layers: viewLayers,
    layerRules,
    setLayer,
    layerOps,
    editElement: startEditing,
    deleteElement: requestDelete,
    startConnect: (elId) => useCanvasStore.getState().setConnectFrom(elId),
    editRelation: (r) => setRelationDialog({ relation: r }),
    deleteRelation: (r) => submit([deleteRelationInput(r.id)]),
  }
  const menu = editable
    ? {
        node: (elId: string) => nodeEntries(menuDeps, elId),
        edge: (ids: readonly string[]) => edgeEntries(menuDeps, ids),
        pane: () => paneEntries(menuDeps),
        layer: (name: string, unassigned: boolean) => layerEntries(menuDeps, name, unassigned),
        stack: () => stackEntries(menuDeps),
      }
    : undefined

  const panelActions: PanelActions | undefined = editable
    ? {
        editingId,
        onEdit: startEditing,
        renderEditor: (e) => (
          <ElementEditor
            key={e.id}
            element={e}
            tree={liveTree}
            deployedOn={hostIds(modelRelations, e.id)}
            ownerships={ownOwnerships(e.id)}
            owners={owners}
            layerOptions={layerOptions}
            projectOptions={projectOptions}
            technologyOptions={technologyOptions}
            layerRules={layerRules}
            canProtect={canProtect}
            pending={pending}
            onSave={(values) => saveElement(e, values)}
            onCancel={() => setEditingId(null)}
          />
        ),
        onDelete: (e) => requestDelete(e.id),
        onAddChild: (e) => !isGhost(e.id) && setElementDialog({ parentId: e.id, kind: childKindsOf(e.kind)[0] }),
        onEditRelation: (r) => setRelationDialog({ relation: r }),
        onDeleteRelation: (r) => submit([deleteRelationInput(r.id)]),
      }
    : undefined

  /** The "Add" menu of the page's toolbar: element kinds and the owners dialog. */
  const addMenu = editable ? (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button size="sm" variant="outline" />}>
        <Plus aria-hidden /> <span className="hidden lg:inline">Add</span>
        <ChevronDown aria-hidden className="text-muted-foreground" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Add element</DropdownMenuLabel>
          {kinds.map((k) => {
            const Icon = kindMeta[k].icon
            return (
              <DropdownMenuItem
                key={k}
                onClick={() => {
                  const sel = useCanvasStore.getState().selectedId
                  setElementDialog({ kind: k, parentId: sel && !isGhost(sel) ? sel : null })
                }}
              >
                <Icon aria-hidden style={{ color: `var(--kind-${k})` }} /> {kindMeta[k].label}
              </DropdownMenuItem>
            )
          })}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={() => setOwnersOpen(true)}>
          <UsersRound aria-hidden /> Owners…
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  ) : null

  const dialogs = (
    <>
      <ElementDialog
        open={elementDialog !== null}
        onOpenChange={(o) => !o && setElementDialog(null)}
        tree={liveTree}
        defaultKind={elementDialog?.kind}
        defaultParentId={elementDialog?.parentId}
        defaultLayer={elementDialog?.layer}
        owners={owners}
        layerOptions={layerOptions}
        projectOptions={projectOptions}
        technologyOptions={technologyOptions}
        layerRules={layerRules}
        canProtect={canProtect}
        pending={pending}
        onSubmit={submitElement}
      />

      <OwnersDialog
        open={ownersOpen}
        onOpenChange={setOwnersOpen}
        owners={owners}
        ownerships={model?.ownerships ?? []}
        pending={pending}
        onSubmit={(inputs) => submit(inputs)}
      />

      <RelationDialog
        open={relationDialog !== null}
        onOpenChange={(o) => !o && setRelationDialog(null)}
        sourceName={
          relationDialog === null
            ? ""
            : "relation" in relationDialog
              ? relationEnds(relationDialog.relation).source
              : (liveTree.byId.get(relationDialog.source)?.name ?? "")
        }
        targetName={
          relationDialog === null
            ? ""
            : "relation" in relationDialog
              ? relationEnds(relationDialog.relation).target
              : (liveTree.byId.get(relationDialog.target)?.name ?? "")
        }
        relation={relationDialog && "relation" in relationDialog ? relationDialog.relation : undefined}
        pending={pending}
        onSubmit={submitRelation}
      />

      {viewLayers && (
        <LayerNameDialog
          open={layerDialog !== null}
          onOpenChange={(o) => !o && setLayerDialog(null)}
          layers={viewLayers}
          renaming={layerDialog?.renaming}
          pending={pending || updateView.isPending}
          onSubmit={(name) =>
            layerDialog?.renaming
              ? renameLayer(layerDialog.renaming, name)
              : saveLayers([...viewLayers, name], () => setLayerDialog(null))
          }
        />
      )}

      <AlertDialog open={layerDelete !== null} onOpenChange={(o) => !o && setLayerDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete layer {layerDelete}?</AlertDialogTitle>
            <AlertDialogDescription>
              {layerDelete && onLayer(elements, layerDelete).length > 0
                ? `${onLayer(elements, layerDelete).length} element(s) keep their layer name but are shown as Unassigned in this view until they are moved. The model is not changed.`
                : "The layer is empty. The model is not changed."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (viewLayers && layerDelete) saveLayers(removeLayerFrom(viewLayers, layerDelete))
                setLayerDelete(null)
              }}
            >
              Delete layer
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={deleteTarget !== null} onOpenChange={(o) => !o && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {deleteTarget?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget && (
                <DeleteSummary element={deleteTarget} tree={liveTree} relations={modelRelations} target={target} />
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (!deleteTarget) return
                submit([deleteElementInput(deleteTarget.id, descendantCount(liveTree, deleteTarget.id) > 0)])
                select(null)
                setDeleteTarget(null)
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )

  return {
    liveTree,
    menu,
    panelActions,
    addMenu,
    dialogs,
    onConnect: startConnect,
    onDeleteRequest: requestDelete,
    onMoveToLayer: editable ? setLayer : undefined,
  }
}

function DeleteSummary({
  element,
  tree,
  relations,
  target,
}: {
  element: Element
  tree: ElementTree
  relations: readonly Relation[]
  target: "draft" | "main"
}) {
  const descendants = descendantCount(tree, element.id)
  const touching = relations.filter((r) => r.source_id === element.id || r.target_id === element.id).length
  return (
    <>
      {target === "draft"
        ? "This removes it from the draft only; main is unchanged until the draft is merged."
        : "This removes it from main as a new version."}
      {descendants > 0 && (
        <>
          {" "}
          It also deletes its <b>{descendants}</b> nested element{descendants === 1 ? "" : "s"}.
        </>
      )}
      {touching > 0 && (
        <>
          {" "}
          <b>{touching}</b> relation{touching === 1 ? "" : "s"} touching it will be deleted too.
        </>
      )}{" "}
      You can undo this.
    </>
  )
}
