import { useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { KeyRound, LogOut } from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { ChangePasswordDialog } from "./ChangePasswordDialog"
import { useLogout, useSession } from "./queries"

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  return ((parts[0]?.[0] ?? "?") + (parts.length > 1 ? (parts.at(-1)?.[0] ?? "") : "")).toUpperCase()
}

/** The signed-in user: who, which role, change password (local sign-in), sign out. */
export function UserMenu() {
  const session = useSession()
  const signOut = useLogout()
  const navigate = useNavigate()
  const [changing, setChanging] = useState(false)
  const me = session.data?.me
  if (!me) return null

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          className="inline-flex h-7 items-center gap-1.5 rounded-md pr-1.5 pl-0.5 text-sm hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          aria-label={`Account: ${me.name}`}
        >
          <span className="grid size-6 place-items-center rounded-md bg-muted text-[11px] font-medium" aria-hidden>
            {initials(me.name)}
          </span>
          <span className="hidden max-w-32 truncate lg:inline">{me.name}</span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuGroup>
            <DropdownMenuLabel className="flex flex-col gap-0.5">
              <span className="truncate text-sm font-medium text-foreground">{me.name}</span>
              {me.email && <span className="truncate text-xs font-normal">{me.email}</span>}
              <span className="mt-1 flex gap-1">
                <span className="rounded bg-muted px-1.5 text-[11px] font-normal text-foreground capitalize">
                  {me.role}
                </span>
                <span className="rounded bg-muted px-1.5 text-[11px] font-normal">via {me.provider}</span>
              </span>
            </DropdownMenuLabel>
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          {me.provider === "local" && (
            <DropdownMenuItem onClick={() => setChanging(true)}>
              <KeyRound aria-hidden /> Change password
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            onClick={() => signOut.mutate(undefined, { onSuccess: () => void navigate({ to: "/login", search: {} }) })}
          >
            <LogOut aria-hidden /> Sign out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ChangePasswordDialog open={changing} onOpenChange={setChanging} />
    </>
  )
}
