import { Container, Database, Layers, Puzzle, Server, User, type LucideIcon } from "lucide-react"
import type { ElementKind } from "@archmap/api-client"

/** Presentation of element kinds. Kinds use lucide icons; technologies use TechIcon (Iconify). */
export const kindMeta: Record<ElementKind, { label: string; icon: LucideIcon }> = {
  person: { label: "Person", icon: User },
  system: { label: "System", icon: Layers },
  container: { label: "Container", icon: Container },
  component: { label: "Component", icon: Puzzle },
  node: { label: "Node", icon: Server },
  database: { label: "Database", icon: Database },
}

export const kinds = Object.keys(kindMeta) as ElementKind[]

/** CSS variable holding the kind's accent color (defined in styles.css). */
export const kindColor = (kind: ElementKind) => `var(--kind-${kind})`
