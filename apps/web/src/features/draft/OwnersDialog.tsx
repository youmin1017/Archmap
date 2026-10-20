import { useState } from "react"
import { Pencil, Trash2, UserRound, Users, UsersRound } from "lucide-react"
import type { ChangeInput, Owner, OwnerKind, Ownership } from "@archmap/api-client"

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
import { cn } from "@/lib/utils"
import { ownerViews } from "../canvas/owners"
import { useDirectory } from "../users/queries"
import { urlProblem } from "./ElementFields"

const heading = "text-[11px] font-medium tracking-wide text-muted-foreground uppercase"
const kinds: { value: OwnerKind; label: string }[] = [
  { value: "team", label: "Team" },
  { value: "person", label: "Person" },
]
const kindLabel: Record<OwnerKind, string> = { team: "Team", person: "Person", user: "User", group: "Group" }
const kindIcon = { team: UsersRound, person: UserRound, user: UserRound, group: Users } as const

/** Changes that add or edit an owner; null when an edit changes nothing. */
export function ownerInput(
  existing: Owner | null,
  v: Omit<Owner, "id" | "version">,
  newId: string
): ChangeInput | null {
  if (!existing) return { op: "add", target_type: "owner", target_id: newId, patch: { ...v } }
  const patch: Record<string, unknown> = {}
  for (const k of ["kind", "name", "contact", "oncall_url"] as const) if (v[k] !== existing[k]) patch[k] = v[k]
  return Object.keys(patch).length === 0 ? null : { op: "update", target_type: "owner", target_id: existing.id, patch }
}

/**
 * The draft's owner directory: teams and people that elements can be assigned to.
 * Owners are model data, so every edit here is a change in the draft (ADR-063).
 */
export function OwnersDialog({
  open,
  onOpenChange,
  owners,
  ownerships,
  pending,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  owners: readonly Owner[]
  ownerships: readonly Ownership[]
  pending?: boolean
  onSubmit: (inputs: ChangeInput[]) => void
}) {
  const [editing, setEditing] = useState<Owner | null>(null)
  const [removing, setRemoving] = useState<Owner | null>(null)
  const uses = (id: string) => ownerships.filter((o) => o.owner_id === id).length
  const directory = useDirectory()
  const views = ownerViews(owners, directory.data)

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o)
        if (!o) {
          setEditing(null)
          setRemoving(null)
        }
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <div className="flex max-h-[85vh] min-h-0 flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Owners</DialogTitle>
            <DialogDescription>
              Owners elements are assigned to. Users and groups are added when you pick them for an element; add teams
              and people without accounts here. Changes are part of this draft and go through review.
            </DialogDescription>
          </DialogHeader>
          <div className="-mx-4 flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-1">
            {owners.length === 0 ? (
              <p className="text-sm text-muted-foreground">No owners yet.</p>
            ) : (
              <ul className="flex flex-col divide-y rounded-lg border" aria-label="Owners">
                {owners.map((owner) => {
                  const o = views.get(owner.id) ?? { ...owner, inactive: false }
                  const Icon = kindIcon[o.kind]
                  const linked = o.kind === "user" || o.kind === "group"
                  return (
                    <li key={o.id} className="flex items-center gap-3 px-3 py-2">
                      <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                      <span className="flex min-w-0 flex-1 flex-col">
                        <span
                          className={cn(
                            "truncate text-sm font-medium",
                            o.inactive && "text-muted-foreground line-through"
                          )}
                        >
                          {o.name}
                        </span>
                        <span className="truncate text-xs text-muted-foreground">
                          {kindLabel[o.kind]}
                          {o.inactive && (o.kind === "group" ? " (deleted)" : " (disabled)")}
                          {o.contact && ` · ${o.contact}`} · {uses(o.id)} assignment{uses(o.id) === 1 ? "" : "s"}
                        </span>
                      </span>
                      {!linked && (
                        <Button
                          size="icon-sm"
                          variant="ghost"
                          aria-label={`Edit ${o.name}`}
                          onClick={() => {
                            setRemoving(null)
                            setEditing(o)
                          }}
                        >
                          <Pencil aria-hidden />
                        </Button>
                      )}
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        aria-label={`Remove ${o.name}`}
                        onClick={() => {
                          setEditing(null)
                          setRemoving(o)
                        }}
                      >
                        <Trash2 aria-hidden />
                      </Button>
                    </li>
                  )
                })}
              </ul>
            )}

            {removing && (
              <div
                role="alert"
                className="flex flex-col gap-2 rounded-lg border border-destructive/30 bg-destructive/5 p-3"
              >
                <p className="text-sm">
                  Remove <span className="font-medium">{removing.name}</span>?
                  {uses(removing.id) > 0 &&
                    ` Its ${uses(removing.id)} assignment${uses(removing.id) === 1 ? "" : "s"} are removed too; those elements fall back to their parents' owners.`}
                </p>
                <div className="flex justify-end gap-2">
                  <Button size="sm" variant="outline" onClick={() => setRemoving(null)}>
                    Cancel
                  </Button>
                  <Button
                    size="sm"
                    variant="destructive"
                    disabled={pending}
                    onClick={() => {
                      onSubmit([
                        { op: "delete", target_type: "owner", target_id: removing.id, cascade: uses(removing.id) > 0 },
                      ])
                      setRemoving(null)
                    }}
                  >
                    Remove
                  </Button>
                </div>
              </div>
            )}

            <OwnerForm
              key={editing?.id ?? "new"}
              owner={editing}
              taken={owners.filter((o) => o.id !== editing?.id).map((o) => o.name.trim().toLowerCase())}
              pending={pending}
              onCancel={() => setEditing(null)}
              onSave={(input) => {
                if (input) onSubmit([input])
                setEditing(null)
              }}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Close
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function OwnerForm({
  owner,
  taken,
  pending,
  onCancel,
  onSave,
}: {
  owner: Owner | null
  taken: readonly string[]
  pending?: boolean
  onCancel: () => void
  onSave: (input: ChangeInput | null) => void
}) {
  const [kind, setKind] = useState<OwnerKind>(owner?.kind ?? "team")
  const [name, setName] = useState(owner?.name ?? "")
  const [contact, setContact] = useState(owner?.contact ?? "")
  const [oncall, setOncall] = useState(owner?.oncall_url ?? "")
  const [touched, setTouched] = useState(false)
  const nameTaken = taken.includes(name.trim().toLowerCase())
  const oncallProblem = urlProblem(oncall)
  const missing = !name.trim() ? "Give the owner a name." : nameTaken ? "That name is taken." : oncallProblem
  const id = (f: string) => `owner-${f}`

  return (
    <form
      className="flex flex-col gap-3"
      aria-labelledby={id("title")}
      onSubmit={(e) => {
        e.preventDefault()
        setTouched(true)
        if (missing || pending) return
        onSave(
          ownerInput(
            owner,
            { kind, name: name.trim(), contact: contact.trim(), oncall_url: oncall.trim() },
            crypto.randomUUID()
          )
        )
        if (!owner) {
          setName("")
          setContact("")
          setOncall("")
          setTouched(false)
        }
      }}
    >
      <h3 id={id("title")} className={heading}>
        {owner ? `Edit ${owner.name}` : "Add an owner"}
      </h3>
      <div role="radiogroup" aria-label="Owner kind" className="grid grid-cols-2 gap-2">
        {kinds.map((k) => (
          <button
            key={k.value}
            type="button"
            role="radio"
            aria-checked={kind === k.value}
            onClick={() => setKind(k.value)}
            className={cn(
              "rounded-lg border px-3 py-1.5 text-left text-sm transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
              kind === k.value ? "border-primary bg-primary/5 ring-3 ring-primary/15" : "hover:bg-muted/60"
            )}
          >
            {k.label}
          </button>
        ))}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={id("name")}>Name</Label>
        <Input
          id={id("name")}
          value={name}
          onChange={(e) => setName(e.target.value)}
          onBlur={(e) => e.target.value && setTouched(true)}
          aria-invalid={touched && (!name.trim() || nameTaken)}
          placeholder={kind === "team" ? "e.g. Team Atlas" : "e.g. Alex Example"}
          maxLength={120}
        />
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={id("contact")}>Contact</Label>
          <Input
            id={id("contact")}
            value={contact}
            onChange={(e) => setContact(e.target.value)}
            placeholder="Email or chat channel"
            maxLength={200}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={id("oncall")}>On-call link</Label>
          <Input
            id={id("oncall")}
            type="url"
            value={oncall}
            onChange={(e) => setOncall(e.target.value)}
            placeholder="https://…"
            aria-invalid={oncallProblem !== null}
          />
        </div>
      </div>
      <div className="flex items-center justify-end gap-2">
        {touched && missing && <span className="mr-auto text-xs text-destructive">{missing}</span>}
        {owner && (
          <Button type="button" size="sm" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
        )}
        <Button type="submit" size="sm" disabled={pending}>
          {owner ? "Save owner" : "Add owner"}
        </Button>
      </div>
    </form>
  )
}
