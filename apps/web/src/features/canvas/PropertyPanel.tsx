import { useMemo } from "react"
import { ArrowRight, FolderPlus, Lock, MousePointerClick, Pencil, Server, Trash2, TriangleAlert } from "lucide-react"
import type { Element, Relation } from "@archmap/api-client"

import { Button } from "@/components/ui/button"

import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { Textarea } from "@/components/ui/textarea"
import { TechIcon } from "@/components/tech-icon/tech-icon"
import { canBeDeployed, hostedIds, hostIds } from "./deployment"
import { ancestorIds, type ElementTree } from "./graph"
import { kindMeta } from "./kind"
import { useCanvasStore } from "./store"

/** Draft-only actions; omitted on main (read-only). */
export type PanelActions = {
  /** Element being edited in place (its fields turn into inputs), if any. */
  editingId: string | null
  onEdit: (e: Element) => void
  /** The in-place editor for an element (Save / Cancel included). */
  renderEditor: (e: Element) => React.ReactNode
  onDelete: (e: Element) => void
  onAddChild: (e: Element) => void
  onEditRelation: (r: Relation) => void
  onDeleteRelation: (r: Relation) => void
}

function Field({ id, label, children }: { id: string; label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  )
}

/**
 * Details of the selected element. Read-only on main; in a draft the Edit button
 * swaps the fields for the editor (`actions.renderEditor`) in place.
 */
export function PropertyPanel({
  tree,
  relations,
  actions,
}: {
  tree: ElementTree
  relations: readonly Relation[]
  actions?: PanelActions
}) {
  const selectedId = useCanvasStore((s) => s.selectedId)
  const reveal = useCanvasStore((s) => s.reveal)
  const selectedRelationIds = useCanvasStore((s) => s.selectedRelationIds)
  const element = selectedId ? tree.byId.get(selectedId) : undefined

  const related = useMemo(() => {
    if (!element) return { outgoing: [] as Relation[], incoming: [] as Relation[] }
    return {
      outgoing: relations.filter((r) => r.source_id === element.id),
      incoming: relations.filter((r) => r.target_id === element.id),
    }
  }, [element, relations])

  if (!element && selectedRelationIds.length > 0) {
    const selected = relations.filter((r) => selectedRelationIds.includes(r.id))
    const nameOf = (id: string) => tree.byId.get(id)?.name ?? "unknown"
    return (
      <div className="flex flex-col gap-3 p-4" aria-label="Relation details">
        <h2 className="font-heading text-base font-semibold">
          {selected.length === 1 ? "Relation" : `${selected.length} relations`}
        </h2>
        <ul className="flex flex-col gap-2">
          {selected.map((r) => (
            <li key={r.id} className="flex flex-col gap-1 rounded-xl border p-2.5 text-sm">
              <span className="flex items-center gap-1.5">
                <span className="truncate font-medium">{nameOf(r.source_id)}</span>
                <ArrowRight className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
                <span className="truncate font-medium">{nameOf(r.target_id)}</span>
              </span>
              <span className="flex flex-wrap items-center gap-1.5">
                <Badge variant="outline">{r.kind === "deployed_on" ? "deployed on" : "uses"}</Badge>
                {r.technology && <Badge variant="secondary">{r.technology}</Badge>}
              </span>
              {r.description && <span className="text-xs text-muted-foreground">{r.description}</span>}
              {actions && (
                <span className="mt-1 flex gap-1.5">
                  <Button size="xs" variant="outline" onClick={() => actions.onEditRelation(r)}>
                    <Pencil aria-hidden /> Edit
                  </Button>
                  <Button size="xs" variant="outline" onClick={() => actions.onDeleteRelation(r)}>
                    <Trash2 aria-hidden /> Delete
                  </Button>
                </span>
              )}
            </li>
          ))}
        </ul>
      </div>
    )
  }

  if (!element) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center text-sm text-muted-foreground">
        <MousePointerClick className="size-6" aria-hidden />
        Select an element on the canvas or in the tree to see its details.
      </div>
    )
  }

  const { icon: KindIcon, label } = kindMeta[element.kind]
  const path = ancestorIds(tree, element.id)
    .reverse()
    .map((id) => tree.byId.get(id)?.name)
    .filter(Boolean)
    .join(" / ")
  const nameOf = (id: string) => tree.byId.get(id)?.name ?? "unknown"
  const goto = (id: string) => reveal(id, ancestorIds(tree, id))
  const editing = actions?.editingId === element.id
  const deployedOn = hostIds(relations, element.id)
  const hosted = element.kind === "node" ? hostedIds(relations, element.id) : []

  return (
    <div className="flex flex-col gap-4 p-4" aria-label="Element details">
      <header className="flex items-center gap-2">
        <KindIcon className="size-5" style={{ color: `var(--kind-${element.kind})` }} aria-hidden />
        <h2 className="truncate font-heading text-base font-semibold">{element.name}</h2>
        <Badge variant="secondary" className="ml-auto">
          {label}
        </Badge>
        {element.protected && (
          <Badge variant="outline">
            <Lock data-icon="inline-start" aria-hidden /> Protected
          </Badge>
        )}
      </header>
      {path && <p className="-mt-2 truncate text-xs text-muted-foreground">in {path}</p>}
      {actions && !editing && (
        <div className="flex flex-wrap gap-1.5">
          <Button size="sm" variant="outline" onClick={() => actions.onEdit(element)}>
            <Pencil aria-hidden /> Edit
          </Button>
          <Button size="sm" variant="outline" onClick={() => actions.onAddChild(element)}>
            <FolderPlus aria-hidden /> Add child
          </Button>
          <Button size="sm" variant="outline" onClick={() => actions.onDelete(element)}>
            <Trash2 aria-hidden /> Delete
          </Button>
        </div>
      )}

      {editing ? (
        actions.renderEditor(element)
      ) : (
        <>
          <Field id="prop-name" label="Name">
            <Input id="prop-name" value={element.name} readOnly />
          </Field>
          <Field id="prop-description" label="Description">
            <Textarea
              id="prop-description"
              value={element.description}
              readOnly
              rows={3}
              placeholder="No description"
            />
          </Field>
          <Field id="prop-technology" label="Technology">
            <div className="relative">
              {element.technology && (
                <TechIcon
                  technology={element.technology}
                  className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
                />
              )}
              <Input
                id="prop-technology"
                value={element.technology}
                readOnly
                className={element.technology ? "pl-8" : undefined}
                placeholder="None"
              />
            </div>
          </Field>
          <Field id="prop-layer" label="Layer">
            <Input id="prop-layer" value={element.layer} readOnly placeholder="Unassigned" />
          </Field>
          {canBeDeployed(element.kind) && (
            <div className="flex flex-col gap-1.5">
              <span className="text-sm leading-none font-medium">Deployed on</span>
              {deployedOn.length === 0 ? (
                <span className="flex items-center gap-1.5 text-sm text-amber-600 dark:text-amber-400">
                  <TriangleAlert className="size-3.5" aria-hidden /> Not deployed on any node
                </span>
              ) : (
                <ul className="flex flex-col gap-1">
                  {deployedOn.map((id) => (
                    <li key={id}>
                      <NodeLink name={nameOf(id)} onClick={() => goto(id)} />
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
          {element.replicas > 1 && (
            <Field id="prop-replicas" label="Replicas">
              <Input id="prop-replicas" value={String(element.replicas)} readOnly />
            </Field>
          )}
          {Object.keys(element.properties).length > 0 && (
            <div className="flex flex-col gap-1.5">
              <span className="text-sm leading-none font-medium">Properties</span>
              <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
                {Object.entries(element.properties).map(([k, v]) => (
                  <div key={k} className="contents">
                    <dt className="text-muted-foreground">{k}</dt>
                    <dd className="font-mono break-all">{v}</dd>
                  </div>
                ))}
              </dl>
            </div>
          )}
          {element.kind === "node" && (
            <div className="flex flex-col gap-1.5">
              <span className="text-sm leading-none font-medium">
                Hosts <span className="text-muted-foreground">({hosted.length})</span>
              </span>
              {hosted.length === 0 ? (
                <span className="text-sm text-muted-foreground">Nothing is deployed on this node</span>
              ) : (
                <ul className="flex flex-col gap-1">
                  {hosted.map((id) => (
                    <li key={id}>
                      <NodeLink name={nameOf(id)} onClick={() => goto(id)} />
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
          <div className="flex flex-col gap-1.5">
            <span className="text-sm leading-none font-medium">Projects</span>
            <div className="flex flex-wrap gap-1">
              {element.projects.length === 0 && <span className="text-sm text-muted-foreground">No projects</span>}
              {element.projects.map((p) => (
                <Badge key={p} variant="secondary">
                  {p}
                </Badge>
              ))}
            </div>
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-sm leading-none font-medium">Tags</span>
            <div className="flex flex-wrap gap-1">
              {element.tags.length === 0 && <span className="text-sm text-muted-foreground">No tags</span>}
              {element.tags.map((t) => (
                <Badge key={t} variant="outline">
                  {t}
                </Badge>
              ))}
            </div>
          </div>
        </>
      )}

      <Separator />

      <RelationList
        title="Outgoing"
        relations={related.outgoing}
        otherEnd={(r) => r.target_id}
        nameOf={nameOf}
        goto={goto}
      />
      <RelationList
        title="Incoming"
        relations={related.incoming}
        otherEnd={(r) => r.source_id}
        nameOf={nameOf}
        goto={goto}
      />

      <Separator />
      <p className="text-xs text-muted-foreground">
        Version {element.version} · updated {new Date(element.updated_at).toLocaleString()}
      </p>
    </div>
  )
}

function RelationList({
  title,
  relations,
  otherEnd,
  nameOf,
  goto,
  actions,
}: {
  title: string
  relations: Relation[]
  otherEnd: (r: Relation) => string
  nameOf: (id: string) => string
  goto: (id: string) => void
  actions?: PanelActions
}) {
  return (
    <section aria-label={`${title} relations`} className="flex flex-col gap-1.5">
      <h3 className="text-sm font-medium">
        {title} <span className="text-muted-foreground">({relations.length})</span>
      </h3>
      {relations.length === 0 && <p className="text-sm text-muted-foreground">None</p>}
      <ul className="flex flex-col gap-1">
        {relations.map((r) => (
          <li key={r.id} className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => goto(otherEnd(r))}
              className="flex min-w-0 flex-1 items-center gap-1.5 rounded-lg px-2 py-1 text-left text-sm hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            >
              <ArrowRight className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
              <span className="truncate">{nameOf(otherEnd(r))}</span>
              <Badge variant="outline" className="ml-auto">
                {r.kind === "deployed_on" ? "deployed on" : r.technology || "uses"}
              </Badge>
            </button>
            {actions && (
              <>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  aria-label={`Edit relation to ${nameOf(otherEnd(r))}`}
                  onClick={() => actions.onEditRelation(r)}
                >
                  <Pencil aria-hidden />
                </Button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  aria-label={`Delete relation to ${nameOf(otherEnd(r))}`}
                  onClick={() => actions.onDeleteRelation(r)}
                >
                  <Trash2 aria-hidden />
                </Button>
              </>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}

function NodeLink({ name, onClick }: { name: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-1.5 rounded-lg px-2 py-1 text-left text-sm hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      <Server className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
      <span className="truncate">{name}</span>
    </button>
  )
}
