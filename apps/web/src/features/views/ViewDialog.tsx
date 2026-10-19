import { useMemo, useState } from "react"
import { Boxes, Component, Globe, Layers, ListChecks, Network, Server, type LucideIcon } from "lucide-react"
import type { Element, ElementKind, View, ViewInput, ViewKind } from "@archmap/api-client"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { kindMeta } from "../canvas/kind"
import { LayersEditor, defaultLayers } from "./LayersEditor"

export const viewKinds: {
  value: ViewKind
  label: string
  icon: LucideIcon
  hint: string
  scope: "none" | "required" | "optional"
  scopeKinds: ElementKind[]
}[] = [
  {
    value: "landscape",
    label: "Landscape",
    icon: Globe,
    hint: "All people and systems.",
    scope: "none",
    scopeKinds: [],
  },
  {
    value: "context",
    label: "System context",
    icon: Network,
    hint: "One system and the people and systems it talks to.",
    scope: "required",
    scopeKinds: ["system"],
  },
  {
    value: "container",
    label: "Containers",
    icon: Boxes,
    hint: "The containers of one system and what they connect to.",
    scope: "required",
    scopeKinds: ["system"],
  },
  {
    value: "component",
    label: "Components",
    icon: Component,
    hint: "The components of one container and its neighbours.",
    scope: "required",
    scopeKinds: ["container"],
  },
  {
    value: "deployment",
    label: "Deployment",
    icon: Server,
    hint: "Infrastructure nodes and what runs on them.",
    scope: "optional",
    scopeKinds: ["node"],
  },
  {
    value: "custom",
    label: "Custom",
    icon: ListChecks,
    hint: "A hand-picked set of elements.",
    scope: "none",
    scopeKinds: [],
  },
  {
    value: "layers",
    label: "Layers",
    icon: Layers,
    hint: "Flat plans stacked per layer, browsable one layer at a time, e.g. User, Services, Kubernetes, Physical.",
    scope: "none",
    scopeKinds: [],
  },
]

const WHOLE = "__whole__"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Every element of the model (for scope and member pickers). */
  elements: readonly Element[]
  /** Existing view to edit; omit to create. */
  view?: View
  /** Current members of an existing custom view. */
  members?: readonly string[]
  pending?: boolean
  onSubmit: (input: ViewInput) => void
}

export function ViewDialog(props: Props) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {props.open && <ViewForm key={props.view?.id ?? "new"} {...props} />}
      </DialogContent>
    </Dialog>
  )
}

function ViewForm({ elements, view, members, pending, onOpenChange, onSubmit }: Props) {
  const editing = view !== undefined
  const [kind, setKind] = useState<ViewKind>(view?.kind ?? "container")
  const [name, setName] = useState(view?.name ?? "")
  const [description, setDescription] = useState(view?.description ?? "")
  const [scope, setScope] = useState<string | null>(view?.scope_element_id ?? null)
  const [layers, setLayers] = useState<string[]>(view?.layers.length ? view.layers : defaultLayers)
  const [picked, setPicked] = useState<ReadonlySet<string>>(() => new Set(members ?? []))
  const [filter, setFilter] = useState("")
  const [touched, setTouched] = useState(false)

  const spec = viewKinds.find((k) => k.value === kind)!
  const scopeOptions = useMemo(
    () => elements.filter((e) => spec.scopeKinds.includes(e.kind)).sort((a, b) => a.name.localeCompare(b.name)),
    [elements, spec]
  )
  const memberOptions = useMemo(() => {
    const q = filter.trim().toLowerCase()
    return [...elements]
      .filter((e) => !q || e.name.toLowerCase().includes(q))
      .sort((a, b) => a.name.localeCompare(b.name))
  }, [elements, filter])

  const scopeValid =
    spec.scope === "none" ||
    (scope !== null && scopeOptions.some((e) => e.id === scope)) ||
    (spec.scope === "optional" && scope === null)
  const canSubmit = name.trim() !== "" && scopeValid && !pending

  const scopeItems = [
    ...(spec.scope === "optional" ? [{ value: WHOLE, label: "All infrastructure" }] : []),
    ...scopeOptions.map((e) => ({ value: e.id, label: e.name })),
  ]

  function changeKind(next: ViewKind) {
    setKind(next)
    setScope(null) // a scope of the old kind is rarely valid for the new one
  }

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!canSubmit) return
        onSubmit({
          name: name.trim(),
          kind,
          description,
          scope_element_id: spec.scope === "none" ? null : scope,
          ...(kind === "custom" ? { elements: [...picked] } : {}),
          ...(kind === "layers" ? { layers } : {}),
        })
      }}
    >
      <DialogHeader>
        <DialogTitle>{editing ? "Edit view" : "New view"}</DialogTitle>
        <DialogDescription>
          Views are saved perspectives. They never change the model, so they need no review.
        </DialogDescription>
      </DialogHeader>

      <fieldset className="flex flex-col gap-1.5">
        <legend className="mb-1.5 text-sm leading-none font-medium">Kind</legend>
        {/* Kind picker as cards: every option visible at once, with its icon. */}
        <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-4" role="radiogroup" aria-label="View kind">
          {viewKinds.map((k) => {
            const on = k.value === kind
            const Icon = k.icon
            return (
              <button
                key={k.value}
                type="button"
                role="radio"
                aria-checked={on}
                onClick={() => changeKind(k.value)}
                title={k.hint}
                className={cn(
                  "flex flex-col items-start gap-1.5 rounded-lg border p-2.5 text-left text-xs font-medium transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
                  on ? "border-primary bg-primary/5 ring-3 ring-primary/15" : "hover:bg-muted/60"
                )}
              >
                <Icon className={cn("size-4", on ? "text-primary" : "text-muted-foreground")} aria-hidden />
                {k.label}
              </button>
            )
          })}
        </div>
        <p className="text-xs text-muted-foreground">{spec.hint}</p>
      </fieldset>

      {spec.scope !== "none" && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="view-scope">{spec.scopeKinds.map((k) => kindMeta[k].label).join(" / ")}</Label>
          <Select
            value={scope ?? (spec.scope === "optional" ? WHOLE : "")}
            onValueChange={(v) => {
              setScope(v === WHOLE || !v ? null : v)
              setTouched(true)
            }}
            items={scopeItems}
          >
            <SelectTrigger id="view-scope" className="w-full" aria-invalid={touched && !scopeValid}>
              <SelectValue placeholder={`Choose a ${spec.scopeKinds[0]}`} />
            </SelectTrigger>
            <SelectContent>
              {scopeItems.map((s) => (
                <SelectItem key={s.value} value={s.value}>
                  {s.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {scopeOptions.length === 0 && (
            <p className="text-xs text-destructive">There is no {spec.scopeKinds[0]} in the model yet.</p>
          )}
        </div>
      )}

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="view-name">Name</Label>
        <Input
          id="view-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          onBlur={() => setTouched(true)}
          autoFocus
          maxLength={120}
          placeholder={`e.g. ${scopeOptions.find((e) => e.id === scope)?.name ?? "Shop"} ${spec.label.toLowerCase()}`}
        />
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="view-description">Description</Label>
        <Textarea id="view-description" value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
      </div>

      {kind === "layers" && <LayersEditor layers={layers} onChange={setLayers} />}

      {kind === "custom" && (
        <fieldset className="flex flex-col gap-1.5">
          <legend className="mb-1.5 text-sm leading-none font-medium">
            Elements <span className="font-normal text-muted-foreground">({picked.size} selected)</span>
          </legend>
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter elements"
            aria-label="Filter elements"
          />
          <ul className="max-h-48 overflow-y-auto rounded-lg border p-1">
            {memberOptions.map((e) => {
              const Icon = kindMeta[e.kind].icon
              return (
                <li key={e.id}>
                  <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-muted">
                    <input
                      type="checkbox"
                      checked={picked.has(e.id)}
                      onChange={() =>
                        setPicked((prev) => {
                          const next = new Set(prev)
                          if (!next.delete(e.id)) next.add(e.id)
                          return next
                        })
                      }
                    />
                    <Icon className="size-4" style={{ color: `var(--kind-${e.kind})` }} aria-hidden />
                    <span className="truncate">{e.name}</span>
                  </label>
                </li>
              )
            })}
          </ul>
        </fieldset>
      )}

      <DialogFooter className="items-center">
        {!canSubmit && !pending && (
          <span className="mr-auto hidden text-xs text-muted-foreground sm:inline">
            {!scopeValid ? `Choose a ${spec.scopeKinds[0]} first.` : "Give the view a name."}
          </span>
        )}
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button type="submit" disabled={!canSubmit}>
          {editing ? "Save view" : "Create view"}
        </Button>
      </DialogFooter>
    </form>
  )
}
