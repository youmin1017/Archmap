import type { ElementKind } from "@archmap/api-client"

// UI hints only: they narrow what the forms offer. The API's lint rules are the authority
// (hierarchy errors block a merge); this just avoids obviously invalid choices.
export const allowedParentKinds: Record<ElementKind, readonly ElementKind[]> = {
  person: [],
  system: [],
  container: ["system"],
  component: ["container"],
  node: ["node"],
}

export const requiresParent = (kind: ElementKind) => kind === "container" || kind === "component"

/** Parses "a, b ,,c" into ["a", "b", "c"] (trimmed, de-duplicated, empty entries dropped). */
export function parseTags(input: string): string[] {
  return [
    ...new Set(
      input
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean)
    ),
  ]
}
