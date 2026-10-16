package apiserver_test

import (
	"net/http"
	"slices"
	"testing"

	"archmap/api/internal/gen/api"
)

func TestHTTPLayersView(t *testing.T) {
	c, editor := newClient(t)
	const viewer = "viewer@test.local"
	j, p := "application/json", "application/problem+json"

	// The seeded sample assigns layers to its elements.
	var els api.ElementList
	c.expect(c.do("GET", "/api/v1/model/elements", viewer, nil, &els), http.StatusOK, j)
	layerOf := map[string]string{}
	var apiEl api.Element
	for _, e := range els.Elements {
		layerOf[e.Name] = e.Layer
		if e.Name == "API" {
			apiEl = e
		}
	}
	if layerOf["Customer"] != "User" || layerOf["API"] != "Services" || layerOf["Physical Host"] != "Physical" || layerOf["Shop"] != "" {
		t.Fatalf("sample layers = %v", layerOf)
	}

	// Validation.
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "x", "kind": "layers"}, nil), http.StatusBadRequest, p)                               // no layers
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "x", "kind": "layers", "layers": []string{"A", "a"}}, nil), http.StatusBadRequest, p) // duplicate
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "x", "kind": "landscape", "layers": []string{"A"}}, nil), http.StatusBadRequest, p)   // wrong kind
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "x", "kind": "layers", "layers": []string{""}}, nil), http.StatusBadRequest, p)       // spec: minLength
	c.expect(c.do("POST", "/api/v1/views", viewer, obj{"name": "x", "kind": "layers", "layers": []string{"A"}}, nil), http.StatusForbidden, p)

	var v api.View
	c.expect(c.do("POST", "/api/v1/views", editor, obj{"name": "Stack", "kind": "layers", "layers": []string{" User", "Services", "Physical"}}, &v), http.StatusCreated, j)
	if !slices.Equal(v.Layers, []string{"User", "Services", "Physical"}) {
		t.Fatalf("layers = %v", v.Layers)
	}

	// The view shows every element; layers are returned with the view.
	var m api.ViewModel
	c.expect(c.do("GET", "/api/v1/views/"+v.Id.String()+"/model", viewer, nil, &m), http.StatusOK, j)
	if len(m.Elements) != len(els.Elements) || !slices.Equal(m.View.Layers, v.Layers) {
		t.Fatalf("model: %d elements, layers %v", len(m.Elements), m.View.Layers)
	}

	// Moving an element to another layer is an ordinary draft change: main is untouched until merge.
	var cs api.Changeset
	c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": "Move API"}, &cs), http.StatusCreated, j)
	changes := "/api/v1/changesets/" + cs.Id.String() + "/changes"
	c.expect(c.do("POST", changes, editor, obj{"changes": []obj{
		{"op": "update", "target_type": "element", "target_id": apiEl.Id, "patch": obj{"layer": "Kubernetes"}},
	}}, nil), http.StatusCreated, j)
	c.expect(c.do("POST", changes, editor, obj{"changes": []obj{
		{"op": "update", "target_type": "element", "target_id": apiEl.Id, "patch": obj{"layer": "a\nb"}},
	}}, nil), http.StatusBadRequest, p)

	layerIn := func(url string) string {
		var vm api.ViewModel
		c.expect(c.do("GET", url, viewer, nil, &vm), http.StatusOK, j)
		for _, e := range vm.Elements {
			if e.Id == apiEl.Id {
				return e.Layer
			}
		}
		return "missing"
	}
	base := "/api/v1/views/" + v.Id.String() + "/model"
	if got := layerIn(base); got != "Services" {
		t.Fatalf("main layer = %q", got)
	}
	if got := layerIn(base + "?changeset_id=" + cs.Id.String()); got != "Kubernetes" {
		t.Fatalf("draft layer = %q", got)
	}

	// The diff reports the layer field.
	var d api.Diff
	c.expect(c.do("GET", "/api/v1/changesets/"+cs.Id.String()+"/diff", viewer, nil, &d), http.StatusOK, j)
	if len(d.Elements.Modified) != 1 || d.Elements.Modified[0].Fields[0].Field != "layer" {
		t.Fatalf("diff = %+v", d.Elements.Modified)
	}
}
