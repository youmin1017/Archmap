// Package seed holds the dev sample data shared by cmd/seed and integration tests.
package seed

import (
	"encoding/json"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
)

// SampleModel builds the dev sample as one batch of change inputs: a small
// system (web, API, worker, DB, queue) plus its deployment tree
// physical host -> Docker host -> Nginx / Traefik, with deployed_on relations.
func SampleModel() []changeset.ChangeInput {
	b := &sampleBuilder{layers: map[string]string{
		"Customer":     "User",
		"Web Frontend": "Services", "API": "Services", "Worker": "Services",
		"PostgreSQL": "Data", "Message Queue": "Data",
		"Docker Host": "Platform", "Nginx": "Platform", "Traefik": "Platform",
		"Physical Host": "Physical",
	}, projects: map[string][]string{
		"Customer": {"Shop"}, "Web Frontend": {"Shop"}, "API": {"Shop", "Payments"}, "Worker": {"Shop", "Payments"},
		"PostgreSQL": {"Shop", "Payments"}, "Message Queue": {"Payments"}, "Payment Provider": {"Payments"},
		"Docker Host": {"Shop", "Payments"}, "Nginx": {"Shop"}, "Traefik": {"Payments"}, "Physical Host": {"Shop", "Payments"},
	}}

	customer := b.element("person", "Customer", "", "Buys things in the shop", nil)
	payments := b.element("system", "Payment Provider", "", "External payment gateway", nil, "external")
	shop := b.element("system", "Shop", "", "Online shop platform", nil)
	web := b.element("container", "Web Frontend", "React", "Single-page storefront", &shop, "frontend")
	apiSvc := b.element("container", "API", "Go", "REST API", &shop, "backend")
	worker := b.element("container", "Worker", "Go", "Processes queued jobs", &shop, "backend")
	pg := b.element("database", "PostgreSQL", "PostgreSQL", "Primary datastore", &shop, "database")
	mq := b.element("container", "Message Queue", "RabbitMQ", "Job queue", &shop, "messaging")

	b.relation("uses", customer, web, "HTTPS", "Browses and orders")
	b.relation("uses", web, apiSvc, "JSON/HTTPS", "Calls")
	b.relation("uses", apiSvc, pg, "SQL", "Reads and writes")
	b.relation("uses", apiSvc, mq, "AMQP", "Publishes jobs")
	b.relation("uses", worker, mq, "AMQP", "Consumes jobs")
	b.relation("uses", worker, pg, "SQL", "Reads and writes")
	b.relation("uses", apiSvc, payments, "HTTPS", "Charges cards")

	host := b.element("node", "Physical Host", "Bare metal", "Rack server in the data center", nil, "physical")
	docker := b.element("node", "Docker Host", "Docker", "Container runtime", &host)
	nginx := b.element("node", "Nginx", "Nginx", "Reverse proxy for the storefront", &docker, "reverse-proxy")
	traefik := b.element("node", "Traefik", "Traefik", "Reverse proxy / ingress for the API", &docker, "reverse-proxy")

	b.relation("deployed_on", web, nginx, "", "Served by")
	b.relation("deployed_on", apiSvc, traefik, "", "Routed by")
	b.relation("deployed_on", worker, docker, "", "Runs on")
	b.relation("deployed_on", pg, docker, "", "Runs on")
	b.relation("deployed_on", mq, docker, "", "Runs on")

	return b.inputs
}

type sampleBuilder struct {
	inputs []changeset.ChangeInput
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
	b.inputs = append(b.inputs, changeset.ChangeInput{
		Op: changeset.OpAdd, TargetType: changeset.TargetElement, TargetID: id, Patch: mustJSON(patch),
	})
	return id
}

func (b *sampleBuilder) relation(kind string, source, target uuid.UUID, tech, desc string) {
	b.inputs = append(b.inputs, changeset.ChangeInput{
		Op: changeset.OpAdd, TargetType: changeset.TargetRelation,
		Patch: mustJSON(map[string]any{
			"kind": kind, "source_id": source, "target_id": target, "technology": tech, "description": desc,
		}),
	})
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
