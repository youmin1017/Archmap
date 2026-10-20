// Package seed holds the dev sample data shared by cmd/seed and integration tests.
// All teams and contacts are fictional (.example domains).
package seed

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/model"
)

// SampleModel builds the dev sample as one batch of change inputs: a small
// system (web, API, worker, a database server with two logical databases, a
// queue) plus its deployment tree physical host -> Docker host -> Nginx /
// Traefik, with deployed_on relations, fictional owner teams and operational
// fields (M10).
func SampleModel() []changeset.ChangeInput {
	b := &sampleBuilder{patches: map[uuid.UUID]map[string]any{}, layers: map[string]string{
		"Customer":     "User",
		"Web Frontend": "Services", "API": "Services", "Worker": "Services",
		"PostgreSQL": "Data", "Orders DB": "Data", "Payments DB": "Data", "Message Queue": "Data",
		"Docker Host": "Platform", "Nginx": "Platform", "Traefik": "Platform",
		"Physical Host": "Physical",
	}, projects: map[string][]string{
		"Customer": {"Shop"}, "Web Frontend": {"Shop"}, "API": {"Shop", "Payments"}, "Worker": {"Shop", "Payments"},
		"PostgreSQL": {"Shop", "Payments"}, "Orders DB": {"Shop"}, "Payments DB": {"Payments"},
		"Message Queue": {"Payments"}, "Payment Provider": {"Payments"},
		"Docker Host": {"Shop", "Payments"}, "Nginx": {"Shop"}, "Traefik": {"Payments"}, "Physical Host": {"Shop", "Payments"},
	}}

	customer := b.element("person", "Customer", "", "Buys things in the shop", nil)
	payments := b.element("system", "Payment Provider", "", "External payment gateway", nil, "external")
	shop := b.element("system", "Shop", "", "Online shop platform", nil)
	web := b.element("container", "Web Frontend", "React", "Single-page storefront", &shop, "frontend")
	apiSvc := b.element("container", "API", "Go", "REST API", &shop, "backend")
	worker := b.element("container", "Worker", "Go", "Processes queued jobs", &shop, "backend")
	pg := b.element("container", "PostgreSQL", "PostgreSQL", "Database server shared by the shop", &shop, "database")
	orders := b.element("component", "Orders DB", "PostgreSQL", "Logical database for orders", &pg)
	paymentsDB := b.element("component", "Payments DB", "PostgreSQL", "Logical database for payments", &pg)
	mq := b.element("container", "Message Queue", "RabbitMQ", "Job queue", &shop, "messaging")

	b.relation("uses", customer, web, "HTTPS", "Browses and orders")
	b.relation("uses", web, apiSvc, "JSON/HTTPS", "Calls")
	b.relation("uses", apiSvc, orders, "SQL", "Reads and writes orders")
	b.relation("uses", apiSvc, mq, "AMQP", "Publishes jobs")
	b.relation("uses", worker, mq, "AMQP", "Consumes jobs")
	b.relation("uses", worker, paymentsDB, "SQL", "Records payments")
	b.relation("uses", apiSvc, payments, "HTTPS", "Charges cards")

	host := b.element("node", "Physical Host", "Bare metal", "Rack server in the data center", nil, "physical")
	docker := b.element("node", "Docker Host", "Docker", "Container runtime", &host)
	nginx := b.element("node", "Nginx", "Nginx", "Reverse proxy for the storefront", &docker, "reverse-proxy")
	traefik := b.element("node", "Traefik", "Traefik", "Reverse proxy / ingress for the API", &docker, "reverse-proxy")

	b.relation("deployed_on", web, nginx, "", "Served by")
	b.relation("deployed_on", apiSvc, traefik, "", "Routed by")
	b.relation("deployed_on", worker, docker, "", "Runs on")
	b.relation("deployed_on", pg, docker, "", "Runs on") // the logical databases inherit this
	b.relation("deployed_on", mq, docker, "", "Runs on")

	// Operational fields: the shop and its hosts run in prod; everything there was reviewed recently.
	reviewed := time.Now().AddDate(0, 0, -30).Format(time.DateOnly)
	b.with(shop, map[string]any{"environment": "prod", "docs_url": "https://docs.example/shop", "last_reviewed_at": reviewed})
	b.with(host, map[string]any{"environment": "prod", "last_reviewed_at": reviewed})
	for _, id := range []uuid.UUID{web, apiSvc, worker, pg, orders, paymentsDB, mq, docker, nginx, traefik} {
		b.with(id, map[string]any{"last_reviewed_at": reviewed})
	}
	b.with(pg, map[string]any{"type": "database-server", "criticality": "critical"})
	b.with(orders, map[string]any{"type": "database"})
	b.with(paymentsDB, map[string]any{"type": "database", "criticality": "high"})
	b.with(mq, map[string]any{"type": "queue"})
	b.with(apiSvc, map[string]any{"criticality": "high", "runbook_url": "https://runbooks.example/shop-api"})
	// Traefik is being replaced: the API still routes through it (a deprecated-dependency warning).
	b.with(traefik, map[string]any{"type": "proxy", "lifecycle": "deprecated"})
	b.with(nginx, map[string]any{"type": "proxy"})

	atlas := b.owner("team", "Team Atlas", "atlas@shop.example", "https://oncall.example/atlas")
	borealis := b.owner("team", "Team Borealis", "#payments-borealis", "https://oncall.example/borealis")
	crew := b.owner("team", "Platform Crew", "platform@ops.example", "https://oncall.example/platform")
	b.ownership(shop, atlas, model.RoleOwner)          // inherited by every shop container
	b.ownership(shop, crew, model.RoleOncall)          // ... and on-call
	b.ownership(paymentsDB, borealis, model.RoleOwner) // override: payments data belongs to Borealis
	b.ownership(host, crew, model.RoleOwner)

	return b.finish()
}

type sampleBuilder struct {
	order   []uuid.UUID
	patches map[uuid.UUID]map[string]any
	rest    []changeset.ChangeInput // relations, owners and ownerships, after the elements
	// layers maps element name to its layer in layered (3D) views.
	layers map[string]string
	// projects maps element name to the projects it belongs to.
	projects map[string][]string
}

func (b *sampleBuilder) element(kind, name, tech, desc string, parent *uuid.UUID, tags ...string) uuid.UUID {
	id := uuid.Must(uuid.NewV7())
	patch := map[string]any{"kind": kind, "name": name, "technology": tech, "description": desc, "tags": tags, "layer": b.layers[name], "projects": b.projects[name]}
	if tags == nil {
		patch["tags"] = []string{}
	}
	if b.projects[name] == nil {
		patch["projects"] = []string{}
	}
	if parent != nil {
		patch["parent_id"] = *parent
	}
	b.order = append(b.order, id)
	b.patches[id] = patch
	return id
}

// with sets more fields on an element added earlier.
func (b *sampleBuilder) with(id uuid.UUID, fields map[string]any) {
	for k, v := range fields {
		b.patches[id][k] = v
	}
}

func (b *sampleBuilder) relation(kind string, source, target uuid.UUID, tech, desc string) {
	b.rest = append(b.rest, changeset.ChangeInput{
		Op: changeset.OpAdd, TargetType: changeset.TargetRelation,
		Patch: mustJSON(map[string]any{
			"kind": kind, "source_id": source, "target_id": target, "technology": tech, "description": desc,
		}),
	})
}

func (b *sampleBuilder) owner(kind, name, contact, oncall string) uuid.UUID {
	id := uuid.Must(uuid.NewV7())
	b.rest = append(b.rest, changeset.ChangeInput{
		Op: changeset.OpAdd, TargetType: changeset.TargetOwner, TargetID: id,
		Patch: mustJSON(map[string]any{"kind": kind, "name": name, "contact": contact, "oncall_url": oncall}),
	})
	return id
}

func (b *sampleBuilder) ownership(element, owner uuid.UUID, role model.OwnerRole) {
	b.rest = append(b.rest, changeset.ChangeInput{
		Op: changeset.OpAdd, TargetType: changeset.TargetOwnership,
		Patch: mustJSON(map[string]any{"element_id": element, "owner_id": owner, "role": role}),
	})
}

func (b *sampleBuilder) finish() []changeset.ChangeInput {
	out := make([]changeset.ChangeInput, 0, len(b.order)+len(b.rest))
	for _, id := range b.order {
		out = append(out, changeset.ChangeInput{
			Op: changeset.OpAdd, TargetType: changeset.TargetElement, TargetID: id, Patch: mustJSON(b.patches[id]),
		})
	}
	return append(out, b.rest...)
}

func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err) // static, JSON-safe values only
	}
	return raw
}

// SampleDatabaseLayers are the layers databases may be placed on in the sample setup.
var SampleDatabaseLayers = []string{"Services", "Data"}

// SampleLayers is the layer order of the sample layers view, top to bottom.
var SampleLayers = []string{"User", "Services", "Data", "Platform", "Physical"}
