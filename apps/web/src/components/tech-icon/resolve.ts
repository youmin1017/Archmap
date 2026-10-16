import type { IconifyIcon } from "@iconify/react"

import { logoIcons, simpleIcons } from "./icons"

export type ResolvedTechIcon = {
  data: IconifyIcon
  /** simple-icons are monochrome (currentColor); logos are colored brand artwork. */
  source: "simple" | "logos"
  /** Colored logo whose artwork is too dark on dark backgrounds: render as a light silhouette there. */
  lowContrastOnDark: boolean
}

// Case-insensitive alias map: free-text technology / tag -> icon slug. Keys are
// normalized (lowercase, only a-z0-9). Slugs refer to icons.ts.
const simpleAliases: Partial<Record<string, keyof typeof simpleIcons>> = {
  postgres: "postgresql",
  postgresql: "postgresql",
  pg: "postgresql",
  pgx: "postgresql",
  mysql: "mysql",
  mariadb: "mysql",
  mongo: "mongodb",
  mongodb: "mongodb",
  redis: "redis",
  valkey: "redis",
  sqlite: "sqlite",
  rabbitmq: "rabbitmq",
  amqp: "rabbitmq",
  kafka: "apachekafka",
  apachekafka: "apachekafka",
  elasticsearch: "elasticsearch",
  elastic: "elasticsearch",
  clickhouse: "clickhouse",
  nginx: "nginx",
  traefik: "traefikproxy",
  caddy: "caddy",
  envoy: "envoyproxy",
  istio: "istio",
  docker: "docker",
  compose: "docker",
  dockercompose: "docker",
  k8s: "kubernetes",
  kubernetes: "kubernetes",
  helm: "helm",
  terraform: "terraform",
  ansible: "ansible",
  go: "go",
  golang: "go",
  rust: "rust",
  python: "python",
  py: "python",
  typescript: "typescript",
  ts: "typescript",
  node: "nodedotjs",
  nodejs: "nodedotjs",
  react: "react",
  reactjs: "react",
  tanstack: "vite",
  vite: "vite",
  vitest: "vitest",
  java: "openjdk",
  openjdk: "openjdk",
  graphql: "graphql",
  prometheus: "prometheus",
  grafana: "grafana",
  otel: "opentelemetry",
  opentelemetry: "opentelemetry",
  linux: "linux",
  ubuntu: "ubuntu",
  debian: "debian",
  rhel: "redhat",
  redhat: "redhat",
  github: "github",
  gitlab: "gitlab",
  jenkins: "jenkins",
  argo: "argo",
  argocd: "argo",
  vault: "vault",
  keycloak: "keycloak",
  minio: "minio",
  cloudflare: "cloudflare",
  aws: "amazonaws",
  amazonaws: "amazonaws",
  azure: "microsoftazure",
  gcp: "googlecloud",
  googlecloud: "googlecloud",
}

// Colored fallbacks for technologies simple-icons does not cover (devicon / logos).
const logoAliases: Partial<Record<string, { slug: keyof typeof logoIcons; lowContrastOnDark?: boolean }>> = {
  spring: { slug: "spring" },
  springboot: { slug: "spring" },
  dotnet: { slug: "dotnet" },
  net: { slug: "dotnet" },
  csharp: { slug: "dotnet" },
  kotlin: { slug: "kotlin" },
  oracle: { slug: "oracle" },
  vmware: { slug: "vmware", lowContrastOnDark: true },
  ibm: { slug: "ibm", lowContrastOnDark: true },
  ec2: { slug: "aws-ec2" },
  rds: { slug: "aws-rds" },
  s3: { slug: "aws-s3" },
  lambda: { slug: "aws-lambda" },
  sqs: { slug: "aws-sqs" },
  eks: { slug: "aws-eks" },
  azurecloud: { slug: "microsoft-azure" },
}

const normalize = (s: string) => s.toLowerCase().replace(/[^a-z0-9]/g, "")

function lookup(key: string): ResolvedTechIcon | undefined {
  const simple = simpleAliases[key]
  if (simple) return { data: simpleIcons[simple], source: "simple", lowContrastOnDark: false }
  const logo = logoAliases[key]
  if (logo) return { data: logoIcons[logo.slug], source: "logos", lowContrastOnDark: logo.lowContrastOnDark ?? false }
  return undefined
}

/**
 * Resolves free-text technology to bundled icon data: simple-icons (monochrome)
 * first, then colored logos. Returns undefined when nothing matches so the
 * caller can fall back to a generic lucide icon. (Custom uploads, M6, are
 * resolved by the caller before this and win over both.)
 */
export function resolveTechIcon(technology: string): ResolvedTechIcon | undefined {
  const whole = normalize(technology)
  if (!whole) return undefined
  const direct = lookup(whole)
  if (direct) return direct
  // "PostgreSQL 16", "Go / Echo", "Node.js + Express": try each word.
  for (const token of technology.split(/[\s,/+&|]+/)) {
    const hit = lookup(normalize(token))
    if (hit) return hit
  }
  return undefined
}
