import { useState } from "react"

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
import { MAX_LAYER_NAME, layerNameError } from "../layers/layers"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Existing layer names of the view. */
  layers: readonly string[]
  /** The layer being renamed; omit to create a new one. */
  renaming?: string
  pending?: boolean
  onSubmit: (name: string) => void
}

/** Name prompt for creating or renaming a layer. */
export function LayerNameDialog(props: Props) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        {props.open && <LayerNameForm key={props.renaming ?? "new"} {...props} />}
      </DialogContent>
    </Dialog>
  )
}

function LayerNameForm({ layers, renaming, pending, onOpenChange, onSubmit }: Props) {
  const [name, setName] = useState(renaming ?? "")
  const [touched, setTouched] = useState(false)
  const error = layerNameError(layers, name, renaming)
  const unchanged = renaming !== undefined && name.trim() === renaming
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        setTouched(true)
        if (error || unchanged || pending) return
        onSubmit(name.trim())
      }}
    >
      <DialogHeader>
        <DialogTitle>{renaming ? `Rename layer ${renaming}` : "New layer"}</DialogTitle>
        <DialogDescription>
          {renaming
            ? "Elements on this layer move to the new name in this draft."
            : "The new layer is added at the bottom of the stack."}
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="layer-name">Name</Label>
        <Input
          id="layer-name"
          value={name}
          maxLength={MAX_LAYER_NAME}
          autoFocus
          aria-invalid={touched && error !== null}
          aria-describedby={touched && error ? "layer-name-error" : undefined}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. Data"
        />
        {touched && error && (
          <p id="layer-name-error" role="alert" className="text-xs text-destructive">
            {error}
          </p>
        )}
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button type="submit" disabled={pending || unchanged}>
          {renaming ? "Rename" : "Create"}
        </Button>
      </DialogFooter>
    </form>
  )
}
