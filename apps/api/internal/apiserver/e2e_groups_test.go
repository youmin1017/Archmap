package apiserver_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/gen/api"
	"archmap/api/internal/model"
)

func TestHTTPGroupsAndPrincipalOwners(t *testing.T) {
	c, editor := newClient(t)
	const viewer, maintainer = "viewer@test.local", "maintainer@test.local"

	var dir api.Directory
	c.expect(c.do("GET", "/api/v1/directory", viewer, nil, &dir), http.StatusOK, jsonCT)
	userID := map[string]uuid.UUID{}
	for _, u := range dir.Users {
		userID[u.Email] = u.Id
	}
	if len(dir.Users) < 3 || len(dir.Groups) != 0 {
		t.Fatalf("directory = %+v", dir)
	}
	c.expect(c.do("GET", "/api/v1/directory", "", nil, nil), http.StatusUnauthorized, problemCT)

	// Only maintainers manage groups; names are unique ignoring case.
	c.expect(c.do("POST", "/api/v1/groups", editor, obj{"name": "Payments"}, nil), http.StatusForbidden, problemCT)
	var g api.Group
	c.expect(c.do("POST", "/api/v1/groups", maintainer, obj{"name": " Payments ", "description": "Owns payments"}, &g), http.StatusCreated, jsonCT)
	if g.Name != "Payments" || len(g.Members) != 0 {
		t.Fatalf("group = %+v", g)
	}
	c.expect(c.do("POST", "/api/v1/groups", maintainer, obj{"name": "payments"}, nil), http.StatusConflict, problemCT)
	group := "/api/v1/groups/" + g.Id.String()
	c.expect(c.do("PUT", group+"/members", maintainer, obj{"user_ids": []uuid.UUID{userID[editor], userID[viewer], userID[editor]}}, &g), http.StatusOK, jsonCT)
	if len(g.Members) != 2 {
		t.Fatalf("members = %+v", g.Members)
	}
	c.expect(c.do("PUT", group+"/members", maintainer, obj{"user_ids": []uuid.UUID{uuid.New()}}, nil), http.StatusBadRequest, problemCT)
	c.expect(c.do("PUT", group, maintainer, obj{"name": "Payments Team"}, &g), http.StatusOK, jsonCT)

	// A draft assigns the group to the API and the editor (a user) to the worker.
	var els api.ElementList
	c.expect(c.do("GET", "/api/v1/model/elements", viewer, nil, &els), http.StatusOK, jsonCT)
	byName := map[string]uuid.UUID{}
	for _, e := range els.Elements {
		byName[e.Name] = e.Id
	}
	var cs api.Changeset
	c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": "principals"}, &cs), http.StatusCreated, jsonCT)
	draft := "/api/v1/changesets/" + cs.Id.String()
	groupOwner, userOwner := model.PrincipalOwnerID(g.Id), model.PrincipalOwnerID(userID[editor])
	c.expect(c.do("POST", draft+"/changes", editor, obj{"changes": []obj{
		{"op": "add", "target_type": "owner", "patch": obj{"kind": "group", "name": "", "contact": "", "oncall_url": "", "principal_id": g.Id}},
		{"op": "add", "target_type": "ownership", "patch": obj{"element_id": byName["API"], "owner_id": groupOwner, "role": "owner"}},
		{"op": "add", "target_type": "owner", "patch": obj{"kind": "user", "name": "", "contact": "", "oncall_url": "", "principal_id": userID[editor]}},
		{"op": "add", "target_type": "ownership", "patch": obj{"element_id": byName["Worker"], "owner_id": userOwner, "role": "oncall"}},
	}}, nil), http.StatusCreated, jsonCT)
	c.expect(c.do("POST", draft+"/submit", editor, nil, nil), http.StatusOK, jsonCT)
	c.expect(c.do("POST", draft+"/approve", maintainer, nil, nil), http.StatusOK, jsonCT)
	c.expect(c.do("POST", draft+"/merge", maintainer, nil, nil), http.StatusOK, jsonCT)

	var owners api.OwnerDirectory
	c.expect(c.do("GET", "/api/v1/model/owners", viewer, nil, &owners), http.StatusOK, jsonCT)
	found := map[uuid.UUID]api.Owner{}
	for _, o := range owners.Owners {
		found[o.Id] = o
	}
	if o := found[groupOwner]; o.Kind != "group" || o.PrincipalId == nil || *o.PrincipalId != g.Id || o.Name != "Payments Team" {
		t.Fatalf("group owner = %+v", o)
	}
	if o := found[userOwner]; o.Kind != "user" || o.PrincipalId == nil || *o.PrincipalId != userID[editor] {
		t.Fatalf("user owner = %+v", o)
	}

	// Disabling the user and deleting the group: owners keep their names; lint warns in new drafts.
	c.expect(c.do("POST", "/api/v1/users/"+userID[editor].String()+"/disable", maintainer, nil, nil), http.StatusOK, jsonCT)
	c.expect(c.do("DELETE", group, maintainer, nil, nil), http.StatusNoContent, "")
	c.expect(c.do("GET", group, maintainer, nil, nil), http.StatusMethodNotAllowed, problemCT)
	var cs2 api.Changeset
	c.expect(c.do("POST", "/api/v1/changesets", maintainer, obj{"title": "check"}, &cs2), http.StatusCreated, jsonCT)
	var report api.LintReport
	c.expect(c.do("GET", "/api/v1/changesets/"+cs2.Id.String()+"/lint", viewer, nil, &report), http.StatusOK, jsonCT)
	inactive := 0
	for _, f := range report.Findings {
		if f.Rule == "owner-inactive" {
			inactive++
		}
	}
	if inactive != 2 || report.Errors != 0 {
		t.Fatalf("lint = %+v", report)
	}
	// A disabled user cannot become a new owner.
	c.expect(c.do("POST", "/api/v1/changesets/"+cs2.Id.String()+"/changes", maintainer, obj{"changes": []obj{
		{"op": "add", "target_type": "owner", "patch": obj{"kind": "user", "name": "", "contact": "", "oncall_url": "", "principal_id": userID[editor]}},
	}}, nil), http.StatusConflict, problemCT) // the owner already exists in main
	c.expect(c.do("POST", "/api/v1/changesets/"+cs2.Id.String()+"/changes", maintainer, obj{"changes": []obj{
		{"op": "update", "target_type": "owner", "target_id": userOwner, "patch": obj{"contact": "x@shop.example"}},
	}}, nil), http.StatusBadRequest, problemCT) // editing it re-checks the principal: disabled
	var dir2 api.Directory
	c.expect(c.do("GET", "/api/v1/directory", viewer, nil, &dir2), http.StatusOK, jsonCT)
	if len(dir2.Groups) != 0 {
		t.Fatalf("deleted group still listed: %+v", dir2.Groups)
	}
}
