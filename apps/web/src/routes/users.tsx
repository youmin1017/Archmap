import { createFileRoute } from "@tanstack/react-router"
import { Users } from "lucide-react"
import type { Role } from "@archmap/api-client"

import { AppTopBar } from "@/components/app-top-bar"
import { StatePanel } from "@/components/state-panel"
import { Badge } from "@/components/ui/badge"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useMe } from "@/features/draft/queries"
import { useSetUserRole, useUsers } from "@/features/users/queries"

export const Route = createFileRoute("/users")({ component: UsersPage })

const roles: { value: Role; label: string; can: string }[] = [
  { value: "viewer", label: "Viewer", can: "Browse main, drafts, views and versions." },
  { value: "editor", label: "Editor", can: "Also create and edit drafts, and submit them for review." },
  {
    value: "maintainer",
    label: "Maintainer",
    can: "Also approve and merge, change protected elements, layer rules and users.",
  },
]

/** Maintainers manage who is a viewer, editor or maintainer. */
function UsersPage() {
  const me = useMe()
  const isMaintainer = me.data?.role === "maintainer"
  const users = useUsers(isMaintainer)
  const setRole = useSetUserRole()

  return (
    <div className="flex h-svh flex-col">
      <AppTopBar
        context={
          <Badge variant="secondary">
            <Users data-icon="inline-start" aria-hidden /> Users
          </Badge>
        }
      />
      <main className="mx-auto flex w-full max-w-4xl flex-1 flex-col gap-6 overflow-y-auto p-6">
        <h1 className="font-heading text-xl font-semibold">Users and roles</h1>

        <section aria-label="What each role can do" className="grid gap-3 sm:grid-cols-3">
          {roles.map((r) => (
            <div key={r.value} className="rounded-xl border p-3">
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
          <div className="rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead className="w-44">Role</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.data.map((u) => (
                  <TableRow key={u.id}>
                    <TableCell className="font-medium">
                      {u.name}
                      {u.id === me.data?.id && <span className="ml-2 text-xs text-muted-foreground">(you)</span>}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{u.email}</TableCell>
                    <TableCell>
                      <Select
                        value={u.role}
                        onValueChange={(role) => {
                          if (role && role !== u.role) setRole.mutate({ id: u.id, role: role })
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
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </main>
    </div>
  )
}
