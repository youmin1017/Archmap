import type { Element, ElementKind, Relation, RelationKind } from "@archmap/api-client"

import { elementDefaults } from "./element"

// Small model mirroring the API's seeded sample (see apps/api/internal/seed),
// used by unit tests and as an offline story for layout checks.

const id = (n: string) => `00000000-0000-7000-8000-${n.padStart(12, "0")}`

function el(
  key: string,
  kind: ElementKind,
  name: string,
  parent: string | null,
  extra: Partial<Element> = {}
): Element {
  return {
    ...elementDefaults,
    id: id(key),
    parent_id: parent ? id(parent) : null,
    kind,
    name,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...extra,
  }
}

function rel(key: string, kind: RelationKind, source: string, target: string, technology = ""): Relation {
  return {
    id: id(key),
    kind,
    source_id: id(source),
    target_id: id(target),
    description: "",
    technology,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  }
}

export const ids = {
  customer: id("1"),
  payments: id("2"),
  shop: id("3"),
  web: id("4"),
  api: id("5"),
  worker: id("6"),
  pg: id("7"),
  mq: id("8"),
  host: id("9"),
  docker: id("10"),
  nginx: id("11"),
  traefik: id("12"),
}

export const sampleElements: Element[] = [
  el("1", "person", "Customer", null),
  el("2", "system", "Payment Provider", null, { tags: ["external"] }),
  el("3", "system", "Shop", null, { description: "Online shop platform" }),
  el("4", "container", "Web Frontend", "3", { technology: "React", tags: ["frontend"] }),
  el("5", "container", "API", "3", { technology: "Go", tags: ["backend"], protected: true }),
  el("6", "container", "Worker", "3", { technology: "Go", tags: ["backend"] }),
  el("7", "container", "PostgreSQL", "3", { technology: "PostgreSQL", tags: ["database"] }),
  el("8", "container", "Message Queue", "3", { technology: "RabbitMQ" }),
  el("9", "node", "Physical Host", null, { technology: "Bare metal" }),
  el("10", "node", "Docker Host", "9", { technology: "Docker" }),
  el("11", "node", "Nginx", "10", { technology: "Nginx" }),
  el("12", "node", "Traefik", "10", { technology: "Traefik" }),
]

export const sampleRelations: Relation[] = [
  rel("101", "uses", "1", "4", "HTTPS"),
  rel("102", "uses", "4", "5", "JSON/HTTPS"),
  rel("103", "uses", "5", "7", "SQL"),
  rel("104", "uses", "5", "8", "AMQP"),
  rel("105", "uses", "6", "8", "AMQP"),
  rel("106", "uses", "6", "7", "SQL"),
  rel("107", "uses", "5", "2", "HTTPS"),
  rel("108", "deployed_on", "4", "11"),
  rel("109", "deployed_on", "5", "12"),
  rel("110", "deployed_on", "6", "10"),
  rel("111", "deployed_on", "7", "10"),
  rel("112", "deployed_on", "8", "10"),
]
