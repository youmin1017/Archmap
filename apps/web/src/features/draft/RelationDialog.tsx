import { useState } from "react"
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"

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

const kindItems: { value: RelationKind; label: string }[] = [
  { value: "uses", label: "uses" },
  { value: "deployed_on", label: "deployed on" },
]

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
        <DialogDescription>
          {sourceName} → {targetName}
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="rel-kind">Kind</Label>
        <Select value={kind} onValueChange={(v) => setKind(v as RelationKind)} items={kindItems}>
          <SelectTrigger id="rel-kind" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {kindItems.map((k) => (
              <SelectItem key={k.value} value={k.value}>
                {k.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="rel-technology">Technology</Label>
        <Input
          id="rel-technology"
          value={technology}
          onChange={(e) => setTechnology(e.target.value)}
          placeholder="e.g. HTTPS, gRPC, SQL"
          autoFocus
        />
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
