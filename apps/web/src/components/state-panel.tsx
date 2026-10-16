import { CircleAlert, Inbox, LoaderCircle } from "lucide-react"
import { useReducedMotion } from "motion/react"

import { BorderBeam } from "@/components/ui/border-beam"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"

export type StateTone = "loading" | "empty" | "error"

/**
 * Loading, empty and error states of a page or panel: skeleton placeholders
 * while loading, a quiet card with a beam for "nothing here yet", and a
 * clear message with an optional retry for errors.
 */
export function StatePanel({
  tone = "empty",
  title,
  children,
  onRetry,
  action,
}: {
  tone?: StateTone
  title?: string
  children?: React.ReactNode
  /** Error states: shows a Retry button. */
  onRetry?: () => void
  /** Anything else the user can do next (a link or button). */
  action?: React.ReactNode
}) {
  const reducedMotion = useReducedMotion()
  if (tone === "loading") {
    return (
      <div role="status" aria-busy="true" className="m-auto flex w-full max-w-md flex-col gap-3 p-6">
        <span className="flex items-center gap-2 text-sm text-muted-foreground">
          <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" aria-hidden />
          {children ?? title ?? "Loading…"}
        </span>
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-5/6" />
        <Skeleton className="h-4 w-2/3" />
      </div>
    )
  }
  const Icon = tone === "error" ? CircleAlert : Inbox
  return (
    <div role={tone === "error" ? "alert" : "status"} className="m-auto w-full max-w-md p-6">
      <div className="relative flex flex-col items-center gap-3 rounded-2xl border bg-card/60 p-8 text-center">
        <span
          className={
            tone === "error"
              ? "flex size-11 items-center justify-center rounded-full bg-destructive/10 text-destructive"
              : "flex size-11 items-center justify-center rounded-full bg-muted text-muted-foreground"
          }
        >
          <Icon className="size-5" aria-hidden />
        </span>
        {title && <h2 className="font-heading text-base font-semibold">{title}</h2>}
        {children && (
          <div className={tone === "error" ? "text-sm text-destructive" : "text-sm text-muted-foreground"}>
            {children}
          </div>
        )}
        {(onRetry || action) && (
          <div className="flex items-center gap-2">
            {onRetry && (
              <Button size="sm" variant="outline" onClick={onRetry}>
                Retry
              </Button>
            )}
            {action}
          </div>
        )}
        {tone === "empty" && !reducedMotion && <BorderBeam size={90} duration={8} />}
      </div>
    </div>
  )
}
