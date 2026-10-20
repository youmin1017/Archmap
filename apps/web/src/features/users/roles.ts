import type { Role } from "@archmap/api-client"

export const roles: { value: Role; label: string; short: string; can: string }[] = [
  { value: "viewer", label: "Viewer", short: "Browse only", can: "Browse main, drafts, views and versions." },
  {
    value: "editor",
    label: "Editor",
    short: "Draft changes",
    can: "Also create and edit drafts, and submit them for review.",
  },
  {
    value: "maintainer",
    label: "Maintainer",
    short: "Review and administer",
    can: "Also approve and merge, change protected elements, layer rules and users.",
  },
]
