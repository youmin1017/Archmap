import { Icon } from "@iconify/react"
import { Box } from "lucide-react"
import postgresql from "@iconify-icons/simple-icons/postgresql"
import kubernetes from "@iconify-icons/simple-icons/kubernetes"
import docker from "@iconify-icons/simple-icons/docker"
import nginx from "@iconify-icons/simple-icons/nginx"
import go from "@iconify-icons/simple-icons/go"
import react from "@iconify-icons/simple-icons/react"

// Icon data is imported statically so it is bundled at build time (offline-safe).
// M0 stub: simple-icons subset + lucide fallback. Custom uploads and devicon
// fallback land in M3/M6 (see docs/PROJECT_BRIEF.md section 2, "Icons").
const icons = { postgresql, kubernetes, docker, nginx, go, react }

// Case-insensitive alias map: free-text technology -> icon slug.
const aliases: Partial<Record<string, keyof typeof icons>> = {
  postgres: "postgresql",
  postgresql: "postgresql",
  k8s: "kubernetes",
  kubernetes: "kubernetes",
  docker: "docker",
  nginx: "nginx",
  go: "go",
  golang: "go",
  react: "react",
}

export function resolveTechIcon(technology: string) {
  const slug = aliases[technology.trim().toLowerCase()]
  return slug ? icons[slug] : undefined
}

export function TechIcon({ technology, className }: { technology: string; className?: string }) {
  const data = resolveTechIcon(technology)
  // simple-icons are monochrome: currentColor keeps contrast in both themes.
  return data ? (
    <Icon icon={data} className={className} aria-label={technology} role="img" />
  ) : (
    <Box className={className} aria-label={technology} role="img" />
  )
}
