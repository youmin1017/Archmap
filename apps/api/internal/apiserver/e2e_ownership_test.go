package apiserver_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/gen/api"
	"archmap/api/internal/model"
)

func TestHTTPOwnershipAndEnrichment(t *testing.T) {
	c, editor := newClient(t)
	const viewer, maintainer = "viewer@test.local", "maintainer@test.local"

	// Main as seeded: fictional teams, an override on the payments database.
	var dir api.OwnerDirectory
	c.expect(c.do("GET", "/api/v1/model/owners", viewer, nil, &dir), http.StatusOK, jsonCT)
	ownerName := map[uuid.UUID]string{}
	ownerID := map[string]uuid.UUID{}
	for _, o := range dir.Owners {
		ownerName[o.Id] = o.Name
		ownerID[o.Name] = o.Id
	}
	if len(dir.Owners) != 3 || len(dir.Ownerships) != 4 {
		t.Fatalf("directory = %+v", dir)
	}

	var els api.ElementList
	c.expect(c.do("GET", "/api/v1/model/elements", viewer, nil, &els), http.StatusOK, jsonCT)
	byName := map[string]api.Element{}
	for _, e := range els.Elements {
		byName[e.Name] = e
		if e.Kind == "database" || e.Effective == nil {
			t.Fatalf("element %s: kind %s, effective %v", e.Name, e.Kind, e.Effective)
		}
	}
	owner := func(e api.Element) (string, uuid.UUID) {
		for _, o := range e.Effective.Owners {
			if o.Role == "owner" {
				return ownerName[o.OwnerId], o.SourceElementId
			}
		}
		return "", uuid.Nil
	}
	orders, payments, pg, shop := byName["Orders DB"], byName["Payments DB"], byName["PostgreSQL"], byName["Shop"]
	if name, from := owner(orders); name != "Team Atlas" || from != shop.Id {
		t.Fatalf("Orders DB owner = %s from %s", name, from)
	}
	if name, from := owner(payments); name != "Team Borealis" || from != payments.Id {
		t.Fatalf("Payments DB owner = %s from %s (override)", name, from)
	}
	if orders.Effective.Environment != "prod" || *orders.Effective.EnvironmentSourceId != shop.Id ||
		orders.Effective.DeployedViaId == nil || *orders.Effective.DeployedViaId != pg.Id || orders.Type != "database" || pg.Type != "database-server" {
		t.Fatalf("Orders DB = %+v / %+v", orders, *orders.Effective)
	}

	newDraft := func(title string) string {
		var cs api.Changeset
		c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": title}, &cs), http.StatusCreated, jsonCT)
		return "/api/v1/changesets/" + cs.Id.String()
	}
	change := func(path string, changes ...obj) *api.ChangeList {
		var out api.ChangeList
		c.expect(c.do("POST", path+"/changes", editor, obj{"changes": changes}, &out), http.StatusCreated, jsonCT)
		return &out
	}
	review := func(path string) {
		c.expect(c.do("POST", path+"/submit", editor, nil, nil), http.StatusOK, jsonCT)
		c.expect(c.do("POST", path+"/approve", maintainer, nil, nil), http.StatusOK, jsonCT)
	}

	// A new team takes over the orders database (override), in one draft.
	cosmo := uuid.New()
	a := newDraft("Cosmo owns orders")
	change(a,
		obj{"op": "add", "target_type": "owner", "target_id": cosmo, "patch": obj{"kind": "team", "name": "Team Cosmo", "contact": "cosmo@shop.example", "oncall_url": ""}},
		obj{"op": "add", "target_type": "ownership", "patch": obj{"element_id": orders.Id, "owner_id": cosmo, "role": "owner"}},
	)
	var diff api.Diff
	c.expect(c.do("GET", a+"/diff", viewer, nil, &diff), http.StatusOK, jsonCT)
	if len(diff.Owners.Added) != 1 || len(diff.Ownerships.Added) != 1 || diff.Ownerships.Added[0].Id != model.OwnershipID(orders.Id, model.RoleOwner) {
		t.Fatalf("diff = owners %+v, ownerships %+v", diff.Owners, diff.Ownerships)
	}
	var draftModel api.Model
	c.expect(c.do("GET", a+"/model", viewer, nil, &draftModel), http.StatusOK, jsonCT)
	for _, e := range draftModel.Elements {
		if e.Id == orders.Id && (len(e.Effective.Owners) == 0 || e.Effective.Owners[0].OwnerId != cosmo) {
			t.Fatalf("draft effective owner of Orders DB = %+v", e.Effective.Owners)
		}
	}

	// A competing draft assigns the same role to another team: a conflict on owner_id.
	b := newDraft("Borealis owns orders")
	change(b, obj{"op": "add", "target_type": "ownership", "patch": obj{"element_id": orders.Id, "owner_id": ownerID["Team Borealis"], "role": "owner"}})
	review(a)
	c.expect(c.do("POST", a+"/merge", maintainer, nil, nil), http.StatusOK, jsonCT)
	review(b)
	rec := c.do("POST", b+"/merge", maintainer, nil, nil)
	c.expect(rec, http.StatusConflict, jsonCT)
	var conflicts api.ConflictList
	c.decode(rec, &conflicts)
	if len(conflicts.Conflicts) != 1 || conflicts.Conflicts[0].TargetType != "ownership" || conflicts.Conflicts[0].Field != "owner_id" {
		t.Fatalf("conflicts = %+v", conflicts.Conflicts)
	}

	// Production needs an owner: marking the payment provider prod blocks the merge until one is assigned.
	p := newDraft("Payment provider is prod")
	provider := byName["Payment Provider"]
	change(p, obj{"op": "update", "target_type": "element", "target_id": provider.Id, "patch": obj{"environment": "prod", "last_reviewed_at": "2026-10-01"}})
	var report api.LintReport
	c.expect(c.do("GET", p+"/lint", viewer, nil, &report), http.StatusOK, jsonCT)
	if report.Errors != 1 || report.Findings[0].Rule != "owner-required" {
		t.Fatalf("lint = %+v", report)
	}
	review(p)
	c.expect(c.do("POST", p+"/merge", maintainer, nil, nil), http.StatusConflict, problemCT)

	// Invalid values are rejected when edited.
	bad := newDraft("bad values")
	for _, patch := range []obj{{"environment": "qa"}, {"runbook_url": "not-a-url"}, {"type": "mainframe"}, {"last_reviewed_at": "yesterday"}} {
		c.expect(c.do("POST", bad+"/changes", editor, obj{"changes": []obj{{"op": "update", "target_type": "element", "target_id": provider.Id, "patch": patch}}}, nil), http.StatusBadRequest, problemCT)
	}

	// The Unowned view lists what has no owner: the payment provider, not the shop's containers.
	var v api.View
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "Unowned", "kind": "unowned"}, &v), http.StatusCreated, jsonCT)
	var vm api.ViewModel
	c.expect(c.do("GET", "/api/v1/views/"+v.Id.String()+"/model", viewer, nil, &vm), http.StatusOK, jsonCT)
	shown := map[string]bool{}
	for _, e := range vm.Elements {
		shown[e.Name] = true
	}
	if !shown["Payment Provider"] || shown["API"] || shown["Orders DB"] || shown["Customer"] {
		t.Fatalf("unowned view = %v", shown)
	}
}
