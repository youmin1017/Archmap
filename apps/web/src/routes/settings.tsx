import { createFileRoute } from "@tanstack/react-router"
import { GitPullRequest, Radio } from "lucide-react"
import type { ReviewPolicy } from "@archmap/api-client"

import { AppTopBar } from "@/components/app-top-bar"
import { StatePanel } from "@/components/state-panel"
import { cn } from "@/lib/utils"
import { useMe } from "@/features/draft/queries"
import { useSettings, useUpdateSettings } from "@/features/settings/queries"

export const Route = createFileRoute("/settings")({ component: SettingsPage })

const heading = "text-[11px] font-medium tracking-wide text-muted-foreground uppercase"
const card =
  "flex flex-col items-start gap-1.5 rounded-lg border p-4 text-left transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:opacity-60"
const cardOn = "border-primary bg-primary/5 ring-3 ring-primary/15"

const policies: { value: ReviewPolicy; label: string; icon: typeof Radio; what: string; keeps: string }[] = [
  {
    value: "required",
    label: "Review required",
    icon: GitPullRequest,
    what: "Main is read-only. Every change is made in a draft, submitted, approved and merged by a maintainer.",
    keeps: "Best for teams that want four-eyes review of every change.",
  },
  {
    value: "direct",
    label: "Direct editing",
    icon: Radio,
    what: "Editors change main in place; every save is a new version that can be undone. Drafts stay available for larger changes.",
    keeps:
      "Protected elements still need an independent review, and edits that add lint errors are kept as drafts to fix.",
  },
]

/** Workspace settings (maintainers): today the review policy (ADR-072). */
function SettingsPage() {
  const me = useMe()
  const settings = useSettings()
  const update = useUpdateSettings()
  const isMaintainer = me.data?.role === "maintainer"
  const current = settings.data?.review_policy

  return (
    <div className="flex h-svh flex-col">
      <AppTopBar />
      <main className="mx-auto flex w-full max-w-3xl flex-1 flex-col gap-6 overflow-y-auto px-6 py-8">
        <div className="flex flex-col gap-1">
          <h1 className="font-heading text-2xl font-semibold tracking-tight">Settings</h1>
          <p className="text-sm text-muted-foreground">How the model changes. Settings apply to everyone at once.</p>
        </div>

        {(me.isPending || settings.isPending) && <StatePanel tone="loading">Loading…</StatePanel>}
        {settings.isError && (
          <StatePanel tone="error" title="Could not load the settings" onRetry={() => void settings.refetch()}>
            {String(settings.error.message).slice(0, 200)}
          </StatePanel>
        )}
        {me.isSuccess && !isMaintainer && (
          <StatePanel title="Maintainers only">
            You are a {me.data.role}. Ask a maintainer to change settings.
          </StatePanel>
        )}

        {isMaintainer && settings.isSuccess && (
          <section aria-labelledby="review-policy" className="flex flex-col gap-3">
            <h2 id="review-policy" className={heading}>
              Review policy
            </h2>
            <div role="radiogroup" aria-labelledby="review-policy" className="grid gap-3 sm:grid-cols-2">
              {policies.map((p) => {
                const on = current === p.value
                const Icon = p.icon
                return (
                  <button
                    key={p.value}
                    type="button"
                    role="radio"
                    aria-checked={on}
                    disabled={update.isPending}
                    onClick={() => !on && update.mutate(p.value)}
                    className={cn(card, on ? cardOn : "hover:bg-muted/50")}
                  >
                    <span className="flex items-center gap-2 text-sm font-medium">
                      <Icon className="size-4 text-muted-foreground" aria-hidden /> {p.label}
                    </span>
                    <span className="text-xs text-muted-foreground">{p.what}</span>
                    <span className="text-xs text-muted-foreground">{p.keeps}</span>
                  </button>
                )
              })}
            </div>
            {settings.data.updated_by && (
              <p className="text-xs text-muted-foreground">
                Last changed {new Date(settings.data.updated_at).toLocaleString()}.
              </p>
            )}
          </section>
        )}
      </main>
    </div>
  )
}
