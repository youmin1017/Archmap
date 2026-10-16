import type { LucideIcon } from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

export type MenuEntry =
  | {
      type?: "item"
      label: string
      icon?: LucideIcon
      onSelect: () => void
      disabled?: boolean
      destructive?: boolean
    }
  | { type: "submenu"; label: string; icon?: LucideIcon; entries: MenuEntry[]; disabled?: boolean }
  | { type: "separator" }

/** Joins entry groups with separators between non-empty groups. */
export function joinGroups(...groups: MenuEntry[][]): MenuEntry[] {
  const out: MenuEntry[] = []
  for (const g of groups) {
    if (g.length === 0) continue
    if (out.length > 0) out.push({ type: "separator" })
    out.push(...g)
  }
  return out
}

export type MenuPosition = { x: number; y: number }

/**
 * Context menu opened at a screen position (right click). Built on the shadcn
 * dropdown menu, anchored to an invisible point, so it gets keyboard
 * navigation (arrows, Enter, Escape), submenus and focus handling for free.
 */
export function CanvasContextMenu({
  position,
  entries,
  onClose,
}: {
  position: MenuPosition | null
  entries: MenuEntry[]
  onClose: () => void
}) {
  return (
    <DropdownMenu open={position !== null && entries.length > 0} onOpenChange={(open) => !open && onClose()}>
      <DropdownMenuTrigger
        nativeButton={false}
        render={
          <span
            aria-hidden
            style={{
              position: "fixed",
              left: position?.x ?? 0,
              top: position?.y ?? 0,
              width: 1,
              height: 1,
              pointerEvents: "none",
            }}
          />
        }
      />
      <DropdownMenuContent align="start" side="bottom" sideOffset={0} className="w-auto">
        <Entries entries={entries} />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function Entries({ entries }: { entries: MenuEntry[] }) {
  return entries.map((entry, i) => {
    if (entry.type === "separator") return <DropdownMenuSeparator key={`sep-${i}`} />
    const Icon = entry.icon
    if (entry.type === "submenu") {
      return (
        <DropdownMenuSub key={entry.label}>
          <DropdownMenuSubTrigger disabled={entry.disabled}>
            {Icon && <Icon aria-hidden />}
            {entry.label}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <Entries entries={entry.entries} />
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      )
    }
    return (
      <DropdownMenuItem
        key={entry.label}
        disabled={entry.disabled}
        variant={entry.destructive ? "destructive" : "default"}
        onClick={entry.onSelect}
      >
        {Icon && <Icon aria-hidden />}
        {entry.label}
      </DropdownMenuItem>
    )
  })
}
