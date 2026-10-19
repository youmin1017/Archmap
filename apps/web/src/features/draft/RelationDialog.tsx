import { useState } from "react"
import { ArrowRight } from "lucide-react"
import type { Relation, RelationKind } from "@archmap/api-client"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"

export type RelationFormValues = { kind: RelationKind; technology: string; description: string }

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Names shown in the title so the user knows what is being connected. */
  sourceName: string
  targetName: string
  /** Existing relation to edit; omit to create. */
  relation?: Relation
  pending?: boolean
  onSubmit: (values: RelationFormValues) => void
}

const kindItems: { value: RelationKind; label: string; hint: string }[] = [
  { value: "uses", label: "Uses", hint: "Calls, reads from or sends to" },
  { value: "deployed_on", label: "Deployed on", hint: "Runs on this node" },
]

/** Common protocols offered as one-click picks for the technology field. */
const protocols = ["HTTPS", "gRPC", "SQL", "AMQP", "TCP", "WebSocket"]

const endpoint = "min-w-0 truncate rounded-md border bg-muted/40 px-2 py-1 text-sm font-medium"

export function RelationDialog(props: Props) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {props.open && <RelationForm key={props.relation?.id ?? "new"} {...props} />}
      </DialogContent>
    </Dialog>
  )
}

function RelationForm({ sourceName, targetName, relation, pending, onOpenChange, onSubmit }: Props) {
  const [kind, setKind] = useState<RelationKind>(relation?.kind ?? "uses")
  const [technology, setTechnology] = useState(relation?.technology ?? "")
  const [description, setDescription] = useState(relation?.description ?? "")

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!pending) onSubmit({ kind, technology: technology.trim(), description })
      }}
    >
      <DialogHeader>
        <DialogTitle>{relation ? "Edit relation" : "Connect elements"}</DialogTitle>
        <DialogDescription className="sr-only">
          {sourceName} to {targetName}
        </DialogDescription>
      </DialogHeader>

      <div className="flex items-center gap-2" aria-hidden>
        <span className={endpoint}>{sourceName}</span>
        <ArrowRight className="size-4 shrink-0 text-muted-foreground" />
        <span className={endpoint}>{targetName}</span>
      </div>

      <fieldset className="flex flex-col gap-1.5">
        <legend className="mb-1.5 text-sm leading-none font-medium">Kind</legend>
        <div className="grid grid-cols-2 gap-1.5" role="radiogroup" aria-label="Relation kind">
          {kindItems.map((k) => {
            const on = k.value === kind
            return (
              <button
                key={k.value}
                type="button"
                role="radio"
                aria-checked={on}
                onClick={() => setKind(k.value)}
                className={cn(
                  "flex flex-col items-start gap-0.5 rounded-lg border px-3 py-2 text-left transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
                  on ? "border-primary bg-primary/5 ring-3 ring-primary/15" : "hover:bg-muted/60"
                )}
              >
                <span className="text-sm font-medium">{k.label}</span>
                <span className="text-xs text-muted-foreground">{k.hint}</span>
              </button>
            )
          })}
        </div>
      </fieldset>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="rel-technology">Technology</Label>
        <Input
          id="rel-technology"
          value={technology}
          onChange={(e) => setTechnology(e.target.value)}
          placeholder="e.g. HTTPS, gRPC, SQL"
          autoFocus
        />
        <div className="flex flex-wrap gap-1" role="group" aria-label="Common protocols">
          {protocols.map((p) => (
            <button
              key={p}
              type="button"
              aria-pressed={technology === p}
              onClick={() => setTechnology(p)}
              className="inline-flex h-6 items-center rounded-md border px-2 font-mono text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none aria-pressed:border-primary/40 aria-pressed:bg-primary/10 aria-pressed:text-foreground"
            >
              {p}
            </button>
          ))}
        </div>
        {kind === "uses" && technology.trim() === "" && (
          <p className="text-xs text-muted-foreground">Lint warns about “uses” relations without a technology.</p>
        )}
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="rel-description">Description</Label>
        <Textarea id="rel-description" value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button type="submit" disabled={pending}>
          {relation ? "Save changes" : "Connect"}
        </Button>
      </DialogFooter>
    </form>
  )
}
