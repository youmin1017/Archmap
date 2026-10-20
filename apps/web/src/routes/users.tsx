import { useState } from "react"
import { createFileRoute } from "@tanstack/react-router"
import { MoreHorizontal, UserPlus } from "lucide-react"
import type { User } from "@archmap/api-client"

import { AppTopBar } from "@/components/app-top-bar"
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
import { Dialog, DialogContent } from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { cn } from "@/lib/utils"
import { useMe } from "@/features/draft/queries"
import { GroupsSection } from "@/features/users/GroupsSection"
import { IdentitiesDialog } from "@/features/users/IdentitiesDialog"
import { NewUserDialog, OneTimePassword } from "@/features/users/NewUserDialog"
import { useResetPassword, useSetDisabled, useSetUserRole, useUsers } from "@/features/users/queries"
import { roles } from "@/features/users/roles"

export const Route = createFileRoute("/users")({ component: UsersPage })

type Confirm = { kind: "disable" | "reset"; user: User }

function lastSignIn(t?: string | null) {
  if (!t) return "Never"
  return new Date(t).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" })
}

/** Maintainers manage accounts: roles, sign-in identities, passwords and who may sign in. */
function UsersPage() {
  const me = useMe()
  const isMaintainer = me.data?.role === "maintainer"
  const users = useUsers(isMaintainer)
  const setRole = useSetUserRole()
  const setDisabled = useSetDisabled()
  const reset = useResetPassword()
  const [creating, setCreating] = useState(false)
  const [identitiesOf, setIdentitiesOf] = useState<User | null>(null)
  const [confirm, setConfirm] = useState<Confirm | null>(null)
  const [temporary, setTemporary] = useState<{ user: User; password: string } | null>(null)

  return (
    <div className="flex h-svh flex-col">
      <AppTopBar />
      <main className="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-6 overflow-y-auto px-6 py-8">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div className="flex flex-col gap-1">
            <h1 className="font-heading text-2xl font-semibold tracking-tight">Users and roles</h1>
            <p className="text-sm text-muted-foreground">Roles decide who can view, draft, review and merge changes.</p>
          </div>
          {isMaintainer && (
            <Button size="sm" onClick={() => setCreating(true)}>
              <UserPlus aria-hidden /> New user
            </Button>
          )}
        </div>

        <section aria-label="What each role can do" className="grid gap-3 sm:grid-cols-3">
          {roles.map((r) => (
            <div key={r.value} className="rounded-lg border bg-card p-4">
              <h2 className="text-sm font-medium">{r.label}</h2>
              <p className="mt-1 text-xs text-muted-foreground">{r.can}</p>
            </div>
          ))}
        </section>

        {me.isPending && <StatePanel tone="loading">Loading…</StatePanel>}
        {me.isSuccess && !isMaintainer && (
          <StatePanel title="Maintainers only">
            You are a {me.data.role}. Ask a maintainer to change your role.
          </StatePanel>
        )}
        {isMaintainer && users.isPending && <StatePanel tone="loading">Loading users…</StatePanel>}
        {isMaintainer && users.isError && (
          <StatePanel tone="error" title="Could not load users" onRetry={() => void users.refetch()}>
            {String(users.error.message).slice(0, 200)}
          </StatePanel>
        )}
        {isMaintainer && users.isSuccess && users.data.length === 0 && <StatePanel title="No users" />}
        {isMaintainer && users.isSuccess && users.data.length > 0 && (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/40 hover:bg-muted/40">
                  <TableHead>Name</TableHead>
                  <TableHead className="hidden md:table-cell">Email</TableHead>
                  <TableHead className="hidden sm:table-cell">Last sign-in</TableHead>
                  <TableHead className="w-40">Role</TableHead>
                  <TableHead className="w-10">
                    <span className="sr-only">Actions</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.data.map((u) => {
                  const self = u.id === me.data?.id
                  return (
                    <TableRow key={u.id} className={cn(u.disabled && "text-muted-foreground")}>
                      <TableCell className="font-medium">
                        <span className="flex items-center gap-2">
                          {u.name}
                          {self && <span className="text-xs font-normal text-muted-foreground">(you)</span>}
                          {u.disabled && (
                            <span className="inline-flex h-5 items-center rounded-full bg-muted px-2 text-xs font-medium text-muted-foreground">
                              Disabled
                            </span>
                          )}
                        </span>
                      </TableCell>
                      <TableCell className="hidden text-muted-foreground md:table-cell">{u.email || "—"}</TableCell>
                      <TableCell className="hidden text-muted-foreground sm:table-cell">
                        {lastSignIn(u.last_login_at)}
                      </TableCell>
                      <TableCell>
                        <Select
                          value={u.role}
                          onValueChange={(role) => {
                            if (role && role !== u.role) setRole.mutate({ id: u.id, role })
                          }}
                          items={roles.map((r) => ({ value: r.value, label: r.label }))}
                          disabled={setRole.isPending}
                        >
                          <SelectTrigger size="sm" className="w-36" aria-label={`Role of ${u.name}`}>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {roles.map((r) => (
                              <SelectItem key={r.value} value={r.value}>
                                {r.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </TableCell>
                      <TableCell>
                        <DropdownMenu>
                          <DropdownMenuTrigger
                            render={<Button size="icon-sm" variant="ghost" aria-label={`Actions for ${u.name}`} />}
                          >
                            <MoreHorizontal aria-hidden />
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end" className="w-auto min-w-48">
                            <DropdownMenuItem onClick={() => setIdentitiesOf(u)}>
                              Sign-in identities ({u.identity_count})
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => setConfirm({ kind: "reset", user: u })}>
                              Reset password…
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            {u.disabled ? (
                              <DropdownMenuItem onClick={() => setDisabled.mutate({ id: u.id, disabled: false })}>
                                Enable
                              </DropdownMenuItem>
                            ) : (
                              <DropdownMenuItem
                                variant="destructive"
                                disabled={self}
                                onClick={() => setConfirm({ kind: "disable", user: u })}
                              >
                                Disable…
                              </DropdownMenuItem>
                            )}
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
        {isMaintainer && users.isSuccess && <GroupsSection users={users.data} />}
      </main>

      <NewUserDialog open={creating} onOpenChange={setCreating} />
      <IdentitiesDialog user={identitiesOf} onClose={() => setIdentitiesOf(null)} />

      <AlertDialog open={confirm !== null} onOpenChange={(o) => !o && setConfirm(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirm?.kind === "disable"
                ? `Disable ${confirm.user.name}?`
                : `Reset the password of ${confirm?.user.name}?`}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirm?.kind === "disable"
                ? "They are signed out now and cannot sign in until enabled again. Their drafts and history stay."
                : "A temporary password is generated and shown once. They are signed out now and must choose a new password at their next sign-in."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant={confirm?.kind === "disable" ? "destructive" : "default"}
              onClick={() => {
                if (!confirm) return
                const { kind, user } = confirm
                setConfirm(null)
                if (kind === "disable") setDisabled.mutate({ id: user.id, disabled: true })
                else
                  reset.mutate(user.id, {
                    onSuccess: (res) =>
                      res.temporary_password && setTemporary({ user, password: res.temporary_password }),
                  })
              }}
            >
              {confirm?.kind === "disable" ? "Disable" : "Reset password"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={temporary !== null} onOpenChange={(o) => !o && setTemporary(null)}>
        <DialogContent className="sm:max-w-lg">
          {temporary && (
            <OneTimePassword
              title={`New password for ${temporary.user.name}`}
              who={temporary.user.email}
              password={temporary.password}
              onDone={() => setTemporary(null)}
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
