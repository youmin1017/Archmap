import { useState } from "react"
import { Check, MoreHorizontal, Plus, Search, X } from "lucide-react"
import type { Group, User } from "@archmap/api-client"
import { toast } from "sonner"

import { StatePanel } from "@/components/state-panel"
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import { useCreateGroup, useDeleteGroup, useGroups, useSetGroupMembers, useUpdateGroup } from "./queries"

const heading = "text-[11px] font-medium tracking-wide text-muted-foreground uppercase"

/**
 * Groups of users that can own elements (ADR-070). They are account administration:
 * edits apply at once and are not versioned; assigning a group to an element is a draft change.
 */
export function GroupsSection({ users }: { users: readonly User[] }) {
  const groups = useGroups()
  const remove = useDeleteGroup()
  const [editing, setEditing] = useState<Group | "new" | null>(null)
  const [removing, setRemoving] = useState<Group | null>(null)
  const nameOf = new Map(users.map((u) => [u.id, u.name]))

  return (
    <section aria-labelledby="groups-heading" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-col gap-1">
          <h2 id="groups-heading" className="text-lg font-semibold tracking-tight">
            Groups
          </h2>
          <p className="text-sm text-muted-foreground">
            Groups can own elements. Membership changes apply at once and are not versioned.
          </p>
        </div>
        <Button size="sm" variant="outline" onClick={() => setEditing("new")}>
          <Plus aria-hidden /> New group
        </Button>
      </div>

      {groups.isPending && <StatePanel tone="loading">Loading groups…</StatePanel>}
      {groups.isError && (
        <StatePanel tone="error" title="Could not load groups" onRetry={() => void groups.refetch()}>
          {String(groups.error.message).slice(0, 200)}
        </StatePanel>
      )}
      {groups.isSuccess && groups.data.length === 0 && (
        <StatePanel title="No groups yet">Create a group to make a set of users an owner.</StatePanel>
      )}
      {groups.isSuccess && groups.data.length > 0 && (
        <div className="overflow-x-auto rounded-lg border bg-card">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                <TableHead>Name</TableHead>
                <TableHead>Members</TableHead>
                <TableHead className="hidden md:table-cell">Description</TableHead>
                <TableHead className="w-10">
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.data.map((g) => {
                const names = g.members.map((id) => nameOf.get(id) ?? "Unknown user")
                return (
                  <TableRow key={g.id}>
                    <TableCell className="font-medium">{g.name}</TableCell>
                    <TableCell>
                      {names.length === 0 ? (
                        <span className="text-muted-foreground">None</span>
                      ) : (
                        <span className="flex flex-wrap gap-1" title={names.join(", ")}>
                          {names.slice(0, 3).map((n, i) => (
                            <span key={i} className="rounded bg-muted px-1.5 py-0.5 text-[11px]">
                              {n}
                            </span>
                          ))}
                          {names.length > 3 && (
                            <span className="text-[11px] text-muted-foreground">+{names.length - 3}</span>
                          )}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="hidden max-w-72 truncate text-muted-foreground md:table-cell">
                      {g.description || "—"}
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={<Button size="icon-sm" variant="ghost" aria-label={`Actions for ${g.name}`} />}
                        >
                          <MoreHorizontal aria-hidden />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-auto min-w-40">
                          <DropdownMenuItem onClick={() => setEditing(g)}>Edit group…</DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem variant="destructive" onClick={() => setRemoving(g)}>
                            Delete…
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog open={editing !== null} onOpenChange={(o) => !o && setEditing(null)}>
        <DialogContent className="sm:max-w-lg">
          {editing !== null && (
            <GroupForm
              group={editing === "new" ? null : editing}
              users={users}
              taken={(groups.data ?? [])
                .filter((g) => editing === "new" || g.id !== editing.id)
                .map((g) => g.name.trim().toLowerCase())}
              onDone={() => setEditing(null)}
            />
          )}
        </DialogContent>
      </Dialog>

      <AlertDialog open={removing !== null} onOpenChange={(o) => !o && setRemoving(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {removing?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              Its members keep their accounts. Elements it owns keep the assignment under its last name, and lint warns
              until a draft assigns a new owner.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (removing) remove.mutate(removing.id)
                setRemoving(null)
              }}
            >
              Delete group
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}

/** Create or edit a group: name, description and members in one form. */
function GroupForm({
  group,
  users,
  taken,
  onDone,
}: {
  group: Group | null
  users: readonly User[]
  taken: readonly string[]
  onDone: () => void
}) {
  const create = useCreateGroup()
  const update = useUpdateGroup()
  const setMembers = useSetGroupMembers()
  const [name, setName] = useState(group?.name ?? "")
  const [description, setDescription] = useState(group?.description ?? "")
  const [members, setMembersState] = useState<string[]>(() => [...(group?.members ?? [])])
  const [filter, setFilter] = useState("")
  const [touched, setTouched] = useState(false)

  const nameTaken = taken.includes(name.trim().toLowerCase())
  const missing = !name.trim() ? "Give the group a name." : nameTaken ? "That name is taken." : null
  const pending = create.isPending || update.isPending || setMembers.isPending
  const byId = new Map(users.map((u) => [u.id, u]))
  const q = filter.trim().toLowerCase()
  const candidates = users.filter(
    (u) => (!u.disabled || members.includes(u.id)) && (!q || `${u.name} ${u.email}`.toLowerCase().includes(q))
  )
  const toggle = (id: string) =>
    setMembersState((cur) => (cur.includes(id) ? cur.filter((m) => m !== id) : [...cur, id]))

  async function save() {
    const v = { name: name.trim(), description: description.trim() }
    const id = group ? (await update.mutateAsync({ id: group.id, ...v })).id : (await create.mutateAsync(v)).id
    const same = group && group.members.length === members.length && members.every((m) => group.members.includes(m))
    if (!same) await setMembers.mutateAsync({ id, userIds: members })
    toast.success(group ? "Group saved" : "Group created")
    onDone()
  }

  return (
    <form
      className="flex max-h-[85vh] min-h-0 flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        setTouched(true)
        if (missing || pending) return
        save().catch(() => {}) // errors are toasted by the mutations
      }}
    >
      <DialogHeader>
        <DialogTitle>{group ? `Edit ${group.name}` : "New group"}</DialogTitle>
        <DialogDescription>Elements can be assigned to the group as a whole in a draft.</DialogDescription>
      </DialogHeader>

      <div className="-mx-4 flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-4 pb-1">
        <section className="flex flex-col gap-3" aria-labelledby="grp-details">
          <h3 id="grp-details" className={heading}>
            Group
          </h3>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="grp-name">Name</Label>
            <Input
              id="grp-name"
              value={name}
              maxLength={120}
              onChange={(e) => setName(e.target.value)}
              onBlur={() => setTouched(true)}
              aria-invalid={touched && missing !== null}
              aria-describedby="grp-name-hint"
              autoFocus
            />
            <p
              id="grp-name-hint"
              className={cn("text-xs", touched && missing ? "text-destructive" : "text-muted-foreground")}
            >
              {touched && missing ? missing : "Unique, for example Payments Team."}
            </p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="grp-description">Description</Label>
            <Textarea
              id="grp-description"
              value={description}
              maxLength={500}
              rows={2}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
        </section>

        <section className="flex flex-col gap-3" aria-labelledby="grp-members">
          <h3 id="grp-members" className={heading}>
            Members ({members.length})
          </h3>
          {members.length > 0 && (
            <ul className="flex flex-wrap gap-1" aria-label="Selected members">
              {members.map((id) => {
                const u = byId.get(id)
                return (
                  <li
                    key={id}
                    className="inline-flex items-center gap-1 rounded bg-muted py-0.5 pr-0.5 pl-1.5 text-[11px]"
                  >
                    {u?.name ?? "Unknown user"}
                    <button
                      type="button"
                      className="rounded-sm p-0.5 text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                      aria-label={`Remove ${u?.name ?? "member"}`}
                      onClick={() => toggle(id)}
                    >
                      <X className="size-3" aria-hidden />
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
          <div className="relative">
            <Search
              className="pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground"
              aria-hidden
            />
            <Input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Find users"
              aria-label="Find users"
              className="pl-7"
            />
          </div>
          <ul className="flex max-h-56 flex-col overflow-y-auto rounded-lg border" aria-label="Users">
            {candidates.length === 0 && <li className="px-3 py-2 text-sm text-muted-foreground">No matching users.</li>}
            {candidates.map((u) => {
              const on = members.includes(u.id)
              return (
                <li key={u.id}>
                  <button
                    type="button"
                    aria-pressed={on}
                    onClick={() => toggle(u.id)}
                    className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-muted/60 focus-visible:bg-muted/60 focus-visible:outline-none"
                  >
                    <span
                      className={cn(
                        "flex size-4 shrink-0 items-center justify-center rounded border",
                        on && "border-primary bg-primary text-primary-foreground"
                      )}
                      aria-hidden
                    >
                      {on && <Check className="size-3" />}
                    </span>
                    <span className={cn("truncate", u.disabled && "text-muted-foreground line-through")}>{u.name}</span>
                    <span className="ml-auto truncate text-xs text-muted-foreground">
                      {u.disabled ? "Disabled" : u.email}
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        </section>
      </div>

      <DialogFooter className="items-center">
        <p className="mr-auto text-xs text-muted-foreground" aria-live="polite">
          {missing ?? (members.length === 0 ? "No members yet; you can add them later." : "")}
        </p>
        <Button type="button" variant="outline" size="sm" onClick={onDone}>
          Cancel
        </Button>
        <Button type="submit" size="sm" disabled={missing !== null || pending}>
          {group ? "Save group" : "Create group"}
        </Button>
      </DialogFooter>
    </form>
  )
}
