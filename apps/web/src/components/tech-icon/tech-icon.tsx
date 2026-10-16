import { Icon } from "@iconify/react"
import { Box } from "lucide-react"

import { cn } from "@/lib/utils"
import { resolveTechIcon } from "./resolve"

export { resolveTechIcon } from "./resolve"

type Props = {
  technology: string
  className?: string
  /** URL of a user-uploaded custom icon (highest priority; wired up in M6). */
  customIconUrl?: string
}

/**
 * Technology badge icon. Resolution order (brief section 2): custom upload,
 * simple-icons (monochrome, currentColor), colored logos, generic lucide icon.
 */
export function TechIcon({ technology, className, customIconUrl }: Props) {
  if (customIconUrl) {
    return <img src={customIconUrl} alt={technology} className={cn("object-contain", className)} />
  }
  const resolved = resolveTechIcon(technology)
  if (!resolved) return <Box className={className} aria-label={technology} role="img" />
  return (
    <Icon
      icon={resolved.data}
      className={cn(resolved.lowContrastOnDark && "dark:brightness-0 dark:invert", className)}
      aria-label={technology}
      role="img"
    />
  )
}

/** Icon + text chip for an element's technology. */
export function TechBadge({ technology, className }: { technology: string; className?: string }) {
  if (!technology.trim()) return null
  return (
    <span
      className={cn(
        "inline-flex max-w-full items-center gap-1 rounded-full border bg-background/70 px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground",
        className
      )}
    >
      <TechIcon technology={technology} className="size-3 shrink-0" />
      <span className="truncate">{technology}</span>
    </span>
  )
}
