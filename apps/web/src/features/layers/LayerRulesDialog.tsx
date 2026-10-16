import { useState } from "react"
import type { LayerRule } from "@archmap/api-client"

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
import { parseTags } from "../draft/kindRules"
import { ruleClassLabel, ruleClasses } from "./layers"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  rules: readonly LayerRule[]
  /** The view's layers, shown as a hint. */
  layers: readonly string[]
  pending?: boolean
  onSave: (rules: LayerRule[]) => void
}

/** Maintainers: which layers each element class may be placed on. */
export function LayerRulesDialog(props: Props) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">{props.open && <RulesForm {...props} />}</DialogContent>
    </Dialog>
  )
}

function RulesForm({ rules, layers, pending, onOpenChange, onSave }: Props) {
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(ruleClasses.map((c) => [c, rules.find((r) => r.element_class === c)?.layers.join(", ") ?? ""]))
  )
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        onSave(
          ruleClasses
            .map((c) => ({ element_class: c, layers: parseTags(values[c]) }))
            .filter((r) => r.layers.length > 0)
        )
      }}
    >
      <DialogHeader>
        <DialogTitle>Layer rules</DialogTitle>
        <DialogDescription>
          Limit where an element class may be placed. Leave a class empty to allow every layer. Existing elements are
          not changed; edits that break a rule are rejected.
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-3">
        {ruleClasses.map((c) => (
          <div key={c} className="grid grid-cols-[6.5rem_1fr] items-center gap-2">
            <Label htmlFor={`rule-${c}`}>{ruleClassLabel(c)}</Label>
            <Input
              id={`rule-${c}`}
              value={values[c]}
              onChange={(e) => setValues((v) => ({ ...v, [c]: e.target.value }))}
              placeholder="any layer"
            />
          </div>
        ))}
      </div>
      {layers.length > 0 && (
        <p className="text-xs text-muted-foreground">Comma separated. Layers of this view: {layers.join(", ")}.</p>
      )}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button type="submit" disabled={pending}>
          Save rules
        </Button>
      </DialogFooter>
    </form>
  )
}
