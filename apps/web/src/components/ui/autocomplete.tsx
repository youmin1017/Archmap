import * as React from "react"
import { Autocomplete as AutocompletePrimitive } from "@base-ui/react/autocomplete"
import { cn } from "cn"

import { Input } from "@/components/ui/input"

/**
 * A text input that suggests items as you type. The value is free text: picking
 * a suggestion just fills the input, so values that are not listed stay valid.
 */
function TextAutocomplete({
  id,
  value,
  onValueChange,
  items,
  renderItem,
  leading,
  placeholder,
  emptyText = "No match; your text is used as typed.",
  maxLength,
  className,
}: {
  id?: string
  value: string
  onValueChange: (value: string) => void
  items: readonly string[]
  renderItem?: (item: string) => React.ReactNode
  /** Shown inside the input, on the left (e.g. an icon for the current value). */
  leading?: React.ReactNode
  placeholder?: string
  emptyText?: string
  maxLength?: number
  className?: string
}) {
  return (
    <AutocompletePrimitive.Root items={items} value={value} onValueChange={(v) => onValueChange(v)}>
      <div className="relative">
        {leading && (
          <span className="pointer-events-none absolute top-1/2 left-3 flex -translate-y-1/2 items-center">
            {leading}
          </span>
        )}
        <AutocompletePrimitive.Input
          id={id}
          placeholder={placeholder}
          maxLength={maxLength}
          render={<Input className={cn(leading ? "pl-9" : undefined, className)} />}
        />
      </div>
      <AutocompletePrimitive.Portal>
        <AutocompletePrimitive.Positioner sideOffset={4} className="isolate z-50">
          <AutocompletePrimitive.Popup
            data-slot="autocomplete-content"
            className="w-(--anchor-width) max-w-(--available-width) min-w-36 origin-(--transform-origin) overflow-hidden rounded-2xl bg-popover text-popover-foreground shadow-2xl ring-1 ring-foreground/5 duration-100 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
          >
            <AutocompletePrimitive.Empty className="px-3 py-2 text-xs text-muted-foreground empty:hidden">
              {emptyText}
            </AutocompletePrimitive.Empty>
            <AutocompletePrimitive.List className="max-h-64 overflow-y-auto p-1 outline-none data-empty:p-0">
              {(item: string) => (
                <AutocompletePrimitive.Item
                  key={item}
                  value={item}
                  className="flex cursor-default items-center gap-2 rounded-xl px-3 py-1.5 text-sm outline-hidden select-none data-highlighted:bg-accent data-highlighted:text-accent-foreground"
                >
                  {renderItem ? renderItem(item) : item}
                </AutocompletePrimitive.Item>
              )}
            </AutocompletePrimitive.List>
          </AutocompletePrimitive.Popup>
        </AutocompletePrimitive.Positioner>
      </AutocompletePrimitive.Portal>
    </AutocompletePrimitive.Root>
  )
}

export { TextAutocomplete }
