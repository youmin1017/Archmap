import { useMemo, useState } from "react"
import { Check, Plus, Server, UserRound, Users, UsersRound, X } from "lucide-react"
import type {
  Criticality,
  Element,
  ElementKind,
  ElementType,
  Environment,
  LayerRule,
  Lifecycle,
  Owner,
  OwnerRole,
  Ownership,
} from "@archmap/api-client"

import { Button } from "@/components/ui/button"
import { TextAutocomplete } from "@/components/ui/autocomplete"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { TechIcon } from "@/components/tech-icon/tech-icon"
import { cn } from "@/lib/utils"
import { canBeDeployed } from "../canvas/deployment"
import {
  criticalities,
  daysSinceReview,
  effectiveOwner,
  environments,
  labelOf as optionLabel,
  lifecycles,
  ownerRoles,
  REVIEW_MAX_DAYS,
  today,
  typesForKind,
} from "../canvas/element"
import { ancestorIds, type ElementTree } from "../canvas/graph"
import { kinds, kindMeta } from "../canvas/kind"
import { layerViolation, violationMessage } from "../layers/layers"
import { useDirectory } from "../users/queries"
import { allowedParentKinds, parseTags, requiresParent } from "./kindRules"
import { ownerSections } from "./ownerPicker"

const choiceIcon = { user: UserRound, group: Users, team: UsersRound, person: UserRound } as const

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
  type: ElementType
  environment: Environment
  lifecycle: Lifecycle
  criticality: Criticality
  runbookUrl: string
  docsUrl: string
  lastReviewedAt: string | null
  /** The element's own owner per role ("" = none here, inherit from the parents). */
  ownership: Record<OwnerRole, string>
}

const emptyOwnership = (): Record<OwnerRole, string> => ({ owner: "", maintainer: "", oncall: "", escalation: "" })

/** Accepts an empty value or an absolute http(s) URL (the API checks it too). */
export function urlProblem(u: string): string | null {
  if (u.trim() === "") return null
  try {
    const url = new URL(u.trim())
    return url.protocol === "http:" || url.protocol === "https:" ? null : "Use an http(s) link."
  } catch {
    return "Use a full link, e.g. https://…"
  }
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
  /** The existing element's own ownership assignments. */
  ownerships?: readonly Ownership[]
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
  const [type, setType] = useState<ElementType>(element?.type ?? "")
  const [environment, setEnvironment] = useState<Environment>(element?.environment ?? "")
  const [lifecycle, setLifecycle] = useState<Lifecycle>(element?.lifecycle ?? "active")
  const [criticality, setCriticality] = useState<Criticality>(element?.criticality ?? "")
  const [runbookUrl, setRunbookUrl] = useState(element?.runbook_url ?? "")
  const [docsUrl, setDocsUrl] = useState(element?.docs_url ?? "")
  const [lastReviewedAt, setLastReviewedAt] = useState(element?.last_reviewed_at ?? "")
  const [ownership, setOwnership] = useState<Record<OwnerRole, string>>(() => {
    const own = emptyOwnership()
    for (const o of init.ownerships ?? []) own[o.role] = o.owner_id
    return own
  })
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
  const violated = layerViolation(layerRules, { kind, type }, layer)
  const replicaCount = Number(replicas)
  const replicasValid = Number.isInteger(replicaCount) && replicaCount >= 1 && replicaCount <= 100
  const runbookProblem = urlProblem(runbookUrl)
  const docsProblem = urlProblem(docsUrl)
  const typeFits = type === "" || typesForKind(kind).some((t) => t.value === type)
  const canSubmit =
    nameValid && parentValid && replicasValid && violated === null && !runbookProblem && !docsProblem && typeFits

  // A type that does not fit the new kind is cleared (the API's lint would reject it).
  function changeKind(next: ElementKind) {
    setKind(next)
    if (type !== "" && !typesForKind(next).some((t) => t.value === type)) setType("")
  }

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
      type,
      environment,
      lifecycle,
      criticality,
      runbookUrl: runbookUrl.trim(),
      docsUrl: docsUrl.trim(),
      lastReviewedAt: lastReviewedAt === "" ? null : lastReviewedAt,
      ownership,
    }
  }

  return {
    editing: element !== undefined,
    kind,
    setKind: changeKind,
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
    type,
    setType,
    environment,
    setEnvironment,
    lifecycle,
    setLifecycle,
    criticality,
    setCriticality,
    runbookUrl,
    setRunbookUrl,
    docsUrl,
    setDocsUrl,
    lastReviewedAt,
    setLastReviewedAt,
    ownership,
    setOwnership,
    runbookProblem,
    docsProblem,
    touched,
    setTouched,
    parentOptions,
    parentValid,
    nameValid,
    violated,
    replicasValid,
    nodes,
    tree,
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
  /** Owners of the draft's model, offered per role. */
  owners?: readonly Owner[]
}

/** Small uppercase heading that groups related fields. */
function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-3 border-t pt-4 first:border-t-0 first:pt-0">
      <h3 className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">{title}</h3>
      {children}
    </section>
  )
}

function Field({
  id,
  label,
  hint,
  error,
  className,
  children,
}: {
  id?: string
  label: string
  hint?: React.ReactNode
  /** Shown instead of the hint, in the destructive color. */
  error?: React.ReactNode
  className?: string
  children: React.ReactNode
}) {
  return (
    <div className={cn("flex min-w-0 flex-col gap-1.5", className)}>
      <Label htmlFor={id}>{label}</Label>
      {children}
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : (
        hint && <p className="text-xs text-muted-foreground">{hint}</p>
      )}
    </div>
  )
}

const chip =
  "inline-flex h-6 items-center rounded-md border px-2 text-xs transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none aria-pressed:border-primary/40 aria-pressed:bg-primary/10 aria-pressed:text-foreground"

/** A few mutually exclusive options as selectable chips (role="radio"). */
function ChipGroup({
  label,
  hint,
  options,
  value,
  onChange,
}: {
  label: string
  hint?: string
  options: readonly { value: string; label: string }[]
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <span className="text-sm font-medium" id={`chips-${label}`}>
        {label}
      </span>
      <div role="radiogroup" aria-labelledby={`chips-${label}`} className="flex flex-wrap gap-1">
        {options.map((o) => (
          <button
            key={o.value || "none"}
            type="button"
            role="radio"
            aria-checked={value === o.value}
            onClick={() => onChange(o.value)}
            className={cn(
              chip,
              value === o.value
                ? "border-primary/40 bg-primary/10 text-foreground"
                : "text-muted-foreground hover:bg-muted hover:text-foreground"
            )}
          >
            {o.label}
          </button>
        ))}
      </div>
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

/** The element's fields, shared by the "Add element" dialog and the in-place editor of the details panel. */
export function ElementFields({
  form,
  layerOptions = [],
  projectOptions = [],
  technologyOptions = [],
  canProtect,
  compact = false,
  owners = [],
}: FieldsProps) {
  const { kind } = form
  const parentItems = [
    ...(requiresParent(kind) ? [] : [{ value: NONE, label: "None (top level)" }]),
    ...form.parentOptions.map((p) => ({ value: p.id, label: `${p.name} (${kindMeta[p.kind].label})` })),
  ]
  const freeNodes = form.nodes.filter((n) => !form.deployedOn.includes(n.id))
  const labelOf = (id: string) => form.nodes.find((n) => n.id === id)?.label ?? "Unknown node"
  const parentRule = `A ${kindMeta[kind].label.toLowerCase()} lives inside a ${allowedParentKinds[kind].join(" / ")}.`
  // A missing parent is only an error once the user has interacted; before that it is a hint.
  const parentError = !form.parentValid && (form.parentId !== null || form.touched)
  const pair = compact ? "flex flex-col gap-3" : "grid grid-cols-2 gap-3"
  const currentProjects = parseTags(form.projects)
  const parent = form.parentId ? form.tree.byId.get(form.parentId) : undefined
  const inheritedEnv = parent?.effective?.environment ?? ""
  const reviewAge = daysSinceReview(form.lastReviewedAt || null)
  const directory = useDirectory()
  const sections = ownerSections(owners, directory.data, Object.values(form.ownership))
  const choices = sections.flatMap((sec) => sec.choices)
  const ownerName = (id: string) =>
    choices.find((c) => c.value === id)?.label ?? owners.find((o) => o.id === id)?.name ?? "Unknown owner"

  return (
    <div className="flex flex-col gap-4">
      <Section title="Basics">
        <div className={pair}>
          <Field id="el-kind" label="Kind">
            <Select
              value={kind}
              onValueChange={(v) => form.setKind(v as ElementKind)}
              items={kinds.map((k) => ({ value: k, label: kindMeta[k].label }))}
            >
              <SelectTrigger id="el-kind" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {kinds.map((k) => {
                  const Icon = kindMeta[k].icon
                  return (
                    <SelectItem key={k} value={k}>
                      <Icon aria-hidden style={{ color: `var(--kind-${k})` }} />
                      {kindMeta[k].label}
                    </SelectItem>
                  )
                })}
              </SelectContent>
            </Select>
          </Field>
          <Field id="el-name" label="Name">
            <Input
              id="el-name"
              value={form.name}
              onChange={(e) => form.setName(e.target.value)}
              autoFocus
              onBlur={() => form.setTouched(true)}
              aria-invalid={form.touched && !form.nameValid}
              placeholder={kind === "person" ? "e.g. Customer" : "e.g. Orders API"}
            />
          </Field>
        </div>

        <div className={pair}>
          <Field
            id="el-parent"
            label="Parent"
            error={parentError && parentRule}
            hint={requiresParent(kind) && !form.parentId ? parentRule : undefined}
          >
            <Select
              value={form.parentId ?? (requiresParent(kind) ? null : NONE)}
              onValueChange={(v) => {
                form.setParentId(v === NONE ? null : v)
                form.setTouched(true)
              }}
              items={parentItems}
            >
              <SelectTrigger id="el-parent" className="w-full" aria-invalid={parentError}>
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
          </Field>
          <Field id="el-technology" label="Technology">
            <TextAutocomplete
              id="el-technology"
              value={form.technology}
              onValueChange={form.setTechnology}
              items={technologyOptions}
              placeholder="e.g. Go, PostgreSQL"
              maxLength={80}
              leading={
                form.technology.trim() ? <TechIcon technology={form.technology} className="size-4" /> : undefined
              }
              renderItem={(t) => (
                <>
                  <TechIcon technology={t} className="size-4 shrink-0" />
                  <span className="truncate">{t}</span>
                </>
              )}
            />
          </Field>
        </div>

        <Field id="el-description" label="Description">
          <Textarea
            id="el-description"
            value={form.description}
            onChange={(e) => form.setDescription(e.target.value)}
            rows={2}
            placeholder="What does it do?"
          />
        </Field>
      </Section>

      <Section title="Placement">
        {canBeDeployed(kind) && (
          <Field id="el-deployed-on" label="Deployed on" hint={'Each node becomes a "deployed on" relation.'}>
            {form.deployedOn.length > 0 && (
              <ul className="flex flex-wrap gap-1" aria-label="Nodes this element is deployed on">
                {form.deployedOn.map((id) => (
                  <li key={id}>
                    <span className="inline-flex h-6 items-center gap-1 rounded-md border bg-muted/50 pr-0.5 pl-1.5 text-xs">
                      <Server className="size-3 text-muted-foreground" aria-hidden />
                      {labelOf(id)}
                      <button
                        type="button"
                        aria-label={`Remove ${labelOf(id)}`}
                        onClick={() => form.setDeployedOn((ids) => ids.filter((x) => x !== id))}
                        className="grid size-4 place-items-center rounded text-muted-foreground hover:bg-foreground/10 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                      >
                        <X className="size-3" aria-hidden />
                      </button>
                    </span>
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
          </Field>
        )}

        <div
          className={
            compact ? "grid grid-cols-[minmax(0,1fr)_5.5rem] gap-3" : "grid grid-cols-[minmax(0,1fr)_7rem] gap-3"
          }
        >
          <Field
            id="el-layer"
            label="Layer"
            error={form.violated && violationMessage(form.violated)}
            hint="Where layers views place it."
          >
            <Input
              id="el-layer"
              list="el-layer-options"
              value={form.layer}
              onChange={(e) => form.setLayer(e.target.value)}
              placeholder="e.g. Services"
              maxLength={60}
              aria-invalid={form.violated !== null}
            />
            <datalist id="el-layer-options">
              {layerOptions.map((l) => (
                <option key={l} value={l} />
              ))}
            </datalist>
          </Field>
          <Field id="el-replicas" label="Replicas" error={!form.replicasValid && "1 to 100"}>
            <Input
              id="el-replicas"
              type="number"
              min={1}
              max={100}
              value={form.replicas}
              onChange={(e) => form.setReplicas(e.target.value)}
              aria-invalid={!form.replicasValid}
              className="tabular-nums"
            />
          </Field>
        </div>
      </Section>

      <Section title="Operations">
        {typesForKind(kind).length > 0 && (
          <ChipGroup
            label="Type"
            hint="Datastore types are drawn as cylinders."
            options={[{ value: "", label: "None" }, ...typesForKind(kind)]}
            value={form.type}
            onChange={(v) => form.setType(v as ElementType)}
          />
        )}
        <ChipGroup
          label="Environment"
          hint={
            form.environment === "" && inheritedEnv
              ? `Inherits ${optionLabel(environments, inheritedEnv).toLowerCase()} from ${parent?.name}.`
              : "Children inherit it unless they set their own."
          }
          options={[
            { value: "", label: inheritedEnv ? `Inherit (${optionLabel(environments, inheritedEnv)})` : "Not set" },
            ...environments,
          ]}
          value={form.environment}
          onChange={(v) => form.setEnvironment(v as Environment)}
        />
        <div className={pair}>
          <ChipGroup
            label="Lifecycle"
            options={lifecycles}
            value={form.lifecycle}
            onChange={(v) => form.setLifecycle(v as Lifecycle)}
          />
          <ChipGroup
            label="Criticality"
            options={[{ value: "", label: "Not set" }, ...criticalities]}
            value={form.criticality}
            onChange={(v) => form.setCriticality(v as Criticality)}
          />
        </div>
        <div className={pair}>
          <Field id="el-runbook" label="Runbook" error={form.runbookProblem}>
            <Input
              id="el-runbook"
              type="url"
              value={form.runbookUrl}
              onChange={(e) => form.setRunbookUrl(e.target.value)}
              placeholder="https://…"
              aria-invalid={form.runbookProblem !== null}
            />
          </Field>
          <Field id="el-docs" label="Docs" error={form.docsProblem}>
            <Input
              id="el-docs"
              type="url"
              value={form.docsUrl}
              onChange={(e) => form.setDocsUrl(e.target.value)}
              placeholder="https://…"
              aria-invalid={form.docsProblem !== null}
            />
          </Field>
        </div>
        <Field
          id="el-reviewed"
          label="Last reviewed"
          hint={
            reviewAge === null
              ? "Never reviewed."
              : reviewAge > REVIEW_MAX_DAYS
                ? `${reviewAge} days ago; review it again.`
                : `${reviewAge} days ago.`
          }
        >
          <div className="flex items-center gap-1.5">
            <Input
              id="el-reviewed"
              type="date"
              value={form.lastReviewedAt}
              onChange={(e) => form.setLastReviewedAt(e.target.value)}
              className="w-40"
            />
            <Button type="button" variant="outline" size="sm" onClick={() => form.setLastReviewedAt(today())}>
              Today
            </Button>
            {form.lastReviewedAt && (
              <Button type="button" variant="ghost" size="sm" onClick={() => form.setLastReviewedAt("")}>
                Clear
              </Button>
            )}
          </div>
        </Field>
      </Section>

      <Section title="Ownership">
        <div className={compact ? "flex flex-col gap-3" : "grid grid-cols-2 gap-3"}>
          {ownerRoles.map((r) => {
            const inherited = parent ? effectiveOwner(parent.effective, parent.id, r.value) : null
            const inheritLabel = inherited ? `Inherit: ${ownerName(inherited.ownerId)}` : "None (nothing to inherit)"
            const items = [
              { value: "", label: inheritLabel },
              ...choices.map((c) => ({ value: c.value, label: c.label })),
            ]
            return (
              <Field key={r.value} id={`el-owner-${r.value}`} label={r.label} hint={r.hint}>
                <Select
                  value={form.ownership[r.value]}
                  onValueChange={(v) =>
                    form.setOwnership((cur) => ({ ...cur, [r.value]: typeof v === "string" ? v : "" }))
                  }
                  items={items}
                >
                  <SelectTrigger id={`el-owner-${r.value}`} className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="">{inheritLabel}</SelectItem>
                    {sections.map((sec) => (
                      <SelectGroup key={sec.label}>
                        <SelectSeparator />
                        <SelectLabel>{sec.label}</SelectLabel>
                        {sec.choices.map((c) => {
                          const Icon = choiceIcon[c.kind]
                          return (
                            <SelectItem
                              key={c.value}
                              value={c.value}
                              disabled={c.inactive && form.ownership[r.value] !== c.value}
                            >
                              <Icon className="text-muted-foreground" aria-hidden />
                              <span className={cn("truncate", c.inactive && "line-through")}>{c.label}</span>
                              {(c.detail || c.inactive) && (
                                <span className="truncate text-xs text-muted-foreground">
                                  {c.inactive ? (c.kind === "group" ? "deleted" : "disabled") : c.detail}
                                </span>
                              )}
                            </SelectItem>
                          )
                        })}
                      </SelectGroup>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            )
          })}
        </div>
        <p className="text-xs text-muted-foreground">
          Pick a user, a group or a named team. Add teams without accounts with Owners in the draft bar.
        </p>
      </Section>

      <Section title="Organization">
        <Field id="el-projects" label="Projects">
          <Input
            id="el-projects"
            value={form.projects}
            onChange={(e) => form.setProjects(e.target.value)}
            placeholder="comma, separated"
          />
          {projectOptions.length > 0 && (
            <div className="flex flex-wrap gap-1" role="group" aria-label="Known projects">
              {projectOptions.map((p) => {
                const on = currentProjects.some((c) => c.toLowerCase() === p.toLowerCase())
                return (
                  <button
                    key={p}
                    type="button"
                    aria-pressed={on}
                    onClick={() =>
                      form.setProjects(
                        (on
                          ? currentProjects.filter((c) => c.toLowerCase() !== p.toLowerCase())
                          : [...currentProjects, p]
                        ).join(", ")
                      )
                    }
                    className={cn(chip, !on && "text-muted-foreground hover:bg-muted hover:text-foreground")}
                  >
                    {on ? <Check className="mr-1 size-3" aria-hidden /> : <Plus className="mr-1 size-3" aria-hidden />}
                    {p}
                  </button>
                )
              })}
            </div>
          )}
        </Field>
        <Field id="el-tags" label="Tags">
          <Input
            id="el-tags"
            value={form.tags}
            onChange={(e) => form.setTags(e.target.value)}
            placeholder="comma, separated"
          />
        </Field>
      </Section>

      <Section title="Properties">
        <fieldset className="relative flex flex-col gap-1.5">
          <legend className="sr-only">Properties</legend>
          {form.props.length === 0 && (
            <p className="text-xs text-muted-foreground">Key / value details such as ip, port or namespace.</p>
          )}
          {form.props.map((p, i) => (
            <div key={i} className="flex items-center gap-1.5">
              <Input
                aria-label={`Property ${i + 1} name`}
                list="el-property-keys"
                value={p.key}
                onChange={(e) => form.setProps((ps) => ps.map((x, j) => (j === i ? { ...x, key: e.target.value } : x)))}
                placeholder="name"
                maxLength={40}
                className="w-28 shrink-0 font-mono text-xs"
              />
              <Input
                aria-label={`Property ${i + 1} value`}
                value={p.value}
                onChange={(e) =>
                  form.setProps((ps) => ps.map((x, j) => (j === i ? { ...x, value: e.target.value } : x)))
                }
                placeholder="value"
                maxLength={200}
                className="font-mono text-xs"
              />
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
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
            variant="ghost"
            size="sm"
            className="-ml-2 self-start text-muted-foreground"
            disabled={form.props.length >= 20}
            onClick={() =>
              form.setProps((ps) => [...ps, { key: ps.some((x) => x.key === "ip") ? "" : "ip", value: "" }])
            }
          >
            <Plus aria-hidden /> Add property
          </Button>
        </fieldset>
      </Section>

      {canProtect && (
        <label
          htmlFor="el-protected"
          className="flex cursor-pointer items-center justify-between gap-3 rounded-lg border bg-muted/30 p-3"
        >
          <span className="flex flex-col gap-0.5">
            <span className="text-sm font-medium">Protected</span>
            <span className="text-xs text-muted-foreground">
              Changes need an independent maintainer to approve and merge.
            </span>
          </span>
          <Switch id="el-protected" checked={form.isProtected} onCheckedChange={form.setIsProtected} />
        </label>
      )}
    </div>
  )
}
