import { Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { getHealth } from "@archmap/api-client"
import { GitPullRequest, History, LayoutGrid, Network, Settings, Users } from "lucide-react"

import "@/lib/api"
import { ThemeToggle } from "@/components/theme-toggle"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { useMe } from "@/features/draft/queries"
import { UserMenu } from "@/features/auth/UserMenu"

const navLink =
  "inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none aria-[current=page]:bg-muted aria-[current=page]:text-foreground [&_svg]:size-3.5"

/** API health as a status dot; the details are in the tooltip. */
function ApiStatus() {
  const health = useQuery({
    queryKey: ["health"],
    queryFn: async () => (await getHealth({ throwOnError: true })).data,
  })
  const label = health.isPending
    ? "API: checking…"
    : health.isError
      ? "API unreachable"
      : `API: ${health.data.status} · ${health.data.version}`
  return (
    <Tooltip>
      <TooltipTrigger
        render={<span role="status" aria-live="polite" aria-label={label} />}
        className="hidden size-7 items-center justify-center sm:inline-flex"
      >
        <span
          className={cn(
            "size-2 rounded-full",
            health.isPending && "animate-pulse bg-muted-foreground/50",
            health.isError && "bg-destructive",
            health.isSuccess && "bg-emerald-500 shadow-[0_0_0_3px] shadow-emerald-500/20"
          )}
        />
      </TooltipTrigger>
      <TooltipContent side="bottom">{label}</TooltipContent>
    </Tooltip>
  )
}

/** Persistent top bar: brand, navigation, current context (breadcrumb), API status, user, theme. */
export function AppTopBar({ context }: { context?: React.ReactNode }) {
  const me = useMe()

  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b bg-background px-3">
      <Link
        to="/"
        className="flex items-center gap-2 rounded-md pr-1 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
      >
        <span className="grid size-7 place-items-center rounded-lg bg-linear-to-br from-primary to-[oklch(0.55_0.2_300)] text-primary-foreground shadow-sm">
          <Network className="size-4" aria-hidden />
        </span>
        <span className="hidden font-heading text-[15px] font-semibold tracking-tight sm:inline">Archmap</span>
      </Link>

      <span className="mx-1 h-5 w-px bg-border" aria-hidden />

      <nav aria-label="Main" className="flex items-center gap-0.5">
        <Link to="/" activeOptions={{ exact: true }} className={navLink}>
          <LayoutGrid aria-hidden /> <span className="hidden md:inline">Main</span>
        </Link>
        <Link to="/changesets" className={navLink}>
          <GitPullRequest aria-hidden /> <span className="hidden md:inline">Changesets</span>
        </Link>
        <Link to="/versions" className={navLink}>
          <History aria-hidden /> <span className="hidden md:inline">Versions</span>
        </Link>
        {me.data?.role === "maintainer" && (
          <>
            <Link to="/users" className={navLink}>
              <Users aria-hidden /> <span className="hidden md:inline">Users</span>
            </Link>
            <Link to="/settings" className={navLink}>
              <Settings aria-hidden /> <span className="hidden md:inline">Settings</span>
            </Link>
          </>
        )}
      </nav>

      {context && (
        <div className="flex min-w-0 items-center gap-2 text-sm" aria-label="Current context">
          <span className="text-muted-foreground/50 select-none" aria-hidden>
            /
          </span>
          {context}
        </div>
      )}

      <div className="ml-auto flex items-center gap-1.5">
        <ApiStatus />
        <ThemeToggle />
        <UserMenu />
      </div>
    </header>
  )
}
