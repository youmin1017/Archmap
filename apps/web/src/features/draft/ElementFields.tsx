import { useMemo, useState } from "react"
import { Plus, Server, X } from "lucide-react"
import type { Element, ElementKind, LayerRule } from "@archmap/api-client"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { TextAutocomplete } from "@/components/ui/autocomplete"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { TechIcon } from "@/components/tech-icon/tech-icon"
import { canBeDeployed } from "../canvas/deployment"
import { ancestorIds, type ElementTree } from "../canvas/graph"
import { kinds, kindMeta } from "../canvas/kind"
import { layerViolation, violationMessage } from "../layers/layers"
import { allowedParentKinds, parseTags, requiresParent } from "./kindRules"

export type ElementFormValues = {
  kind: ElementKind
  name: string
  description: string
  technology: string
  tags: string[]
  parentId: string | null
  protected: boolean
  layer: string
  projects: string[]
  replicas: number
  properties: Record<string, string>
  /** Ids of the nodes the element is deployed on (empty for kinds that cannot be deployed). */
  deployedOn: string[]
}

export type ElementFormInit = {
  /** Existing element to edit; omit to create. */
  element?: Element
  /** Nodes the existing element is deployed on. */
  deployedOn?: readonly string[]
  defaultKind?: ElementKind
  defaultParentId?: string | null
  /** Layer preselected for a new element (e.g. the plane that was right-clicked). */
  defaultLayer?: string
}

const NONE = "__none__"

/** State and validation of the element form; `ElementFields` renders it. */
export function useElementForm(init: ElementFormInit, tree: ElementTree, layerRules: readonly LayerRule[]) {
  const { element } = init
  const [kind, setKind] = useState<ElementKind>(element?.kind ?? init.defaultKind ?? "container")
  const [name, setName] = useState(element?.name ?? "")
  const [description, setDescription] = useState(element?.description ?? "")
  const [technology, setTechnology] = useState(element?.technology ?? "")
  const [tags, setTags] = useState(element?.tags.join(", ") ?? "")
  const [projects, setProjects] = useState(element?.projects.join(", ") ?? "")
  const [replicas, setReplicas] = useState(String(element?.replicas ?? 1))
  const [props, setProps] = useState<{ key: string; value: string }[]>(() =>
    Object.entries(element?.properties ?? {}).map(([key, value]) => ({ key, value }))
  )
  const [layer, setLayer] = useState(element?.layer ?? init.defaultLayer ?? "")
  const [parentId, setParentId] = useState<string | null>(element ? element.parent_id : (init.defaultParentId ?? null))
  const [isProtected, setIsProtected] = useState(element?.protected ?? false)
  const [deployedOn, setDeployedOn] = useState<string[]>(() => [...(init.deployedOn ?? [])])
  const [touched, setTouched] = useState(false)

  const parentOptions = useMemo(() => {
    const allowed = allowedParentKinds[kind]
    const blocked = new Set<string>()
    if (element) {
      blocked.add(element.id)
      for (const e of tree.byId.values()) if (ancestorIds(tree, e.id).includes(element.id)) blocked.add(e.id)
    }
    return [...tree.byId.values()]
      .filter((e) => allowed.includes(e.kind) && !blocked.has(e.id))
      .sort((a, b) => a.name.localeCompare(b.name))
  }, [tree, kind, element])

  // Nodes with their path ("Physical Host / Docker Host"), for the "deployed on" picker.
  const nodes = useMemo(
    () =>
      [...tree.byId.values()]
        .filter((e) => e.kind === "node")
        .map((e) => ({
          id: e.id,
          label: [
            ...ancestorIds(tree, e.id)
              .reverse()
              .map((id) => tree.byId.get(id)?.name ?? ""),
            e.name,
          ].join(" / "),
        }))
        .sort((a, b) => a.label.localeCompare(b.label)),
    [tree]
  )

  // The chosen parent may stop being valid when the kind changes.
  const parentValid = parentId === null ? !requiresParent(kind) : parentOptions.some((p) => p.id === parentId)
  const nameValid = name.trim() !== ""
  const violated = layerViolation(layerRules, { kind }, layer)
  const replicaCount = Number(replicas)
  const replicasValid = Number.isInteger(replicaCount) && replicaCount >= 1 && replicaCount <= 100
  const canSubmit = nameValid && parentValid && replicasValid && violated === null

  function values(): ElementFormValues {
    return {
      kind,
      name: name.trim(),
      description,
      technology: technology.trim(),
      tags: parseTags(tags),
      parentId,
      protected: isProtected,
      layer: layer.trim(),
      projects: parseTags(projects),
      replicas: replicaCount,
      properties: Object.fromEntries(
        props.map((p) => [p.key.trim(), p.value.trim()] as const).filter(([k, v]) => k !== "" && v !== "")
      ),
      deployedOn: canBeDeployed(kind) ? deployedOn : [],
    }
  }

  return {
    editing: element !== undefined,
    kind,
    setKind,
    name,
    setName,
    description,
    setDescription,
    technology,
    setTechnology,
    tags,
    setTags,
    projects,
    setProjects,
    replicas,
    setReplicas,
    props,
    setProps,
    layer,
    setLayer,
    parentId,
    setParentId,
    isProtected,
    setIsProtected,
    deployedOn,
    setDeployedOn,
    touched,
    setTouched,
    parentOptions,
    parentValid,
    nameValid,
    violated,
    replicasValid,
    nodes,
    canSubmit,
    values,
  }
}

export type ElementForm = ReturnType<typeof useElementForm>

type FieldsProps = {
  form: ElementForm
  /** Known layer names offered as suggestions (free text is allowed). */
  layerOptions?: readonly string[]
  /** Known project names offered as quick picks. */
  projectOptions?: readonly string[]
  /** Suggestions for the technology field (free text is allowed). */
  technologyOptions?: readonly string[]
  /** Only maintainers may change the protected flag. */
  canProtect: boolean
  /** One column (side panel) instead of pairing kind and technology. */
  compact?: boolean
}

/** The element's fields, shared by the "Add element" dialog and the in-place editor of the details panel. */
export function ElementFields({
  form,
  layerOptions = [],
  projectOptions = [],
  technologyOptions = [],
  canProtect,
  compact = false,
}: FieldsProps) {
  const { kind } = form
  const parentItems = [
    ...(requiresParent(kind) ? [] : [{ value: NONE, label: "None (top level)" }]),
    ...form.parentOptions.map((p) => ({ value: p.id, label: `${p.name} (${kindMeta[p.kind].label})` })),
  ]
  const freeNodes = form.nodes.filter((n) => !form.deployedOn.includes(n.id))
  const labelOf = (id: string) => form.nodes.find((n) => n.id === id)?.label ?? "Unknown node"

  return (
    <>
      <div className={compact ? "flex flex-col gap-4" : "grid grid-cols-2 gap-3"}>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="el-kind">Kind</Label>
          <Select
            value={kind}
            onValueChange={(v) => form.setKind(v as ElementKind)}
            items={kinds.map((k) => ({ value: k, label: kindMeta[k].label }))}
          >
            <SelectTrigger id="el-kind" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {kinds.map((k) => (
                <SelectItem key={k} value={k}>
                  {kindMeta[k].label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="el-technology">Technology</Label>
          <TextAutocomplete
            id="el-technology"
            value={form.technology}
            onValueChange={form.setTechnology}
            items={technologyOptions}
            placeholder="Search or type, e.g. Go"
            maxLength={80}
            leading={form.technology.trim() ? <TechIcon technology={form.technology} className="size-4" /> : undefined}
            renderItem={(t) => (
              <>
                <TechIcon technology={t} className="size-4 shrink-0" />
                <span className="truncate">{t}</span>
              </>
            )}
          />
        </div>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="el-name">Name</Label>
        <Input
          id="el-name"
          value={form.name}
          onChange={(e) => form.setName(e.target.value)}
          autoFocus
          onBlur={() => form.setTouched(true)}
          aria-invalid={form.touched && !form.nameValid}
        />
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="el-parent">Parent</Label>
        <Select
          value={form.parentId ?? (requiresParent(kind) ? null : NONE)}
          onValueChange={(v) => form.setParentId(v === NONE ? null : v)}
          items={parentItems}
        >
          <SelectTrigger id="el-parent" className="w-full" aria-invalid={!form.parentValid}>
            <SelectValue placeholder="Choose a parent" />
          </SelectTrigger>
          <SelectContent>
            {parentItems.map((p) => (
              <SelectItem key={p.value} value={p.value}>
                {p.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {!form.parentValid && (
          <p className="text-xs text-destructive">
            A {kindMeta[kind].label.toLowerCase()} needs a {allowedParentKinds[kind].join(" / ")} as its parent.
          </p>
        )}
      </div>

      {canBeDeployed(kind) && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="el-deployed-on">Deployed on</Label>
          {form.deployedOn.length > 0 && (
            <ul className="flex flex-wrap gap-1" aria-label="Nodes this element is deployed on">
              {form.deployedOn.map((id) => (
                <li key={id}>
                  <Badge variant="secondary" className="gap-1 pr-1">
                    <Server data-icon="inline-start" aria-hidden />
                    {labelOf(id)}
                    <button
                      type="button"
                      aria-label={`Remove ${labelOf(id)}`}
                      onClick={() => form.setDeployedOn((ids) => ids.filter((x) => x !== id))}
                      className="rounded-full p-0.5 hover:bg-foreground/10 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                    >
                      <X className="size-3" aria-hidden />
                    </button>
                  </Badge>
                </li>
              ))}
            </ul>
          )}
          <Select
            value={null}
            onValueChange={(v) => {
              if (typeof v === "string") form.setDeployedOn((ids) => (ids.includes(v) ? ids : [...ids, v]))
            }}
            items={freeNodes.map((n) => ({ value: n.id, label: n.label }))}
            disabled={freeNodes.length === 0}
          >
            <SelectTrigger id="el-deployed-on" className="w-full">
              <SelectValue
                placeholder={
                  form.nodes.length === 0
                    ? "No nodes yet; add a Node first"
                    : freeNodes.length === 0
                      ? "Deployed on every node"
                      : "Add a node…"
                }
              />
            </SelectTrigger>
            <SelectContent>
              {freeNodes.map((n) => (
                <SelectItem key={n.id} value={n.id}>
                  {n.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">Recorded as a "deployed on" relation to each node.</p>
        </div>
      )}

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="el-description">Description</Label>
        <Textarea
          id="el-description"
          value={form.description}
          onChange={(e) => form.setDescription(e.target.value)}
          rows={2}
        />
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="el-layer">Layer</Label>
        <Input
          id="el-layer"
          list="el-layer-options"
          value={form.layer}
          onChange={(e) => form.setLayer(e.target.value)}
          placeholder="e.g. Services, Kubernetes, Physical"
          maxLength={60}
        />
        <datalist id="el-layer-options">
          {layerOptions.map((l) => (
            <option key={l} value={l} />
          ))}
        </datalist>
        {form.violated ? (
          <p role="alert" className="text-xs text-destructive">
            {violationMessage(form.violated)}
          </p>
        ) : (
          <p className="text-xs text-muted-foreground">Used by layers views to place the element on a layer.</p>
        )}
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="el-projects">Projects</Label>
        <Input
          id="el-projects"
          value={form.projects}
          onChange={(e) => form.setProjects(e.target.value)}
          placeholder="comma, separated"
        />
        {projectOptions.length > 0 && (
          <div className="flex flex-wrap gap-1" role="group" aria-label="Known projects">
            {projectOptions.map((p) => {
              const current = parseTags(form.projects)
              const on = current.some((c) => c.toLowerCase() === p.toLowerCase())
              return (
                <button
                  key={p}
                  type="button"
                  aria-pressed={on}
                  onClick={() =>
                    form.setProjects(
                      (on ? current.filter((c) => c.toLowerCase() !== p.toLowerCase()) : [...current, p]).join(", ")
                    )
                  }
                  className="rounded-4xl focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
                >
                  <Badge variant={on ? "default" : "outline"}>{p}</Badge>
                </button>
              )
            })}
          </div>
        )}
        <p className="text-xs text-muted-foreground">An element can belong to several projects.</p>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="el-replicas">Replicas</Label>
        <Input
          id="el-replicas"
          type="number"
          min={1}
          max={100}
          value={form.replicas}
          onChange={(e) => form.setReplicas(e.target.value)}
          aria-invalid={!form.replicasValid}
          className="w-28"
        />
        <p className="text-xs text-muted-foreground">
          How many instances this stands for; more than one is drawn stacked.
        </p>
      </div>

      <fieldset className="flex flex-col gap-1.5">
        <legend className="mb-1.5 text-sm leading-none font-medium">Properties</legend>
        {form.props.map((p, i) => (
          <div key={i} className="flex items-center gap-1.5">
            <Input
              aria-label={`Property ${i + 1} name`}
              list="el-property-keys"
              value={p.key}
              onChange={(e) => form.setProps((ps) => ps.map((x, j) => (j === i ? { ...x, key: e.target.value } : x)))}
              placeholder="name"
              maxLength={40}
              className="w-32 shrink-0"
            />
            <Input
              aria-label={`Property ${i + 1} value`}
              value={p.value}
              onChange={(e) => form.setProps((ps) => ps.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)))}
              placeholder="value"
              maxLength={200}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={`Remove property ${i + 1}`}
              onClick={() => form.setProps((ps) => ps.filter((_, j) => j !== i))}
            >
              <X aria-hidden />
            </Button>
          </div>
        ))}
        <datalist id="el-property-keys">
          {["ip", "port", "namespace", "cluster", "image"].map((k) => (
            <option key={k} value={k} />
          ))}
        </datalist>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="self-start"
          disabled={form.props.length >= 20}
          onClick={() => form.setProps((ps) => [...ps, { key: ps.some((x) => x.key === "ip") ? "" : "ip", value: "" }])}
        >
          <Plus aria-hidden /> Add property
        </Button>
      </fieldset>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="el-tags">Tags</Label>
        <Input
          id="el-tags"
          value={form.tags}
          onChange={(e) => form.setTags(e.target.value)}
          placeholder="comma, separated"
        />
      </div>

      {canProtect && (
        <div className="flex items-center justify-between gap-3 rounded-xl border p-3">
          <div>
            <Label htmlFor="el-protected">Protected</Label>
            <p className="text-xs text-muted-foreground">
              Changes need an independent maintainer to approve and merge.
            </p>
          </div>
          <Switch id="el-protected" checked={form.isProtected} onCheckedChange={form.setIsProtected} />
        </div>
      )}
    </>
  )
}
