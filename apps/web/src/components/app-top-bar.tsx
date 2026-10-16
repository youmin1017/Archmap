import { Link } from "@tanstack/react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getHealth } from "@archmap/api-client"
import { Network } from "lucide-react"

import "@/lib/api"
import { ThemeToggle } from "@/components/theme-toggle"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { devUsers, setDevUser } from "@/lib/dev-user"
import { useMe } from "@/features/draft/queries"

const navLink =
  "rounded-lg px-2.5 py-1 text-sm text-muted-foreground transition-colors hover:text-foreground aria-[current=page]:bg-accent aria-[current=page]:text-foreground"

/** Dev-only: switch between the seeded users (viewer / editor / maintainer). */
function DevUserSwitcher() {
  const qc = useQueryClient()
  const me = useMe()
  // Only server data here: reading localStorage during render would differ between SSR and hydration.
  const current = me.data?.email ?? ""
  return (
    <Select
      value={current}
      onValueChange={(email) => {
        if (!email) return
        setDevUser(email)
        void qc.invalidateQueries()
      }}
      items={devUsers.map((u) => ({ value: u, label: u.split("@")[0] ?? u }))}
    >
      <SelectTrigger size="sm" aria-label="Acting as (dev only)" className="w-36">
        <SelectValue placeholder="user" />
      </SelectTrigger>
      <SelectContent>
        {devUsers.map((u) => (
          <SelectItem key={u} value={u}>
            {u.split("@")[0]}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** Persistent top bar: brand, navigation, current context, API status, user, theme. */
export function AppTopBar({ context }: { context?: React.ReactNode }) {
  const me = useMe()
  const health = useQuery({
    queryKey: ["health"],
    queryFn: async () => (await getHealth({ throwOnError: true })).data,
  })

  return (
    <header className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-4 py-2">
      <Link to="/" className="flex items-center gap-2">
        <Network className="size-5 text-primary" aria-hidden />
        <span className="font-heading text-lg font-semibold">Archmap</span>
      </Link>
      <nav aria-label="Main" className="flex items-center gap-1">
        <Link to="/" activeOptions={{ exact: true }} className={navLink}>
          Main
        </Link>
        <Link to="/changesets" className={navLink}>
          Changesets
        </Link>
        <Link to="/versions" className={navLink}>
          Versions
        </Link>
        {me.data?.role === "maintainer" && (
          <Link to="/users" className={navLink}>
            Users
          </Link>
        )}
      </nav>
      {context && <div className="flex items-center gap-2">{context}</div>}
      <div className="ml-auto flex items-center gap-3">
        <span className="hidden text-xs text-muted-foreground sm:inline" aria-live="polite">
          {health.isPending && "API: checking…"}
          {health.isError && "API unreachable"}
          {health.isSuccess && `API: ${health.data.status} · ${health.data.version}`}
        </span>
        {import.meta.env.DEV && <DevUserSwitcher />}
        <ThemeToggle />
      </div>
    </header>
  )
}
