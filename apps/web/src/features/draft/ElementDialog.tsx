import type { ElementKind, LayerRule } from "@archmap/api-client"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import type { ElementTree } from "../canvas/graph"
import { ElementFields, useElementForm, type ElementFormValues } from "./ElementFields"

export type { ElementFormValues } from "./ElementFields"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  tree: ElementTree
  defaultKind?: ElementKind
  defaultParentId?: string | null
  /** Layer preselected for a new element (e.g. the plane that was right-clicked). */
  defaultLayer?: string
  /** Known layer names offered as suggestions (free text is allowed). */
  layerOptions?: readonly string[]
  /** Known project names offered as quick picks. */
  projectOptions?: readonly string[]
  /** Suggestions for the technology field. */
  technologyOptions?: readonly string[]
  /** Layer rules: the layer field is validated against them (the API enforces them too). */
  layerRules?: readonly LayerRule[]
  /** Only maintainers may change the protected flag. */
  canProtect: boolean
  pending?: boolean
  onSubmit: (values: ElementFormValues) => void
}

/** Creates an element. Editing an existing one happens in place in the details panel. */
export function ElementDialog(props: Props) {
  // Remount the form whenever the dialog opens so fields start fresh.
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">{props.open && <NewElementForm {...props} />}</DialogContent>
    </Dialog>
  )
}

function NewElementForm({
  tree,
  defaultKind,
  defaultParentId,
  defaultLayer,
  layerOptions,
  projectOptions,
  technologyOptions,
  layerRules = [],
  canProtect,
  pending,
  onOpenChange,
  onSubmit,
}: Props) {
  const form = useElementForm({ defaultKind, defaultParentId, defaultLayer }, tree, layerRules)
  const canSubmit = form.canSubmit && !pending

  return (
    <form
      className="-mx-1 flex max-h-[85vh] flex-col gap-4 overflow-y-auto px-1"
      onSubmit={(e) => {
        e.preventDefault()
        if (canSubmit) onSubmit(form.values())
      }}
    >
      <DialogHeader>
        <DialogTitle>Add element</DialogTitle>
        <DialogDescription>The element is added to this draft, not to main.</DialogDescription>
      </DialogHeader>

      <ElementFields
        form={form}
        layerOptions={layerOptions}
        projectOptions={projectOptions}
        technologyOptions={technologyOptions}
        canProtect={canProtect}
      />

      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button type="submit" disabled={!canSubmit}>
          Add element
        </Button>
      </DialogFooter>
    </form>
  )
}
