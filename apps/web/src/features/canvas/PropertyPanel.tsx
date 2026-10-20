import { useMemo } from "react"
import {
  ArrowRight,
  Focus,
  FolderPlus,
  Lock,
  MousePointerClick,
  Pencil,
  Server,
  Trash2,
  TriangleAlert,
} from "lucide-react"
import type { Element, Relation } from "@archmap/api-client"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { TechIcon } from "@/components/tech-icon/tech-icon"
import { canBeDeployed, hostedIds, hostIds } from "./deployment"
import {
  criticalities,
  daysSinceReview,
  effectiveOwner,
  environments,
  labelOf,
  lifecycles,
  ownerRoles,
  REVIEW_MAX_DAYS,
  typeMeta,
} from "./element"
import { useOwnerIndex } from "./owners"
import { ancestorIds, type ElementTree } from "./graph"
import { kindMeta } from "./kind"
import { useCanvasStore } from "./store"
import { useFocusAction } from "./focusAction"

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

/** One row of the read-only property list: label on the left, value on the right. */
function Prop({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="contents">
      <dt className="py-1 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 py-1">{children}</dd>
    </div>
  )
}

const none = <span className="text-muted-foreground/70">—</span>

/** Small uppercase heading of a panel section. */
function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h3 className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">{children}</h3>
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
        <h2 className="font-heading text-base font-semibold tracking-tight">
          {selected.length === 1 ? "Relation" : `${selected.length} relations`}
        </h2>
        <ul className="flex flex-col gap-2">
          {selected.map((r) => (
            <li key={r.id} className="flex flex-col gap-1.5 rounded-lg border bg-card p-2.5 text-sm">
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
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
        <span className="grid size-10 place-items-center rounded-xl border bg-card text-muted-foreground shadow-xs">
          <MousePointerClick className="size-5" aria-hidden />
        </span>
        <div className="flex flex-col gap-1">
          <p className="text-sm font-medium">Nothing selected</p>
          <p className="text-xs text-muted-foreground">
            Select an element on the canvas or in the tree to see its details.
          </p>
        </div>
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
  const owners = useOwnerIndex()
  const eff = element.effective
  const deployedVia = eff?.deployed_via_id && eff.deployed_via_id !== element.id ? eff.deployed_via_id : null
  const envSource =
    eff?.environment_source_id && eff.environment_source_id !== element.id ? eff.environment_source_id : null
  const reviewAge = daysSinceReview(element.last_reviewed_at)
  const type = typeMeta(element.type)
  const focus = useFocusAction()

  return (
    <div className="flex flex-col gap-4 p-4" aria-label="Element details">
      <header className="flex items-start gap-3">
        <span
          className="grid size-9 shrink-0 place-items-center rounded-lg border"
          style={{
            color: `var(--kind-${element.kind})`,
            background: `color-mix(in oklch, var(--kind-${element.kind}) 10%, transparent)`,
            borderColor: `color-mix(in oklch, var(--kind-${element.kind}) 25%, transparent)`,
          }}
        >
          <KindIcon className="size-4.5" aria-hidden />
        </span>
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <h2 className="truncate font-heading text-base leading-tight font-semibold tracking-tight">{element.name}</h2>
          <p className="flex items-center gap-1.5 truncate text-xs text-muted-foreground">
            {label}
            {path && <span className="truncate">· in {path}</span>}
          </p>
        </div>
        {element.protected && (
          <Badge variant="outline" className="shrink-0">
            <Lock data-icon="inline-start" aria-hidden /> Protected
          </Badge>
        )}
        {focus && tree.children.has(element.id) && (
          <Button
            size="icon-sm"
            variant="ghost"
            className="shrink-0 text-muted-foreground"
            aria-label={`Focus ${element.name}`}
            title="Focus (F)"
            onClick={() => focus(element.id)}
          >
            <Focus aria-hidden />
          </Button>
        )}
      </header>
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
          {element.description ? (
            <p className="text-sm leading-relaxed text-pretty">{element.description}</p>
          ) : (
            <p className="text-sm text-muted-foreground italic">No description</p>
          )}
          <dl className="grid grid-cols-[6.5rem_minmax(0,1fr)] border-y py-1.5 text-sm">
            <Prop label="Technology">
              {element.technology ? (
                <span className="flex items-center gap-1.5">
                  <TechIcon technology={element.technology} className="size-4 shrink-0" />
                  <span className="truncate">{element.technology}</span>
                </span>
              ) : (
                none
              )}
            </Prop>
            <Prop label="Layer">{element.layer || <span className="text-muted-foreground/70">Unassigned</span>}</Prop>
            {element.replicas > 1 && (
              <Prop label="Replicas">
                <span className="tabular-nums">×{element.replicas}</span>
              </Prop>
            )}
            <Prop label="Projects">
              {element.projects.length === 0 ? (
                none
              ) : (
                <span className="flex flex-wrap gap-1">
                  {element.projects.map((p) => (
                    <Badge key={p} variant="secondary">
                      {p}
                    </Badge>
                  ))}
                </span>
              )}
            </Prop>
            <Prop label="Tags">
              {element.tags.length === 0 ? (
                none
              ) : (
                <span className="flex flex-wrap gap-1">
                  {element.tags.map((t) => (
                    <Badge key={t} variant="outline">
                      #{t}
                    </Badge>
                  ))}
                </span>
              )}
            </Prop>
          </dl>
          <div className="flex flex-col gap-1.5">
            <SectionTitle>Operations</SectionTitle>
            <dl className="grid grid-cols-[6.5rem_minmax(0,1fr)] text-sm">
              {type && <Prop label="Type">{type.label}</Prop>}
              <Prop label="Environment">
                {eff?.environment ? (
                  <span className="flex flex-wrap items-baseline gap-x-1.5">
                    {labelOf(environments, eff.environment)}
                    {envSource && (
                      <button
                        type="button"
                        className="text-xs text-muted-foreground underline-offset-2 hover:underline"
                        onClick={() => goto(envSource)}
                      >
                        from {nameOf(envSource)}
                      </button>
                    )}
                  </span>
                ) : (
                  none
                )}
              </Prop>
              <Prop label="Lifecycle">{labelOf(lifecycles, element.lifecycle)}</Prop>
              <Prop label="Criticality">{labelOf(criticalities, element.criticality) || none}</Prop>
              <Prop label="Runbook">{element.runbook_url ? <ExternalLink href={element.runbook_url} /> : none}</Prop>
              <Prop label="Docs">{element.docs_url ? <ExternalLink href={element.docs_url} /> : none}</Prop>
              <Prop label="Reviewed">
                {element.last_reviewed_at ? (
                  <span className="flex flex-col">
                    <span className="tabular-nums">{element.last_reviewed_at}</span>
                    {reviewAge !== null && reviewAge > REVIEW_MAX_DAYS && (
                      <span className="text-xs text-amber-600 dark:text-amber-400">{reviewAge} days ago</span>
                    )}
                  </span>
                ) : (
                  <span className="text-muted-foreground/70">Never</span>
                )}
              </Prop>
            </dl>
          </div>
          {element.kind !== "person" && (
            <div className="flex flex-col gap-1.5">
              <SectionTitle>Ownership</SectionTitle>
              <dl className="grid grid-cols-[6.5rem_minmax(0,1fr)] text-sm">
                {ownerRoles.map((r) => {
                  const o = effectiveOwner(eff, element.id, r.value)
                  const owner = o ? owners.get(o.ownerId) : undefined
                  return (
                    <Prop key={r.value} label={r.label}>
                      {o ? (
                        <span className="flex flex-col">
                          <span
                            className={cn("truncate", owner?.inactive && "text-muted-foreground line-through")}
                            title={owner?.contact || undefined}
                          >
                            {owner?.name ?? "Unknown owner"}
                          </span>
                          {owner?.inactive && (
                            <span className="text-xs text-amber-600 dark:text-amber-400">
                              {owner.kind === "group" ? "Deleted group" : "Disabled user"}
                            </span>
                          )}
                          {o.inherited && (
                            <button
                              type="button"
                              className="self-start text-xs text-muted-foreground underline-offset-2 hover:underline"
                              onClick={() => goto(o.sourceId)}
                            >
                              inherited from {nameOf(o.sourceId)}
                            </button>
                          )}
                        </span>
                      ) : r.value === "owner" ? (
                        <span className="text-muted-foreground/70">Unowned</span>
                      ) : (
                        none
                      )}
                    </Prop>
                  )
                })}
              </dl>
            </div>
          )}
          {canBeDeployed(element.kind) && (
            <div className="flex flex-col gap-1.5">
              <SectionTitle>Deployed on</SectionTitle>
              {deployedOn.length === 0 && deployedVia ? (
                <span className="flex flex-wrap items-center gap-1.5 text-sm">
                  <span className="text-muted-foreground">Through</span>
                  <NodeLink name={nameOf(deployedVia)} onClick={() => goto(deployedVia)} />
                </span>
              ) : deployedOn.length === 0 ? (
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
          {Object.keys(element.properties).length > 0 && (
            <div className="flex flex-col gap-1.5">
              <SectionTitle>Properties</SectionTitle>
              <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 rounded-lg border bg-muted/40 px-2.5 py-2 text-xs">
                {Object.entries(element.properties).map(([k, v]) => (
                  <div key={k} className="contents">
                    <dt className="font-mono text-muted-foreground">{k}</dt>
                    <dd className="font-mono break-all">{v}</dd>
                  </div>
                ))}
              </dl>
            </div>
          )}
          {element.kind === "node" && (
            <div className="flex flex-col gap-1.5">
              <SectionTitle>
                Hosts <span className="tabular-nums">· {hosted.length}</span>
              </SectionTitle>
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
        </>
      )}

      <Separator />

      <RelationList
        title="Outgoing"
        relations={related.outgoing}
        otherEnd={(r) => r.target_id}
        nameOf={nameOf}
        goto={goto}
        actions={actions}
      />
      <RelationList
        title="Incoming"
        relations={related.incoming}
        otherEnd={(r) => r.source_id}
        nameOf={nameOf}
        goto={goto}
        actions={actions}
      />

      <Separator />
      <p className="font-mono text-[11px] text-muted-foreground">
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
      <SectionTitle>
        {title} <span className="tabular-nums">· {relations.length}</span>
      </SectionTitle>
      {relations.length === 0 && <p className="text-sm text-muted-foreground/70">None</p>}
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

/** An external link (runbook, docs): opens in a new tab and shows the host. */
function ExternalLink({ href }: { href: string }) {
  let host = href
  try {
    host = new URL(href).host
  } catch {
    // keep the raw text
  }
  return (
    <a
      href={href}
      target="_blank"
      rel="noreferrer noopener"
      className="truncate text-primary underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      {host}
    </a>
  )
}
