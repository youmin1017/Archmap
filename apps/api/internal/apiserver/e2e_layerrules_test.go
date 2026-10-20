package apiserver_test

import (
	"net/http"
	"testing"

	"archmap/api/internal/gen/api"
)

func TestHTTPLayerRules(t *testing.T) {
	c, editor := newClient(t)
	const maintainer, viewer = "maintainer@test.local", "viewer@test.local"
	j, p := "application/json", "application/problem+json"
	rules := "/api/v1/layer-rules"

	var got api.LayerRules
	c.expect(c.do("GET", rules, viewer, nil, &got), http.StatusOK, j)
	if len(got.Rules) != 0 {
		t.Fatalf("rules = %+v, want none", got.Rules)
	}

	// Only maintainers change rules; the input is validated and cleaned.
	body := obj{"rules": []obj{{"element_class": "database", "layers": []string{"Services", " data ", "DATA"}}, {"element_class": "node", "layers": []string{}}}}
	c.expect(c.do("PUT", rules, editor, body, nil), http.StatusForbidden, p)
	c.expect(c.do("PUT", rules, maintainer, obj{"rules": []obj{{"element_class": "gizmo", "layers": []string{"A"}}}}, nil), http.StatusBadRequest, p)
	c.expect(c.do("PUT", rules, maintainer, obj{"rules": []obj{{"element_class": "node", "layers": []string{"A"}}, {"element_class": "node", "layers": []string{"B"}}}}, nil), http.StatusBadRequest, p)
	c.expect(c.do("PUT", rules, maintainer, body, &got), http.StatusOK, j)
	if len(got.Rules) != 1 || got.Rules[0].ElementClass != "database" || len(got.Rules[0].Layers) != 2 {
		t.Fatalf("stored rules = %+v", got.Rules)
	}

	var els api.ElementList
	c.expect(c.do("GET", "/api/v1/model/elements", viewer, nil, &els), http.StatusOK, j)
	var pg, apiEl api.Element
	for _, e := range els.Elements {
		switch e.Name {
		case "Orders DB": // a logical database (type database) under the PostgreSQL server
			pg = e
		case "API":
			apiEl = e
		}
	}

	var cs api.Changeset
	c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": "Layers"}, &cs), http.StatusCreated, j)
	changes := "/api/v1/changesets/" + cs.Id.String() + "/changes"
	move := func(id any, layer string) obj {
		return obj{"changes": []obj{{"op": "update", "target_type": "element", "target_id": id, "patch": obj{"layer": layer}}}}
	}
	// A database may only sit on Services or Data (any case); other elements are not restricted.
	c.expect(c.do("POST", changes, editor, move(pg.Id, "Physical"), nil), http.StatusBadRequest, p)
	c.expect(c.do("POST", changes, editor, move(pg.Id, "services"), nil), http.StatusCreated, j)
	c.expect(c.do("POST", changes, editor, move(pg.Id, ""), nil), http.StatusCreated, j)
	c.expect(c.do("POST", changes, editor, move(apiEl.Id, "Physical"), nil), http.StatusCreated, j)
	// Changing the kind into a restricted one is checked too (API is on Physical now).
	c.expect(c.do("POST", changes, editor, obj{"changes": []obj{{"op": "update", "target_type": "element", "target_id": apiEl.Id, "patch": obj{"kind": "database"}}}}, nil), http.StatusBadRequest, p)
	// Adding a database to a disallowed layer is rejected, to an allowed one it is fine.
	add := func(layer string) obj {
		return obj{"changes": []obj{{"op": "add", "target_type": "element", "patch": obj{"kind": "database", "name": "Audit DB", "parent_id": pg.ParentId, "layer": layer}}}}
	}
	c.expect(c.do("POST", changes, editor, add("Kubernetes"), nil), http.StatusBadRequest, p)
	c.expect(c.do("POST", changes, editor, add("Data"), nil), http.StatusCreated, j)

	// Clearing the rules lifts the restriction.
	c.expect(c.do("PUT", rules, maintainer, obj{"rules": []obj{}}, nil), http.StatusOK, j)
	c.expect(c.do("POST", changes, editor, move(pg.Id, "Physical"), nil), http.StatusCreated, j)
}
