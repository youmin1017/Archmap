package apiserver_test

import (
	"net/http"
	"slices"
	"testing"

	"archmap/api/internal/gen/api"
)

func TestHTTPViewsAndLayout(t *testing.T) {
	c, editor := newClient(t)
	const viewer, maintainer = "viewer@test.local", "maintainer@test.local"
	j, p := "application/json", "application/problem+json"

	var els api.ElementList
	c.expect(c.do("GET", "/api/v1/model/elements", viewer, nil, &els), http.StatusOK, j)
	byName := map[string]api.Element{}
	for _, e := range els.Elements {
		byName[e.Name] = e
	}
	shop, apiEl := byName["Shop"], byName["API"]

	// Creating and laying out views must not touch the model, changes or versions.
	before := mainState(t, c)

	// Validation and permissions.
	c.expect(c.do("POST", "/api/v1/views", viewer, obj{"name": "x", "kind": "landscape"}, nil), http.StatusForbidden, p)
	c.expect(c.do("POST", "/api/v1/views", "", obj{"name": "x", "kind": "landscape"}, nil), http.StatusUnauthorized, p)
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "x", "kind": "context"}, nil), http.StatusBadRequest, p) // scope required
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "x", "kind": "nope"}, nil), http.StatusBadRequest, p)    // enum rejected by the spec

	var landscape, container, custom api.View
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "Landscape", "kind": "landscape"}, &landscape), http.StatusCreated, j)
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "Shop containers", "kind": "container", "scope_element_id": shop.Id}, &container), http.StatusCreated, j)
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "Picks", "kind": "custom", "elements": []string{apiEl.Id.String()}}, &custom), http.StatusCreated, j)

	var list api.ViewList
	c.expect(c.do("GET", "/api/v1/views", viewer, nil, &list), http.StatusOK, j)
	if len(list.Views) != 3 {
		t.Fatalf("views = %d", len(list.Views))
	}

	// What each view shows.
	var m api.ViewModel
	c.expect(c.do("GET", "/api/v1/views/"+landscape.Id.String()+"/model", viewer, nil, &m), http.StatusOK, j)
	// Landscape roots: Customer, Payment Provider and Shop (nodes are infrastructure, not landscape).
	if len(m.Roots) != 3 || len(m.Expanded) != 0 || m.ScopeMissing {
		t.Fatalf("landscape: roots=%d expanded=%d scope_missing=%v", len(m.Roots), len(m.Expanded), m.ScopeMissing)
	}
	c.expect(c.do("GET", "/api/v1/views/"+container.Id.String()+"/model", viewer, nil, &m), http.StatusOK, j)
	if !slices.Contains(m.Expanded, shop.Id) || len(m.Roots) != 3 {
		t.Fatalf("container view: expanded=%v roots=%d", m.Expanded, len(m.Roots))
	}
	c.expect(c.do("GET", "/api/v1/views/"+custom.Id.String()+"/model", viewer, nil, &m), http.StatusOK, j)
	if len(m.Roots) != 1 || m.Roots[0] != apiEl.Id {
		t.Fatalf("custom roots = %v", m.Roots)
	}
	if len(m.Layout) != 1 || m.Layout[0].Included == nil || !*m.Layout[0].Included {
		t.Fatalf("custom membership not reported: %+v", m.Layout)
	}
	c.expect(c.do("GET", "/api/v1/views/00000000-0000-7000-8000-0000000000aa/model", viewer, nil, nil), http.StatusNotFound, p)

	// Layout: saved, replaced (not merged), read back; viewers cannot save.
	layoutURL := "/api/v1/views/" + container.Id.String() + "/layout"
	c.expect(c.do("PUT", layoutURL, viewer, obj{"elements": []obj{}}, nil), http.StatusForbidden, p)
	var saved api.ViewLayout
	c.expect(c.do("PUT", layoutURL, editor, obj{"elements": []obj{
		{"element_id": apiEl.Id, "x": 12.5, "y": 40, "collapsed": true},
		{"element_id": shop.Id, "collapsed": false},
	}}, &saved), http.StatusOK, j)
	if len(saved.Elements) != 2 {
		t.Fatalf("saved = %+v", saved.Elements)
	}
	c.expect(c.do("PUT", layoutURL, editor, obj{"elements": []obj{
		{"element_id": apiEl.Id, "x": 99, "y": 1, "collapsed": true},
	}}, &saved), http.StatusOK, j)
	if len(saved.Elements) != 1 || saved.Elements[0].X == nil || *saved.Elements[0].X != 99 {
		t.Fatalf("second save must replace the first: %+v", saved.Elements)
	}
	c.expect(c.do("GET", "/api/v1/views/"+container.Id.String()+"/model", viewer, nil, &m), http.StatusOK, j)
	if len(m.Layout) != 1 || m.Layout[0].ElementId != apiEl.Id {
		t.Fatalf("layout read back = %+v", m.Layout)
	}
	// Members of a custom view survive a layout save.
	c.expect(c.do("PUT", "/api/v1/views/"+custom.Id.String()+"/layout", editor, obj{"elements": []obj{
		{"element_id": apiEl.Id, "x": 5, "y": 6, "collapsed": true},
	}}, nil), http.StatusOK, j)
	c.expect(c.do("GET", "/api/v1/views/"+custom.Id.String()+"/model", viewer, nil, &m), http.StatusOK, j)
	if len(m.Roots) != 1 || m.Layout[0].X == nil {
		t.Fatalf("custom view after layout save: roots=%v layout=%+v", m.Roots, m.Layout)
	}

	// Update / delete rules.
	c.expect(c.do("PUT", "/api/v1/views/"+landscape.Id.String(), "viewer@test.local", obj{"name": "n", "kind": "landscape"}, nil), http.StatusForbidden, p)
	var renamed api.View
	c.expect(c.do("PUT", "/api/v1/views/"+landscape.Id.String(), maintainer, obj{"name": "Renamed", "kind": "landscape", "description": "d"}, &renamed), http.StatusOK, j)
	if renamed.Name != "Renamed" || renamed.Description != "d" {
		t.Fatalf("renamed = %+v", renamed)
	}
	c.expect(c.do("DELETE", "/api/v1/views/"+custom.Id.String(), viewer, nil, nil), http.StatusForbidden, p)
	c.expect(c.do("DELETE", "/api/v1/views/"+custom.Id.String(), editor, nil, nil), http.StatusNoContent, "")
	c.expect(c.do("GET", "/api/v1/views/"+custom.Id.String()+"/model", viewer, nil, nil), http.StatusNotFound, p)

	// None of that touched the model, the change log or the version history.
	if after := mainState(t, c); after != before {
		t.Fatalf("views changed model state:\nbefore=%s\nafter=%s", before, after)
	}

	// A view over a draft shows the draft's elements; main does not.
	var cs api.Changeset
	c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": "d"}, &cs), http.StatusCreated, j)
	c.expect(c.do("POST", "/api/v1/changesets/"+cs.Id.String()+"/changes", editor, obj{"changes": []obj{
		{"op": "add", "target_type": "element", "patch": obj{"kind": "system", "name": "Draft System"}},
	}}, nil), http.StatusCreated, j)
	landscapeURL := "/api/v1/views/" + landscape.Id.String() + "/model"
	hasDraft := func(url string) bool {
		var vm api.ViewModel
		c.expect(c.do("GET", url, viewer, nil, &vm), http.StatusOK, j)
		return slices.ContainsFunc(vm.Elements, func(e api.Element) bool { return e.Name == "Draft System" })
	}
	if hasDraft(landscapeURL) || !hasDraft(landscapeURL+"?changeset_id="+cs.Id.String()) {
		t.Fatal("main and draft views must differ")
	}
	c.expect(c.do("GET", landscapeURL+"?changeset_id=00000000-0000-7000-8000-0000000000aa", viewer, nil, nil), http.StatusNotFound, p)
}

// mainState fingerprints everything views must never change: elements,
// relations, versions and the change log.
func mainState(t *testing.T, c *client) string {
	t.Helper()
	var out string
	for _, path := range []string{"/api/v1/model/elements", "/api/v1/model/relations", "/api/v1/versions", "/api/v1/changesets"} {
		rec := c.do("GET", path, "viewer@test.local", nil, nil)
		c.expect(rec, http.StatusOK, "application/json")
		out += rec.Body.String()
	}
	return out
}
