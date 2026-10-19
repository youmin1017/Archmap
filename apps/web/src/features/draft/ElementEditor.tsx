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
      <ElementFields
        form={form}
        layerOptions={layerOptions}
        projectOptions={projectOptions}
        technologyOptions={technologyOptions}
        canProtect={canProtect}
        compact
      />
      {/* Pinned to the bottom of the scrolling panel so Save is always reachable. */}
      <div className="sticky bottom-0 z-10 -mx-4 flex items-center gap-1.5 border-t bg-sidebar/95 px-4 py-3 backdrop-blur">
        <span className="mr-auto text-[11px] text-muted-foreground">
          <kbd className="font-mono">Enter</kbd> save · <kbd className="font-mono">Esc</kbd> cancel
        </span>
        <Button type="button" size="sm" variant="ghost" onClick={onCancel}>
          <X aria-hidden /> Cancel
        </Button>
        <Button type="submit" size="sm" disabled={!canSave}>
          <Check aria-hidden /> Save
        </Button>
      </div>
    </form>
  )
}
