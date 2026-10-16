import { useState } from "react"
import { ArrowDown, ArrowUp, Plus, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { MAX_LAYERS, planeHue } from "../layers/layers"

export const defaultLayers = ["User", "Services", "Kubernetes", "Physical"]
export { MAX_LAYERS }

/** Adds a layer name (trimmed); ignores blanks, duplicates (any case) and anything beyond the limit. */
export function addLayer(layers: readonly string[], name: string): string[] {
  const n = name.trim()
  if (!n || layers.length >= MAX_LAYERS || layers.some((l) => l.toLowerCase() === n.toLowerCase())) return [...layers]
  return [...layers, n]
}

/** Moves the layer at `index` by `delta` positions (clamped); returns the list unchanged when it cannot move. */
export function moveLayer(layers: readonly string[], index: number, delta: number): string[] {
  const to = index + delta
  if (index < 0 || index >= layers.length || to < 0 || to >= layers.length) return [...layers]
  const next = [...layers]
  const [item] = next.splice(index, 1)
  next.splice(to, 0, item)
  return next
}

/** Ordered list editor for a layers view: the first entry is the top layer. */
export function LayersEditor({
  layers,
  onChange,
}: {
  layers: readonly string[]
  onChange: (layers: string[]) => void
}) {
  const [draft, setDraft] = useState("")
  const add = () => {
    onChange(addLayer(layers, draft))
    setDraft("")
  }
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-1.5 text-sm leading-none font-medium">
        Layers <span className="font-normal text-muted-foreground">(top to bottom)</span>
      </legend>
      <ol className="flex flex-col gap-1" aria-label="Layers, top to bottom">
        {layers.map((l, i) => (
          <li key={l} className="flex items-center gap-1.5 rounded-xl border px-2 py-1">
            <span
              className="size-3 shrink-0 rounded-full"
              style={{ background: `hsl(${planeHue(i)} 70% 60%)` }}
              aria-hidden
            />
            <span className="flex-1 truncate text-sm">{l}</span>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={`Move ${l} up`}
              disabled={i === 0}
              onClick={() => onChange(moveLayer(layers, i, -1))}
            >
              <ArrowUp aria-hidden />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={`Move ${l} down`}
              disabled={i === layers.length - 1}
              onClick={() => onChange(moveLayer(layers, i, 1))}
            >
              <ArrowDown aria-hidden />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={`Remove ${l}`}
              disabled={layers.length === 1}
              onClick={() => onChange(layers.filter((x) => x !== l))}
            >
              <X aria-hidden />
            </Button>
          </li>
        ))}
      </ol>
      <div className="flex gap-1.5">
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault() // do not submit the dialog form
              add()
            }
          }}
          placeholder="Add a layer, e.g. Docker"
          aria-label="New layer name"
          maxLength={60}
        />
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label="Add layer"
          disabled={!draft.trim() || layers.length >= MAX_LAYERS}
          onClick={add}
        >
          <Plus aria-hidden />
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        Elements are placed by their own “layer” field. Anything without a matching layer goes to an “Unassigned” plane.
      </p>
    </fieldset>
  )
}
