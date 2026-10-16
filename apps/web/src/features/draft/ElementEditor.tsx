import { Check, X } from "lucide-react"
import type { Element, LayerRule } from "@archmap/api-client"

import { Button } from "@/components/ui/button"
import type { ElementTree } from "../canvas/graph"
import { ElementFields, useElementForm, type ElementFormValues } from "./ElementFields"

/**
 * In-place editor of the details panel: the read-only fields turn into inputs
 * with Save / Cancel (Enter saves, Escape cancels). Remount it per element.
 */
export function ElementEditor({
  element,
  tree,
  deployedOn,
  layerOptions,
  projectOptions,
  technologyOptions,
  layerRules = [],
  canProtect,
  pending,
  onSave,
  onCancel,
}: {
  element: Element
  tree: ElementTree
  deployedOn: readonly string[]
  layerOptions?: readonly string[]
  projectOptions?: readonly string[]
  technologyOptions?: readonly string[]
  layerRules?: readonly LayerRule[]
  canProtect: boolean
  pending?: boolean
  onSave: (values: ElementFormValues) => void
  onCancel: () => void
}) {
  const form = useElementForm({ element, deployedOn }, tree, layerRules)
  const canSave = form.canSubmit && !pending

  return (
    <form
      aria-label={`Edit ${element.name}`}
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (canSave) onSave(form.values())
      }}
      onKeyDown={(e) => {
        // Escape cancels; stop it here so the canvas does not also treat it as "deselect".
        if (e.key === "Escape" && !e.defaultPrevented) {
          e.stopPropagation()
          onCancel()
        }
      }}
    >
      <div className="flex gap-1.5">
        <Button type="submit" size="sm" disabled={!canSave}>
          <Check aria-hidden /> Save
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={onCancel}>
          <X aria-hidden /> Cancel
        </Button>
      </div>
      <ElementFields
        form={form}
        layerOptions={layerOptions}
        projectOptions={projectOptions}
        technologyOptions={technologyOptions}
        canProtect={canProtect}
        compact
      />
    </form>
  )
}
