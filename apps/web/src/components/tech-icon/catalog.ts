import { resolveTechIcon } from "./resolve"

/** Technologies offered by the technology field; every entry has a bundled icon. */
export const knownTechnologies: readonly string[] = [
  "Go",
  "Rust",
  "Python",
  "TypeScript",
  "Node.js",
  "Java",
  "Kotlin",
  "Spring Boot",
  ".NET",
  "React",
  "Vite",
  "GraphQL",
  "PostgreSQL",
  "MySQL",
  "MongoDB",
  "Redis",
  "SQLite",
  "Oracle",
  "Elasticsearch",
  "ClickHouse",
  "RabbitMQ",
  "Apache Kafka",
  "Nginx",
  "Traefik",
  "Caddy",
  "Envoy",
  "Istio",
  "Docker",
  "Docker Compose",
  "Kubernetes",
  "Kubernetes Service",
  "Kubernetes Ingress",
  "Helm",
  "Terraform",
  "Ansible",
  "Linux",
  "Ubuntu",
  "Debian",
  "Red Hat",
  "VMware",
  "IBM",
  "Prometheus",
  "Grafana",
  "OpenTelemetry",
  "GitHub",
  "GitLab",
  "Jenkins",
  "Argo CD",
  "Vault",
  "Keycloak",
  "MinIO",
  "Cloudflare",
  "AWS",
  "Amazon EC2",
  "Amazon RDS",
  "Amazon S3",
  "Amazon Lambda",
  "Amazon SQS",
  "Amazon EKS",
  "Azure",
  "Google Cloud",
]

/**
 * Suggestions for the technology field: what the model already uses first (most
 * used first, so spelling stays consistent), then the known list.
 * De-duplicated ignoring case.
 */
export function technologySuggestions(used: Iterable<string>): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  const push = (t: string) => {
    const name = t.trim()
    const key = name.toLowerCase()
    if (name === "" || seen.has(key)) return
    seen.add(key)
    out.push(name)
  }
  const counts = new Map<string, number>()
  for (const t of used) counts.set(t.trim(), (counts.get(t.trim()) ?? 0) + 1)
  for (const [t] of [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))) push(t)
  for (const t of knownTechnologies) push(t)
  return out
}

export const hasKnownIcon = (technology: string) => resolveTechIcon(technology) !== undefined
