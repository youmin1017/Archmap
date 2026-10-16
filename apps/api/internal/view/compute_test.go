package view_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/model"
	"archmap/api/internal/view"
)

func id(n byte) uuid.UUID { return uuid.UUID{15: n} }

const (
	customer = 1
	payments = 2
	shop     = 3
	web      = 4
	api      = 5
	worker   = 6
	pg       = 7
	mq       = 8
	host     = 9
	docker   = 10
	nginx    = 11
	traefik  = 12
	orphan   = 13 // top-level system with no relations
)

func el(n byte, kind model.ElementKind, name string, parent byte) model.Element {
	e := model.Element{ID: id(n), Kind: kind, Name: name}
	if parent != 0 {
		p := id(parent)
		e.ParentID = &p
	}
	return e
}

func rel(n byte, kind model.RelationKind, src, dst byte) model.Relation {
	return model.Relation{ID: id(n), Kind: kind, SourceID: id(src), TargetID: id(dst)}
}

// The same shape as the dev sample model.
func sample() model.Snapshot {
	return model.Snapshot{
		Elements: []model.Element{
			el(customer, model.KindPerson, "Customer", 0),
			el(payments, model.KindSystem, "Payments", 0),
			el(shop, model.KindSystem, "Shop", 0),
			el(web, model.KindContainer, "Web", shop),
			el(api, model.KindContainer, "API", shop),
			el(worker, model.KindContainer, "Worker", shop),
			el(pg, model.KindContainer, "PostgreSQL", shop),
			el(mq, model.KindContainer, "Queue", shop),
			el(host, model.KindNode, "Host", 0),
			el(docker, model.KindNode, "Docker", host),
			el(nginx, model.KindNode, "Nginx", docker),
			el(traefik, model.KindNode, "Traefik", docker),
			el(orphan, model.KindSystem, "Orphan", 0),
		},
		Relations: []model.Relation{
			rel(101, model.RelUses, customer, web),
			rel(102, model.RelUses, web, api),
			rel(103, model.RelUses, api, pg),
			rel(104, model.RelUses, api, mq),
			rel(105, model.RelUses, worker, mq),
			rel(106, model.RelUses, worker, pg),
			rel(107, model.RelUses, api, payments),
			rel(108, model.RelDeployedOn, web, nginx),
			rel(109, model.RelDeployedOn, api, traefik),
			rel(110, model.RelDeployedOn, worker, docker),
			rel(111, model.RelDeployedOn, pg, docker),
			rel(112, model.RelDeployedOn, mq, docker),
		},
	}
}

func names(r view.Result) string {
	var n []string
	for _, e := range r.Elements {
		n = append(n, e.Name)
	}
	slices.Sort(n)
	return strings.Join(n, ",")
}

func rootNames(r view.Result, snap model.Snapshot) string {
	var n []string
	for _, rid := range r.Roots {
		for _, e := range snap.Elements {
			if e.ID == rid {
				n = append(n, e.Name)
			}
		}
	}
	slices.Sort(n)
	return strings.Join(n, ",")
}

func scoped(kind view.Kind, scope byte) view.View {
	s := id(scope)
	return view.View{Kind: kind, ScopeElementID: &s}
}

func TestCompute(t *testing.T) {
	snap := sample()
	tests := []struct {
		name      string
		v         view.View
		members   []uuid.UUID
		wantRoots string
		wantElems string
		wantRels  int
		wantExp   []uuid.UUID
	}{
		{
			name:      "landscape: top-level people and systems with their subtrees",
			v:         view.View{Kind: view.KindLandscape},
			wantRoots: "Customer,Orphan,Payments,Shop",
			wantElems: "API,Customer,Orphan,Payments,PostgreSQL,Queue,Shop,Web,Worker",
			wantRels:  7, // the five uses relations inside Shop + customer->web + api->payments; deployed_on go to hidden nodes
		},
		{
			name:      "context: scope plus the people and systems it talks to",
			v:         scoped(view.KindContext, shop),
			wantRoots: "Customer,Payments,Shop",
			wantElems: "API,Customer,Payments,PostgreSQL,Queue,Shop,Web,Worker",
			wantRels:  7,
		},
		{
			name:      "context of a system without relations shows only itself",
			v:         scoped(view.KindContext, orphan),
			wantRoots: "Orphan",
			wantElems: "Orphan",
		},
		{
			name:      "container: scope is expanded, externals are neighbours",
			v:         scoped(view.KindContainer, shop),
			wantRoots: "Customer,Payments,Shop",
			wantElems: "API,Customer,Payments,PostgreSQL,Queue,Shop,Web,Worker",
			wantRels:  7,
			wantExp:   []uuid.UUID{id(shop)},
		},
		{
			name: "component: scope container, siblings it uses are neighbours (not their parent system)",
			v:    scoped(view.KindComponent, api),
			// API uses PostgreSQL, Queue and Payments; Web uses API; Shop (API's parent) is never a neighbour.
			wantRoots: "API,Payments,PostgreSQL,Queue,Web",
			wantElems: "API,Payments,PostgreSQL,Queue,Web",
			wantRels:  4,
			wantExp:   []uuid.UUID{id(api)},
		},
		{
			name:      "deployment: node tree plus what is deployed on it",
			v:         view.View{Kind: view.KindDeployment},
			wantRoots: "API,Host,PostgreSQL,Queue,Web,Worker",
			wantElems: "API,Docker,Host,Nginx,PostgreSQL,Queue,Traefik,Web,Worker",
			wantRels:  10, // 5 deployed_on + the 5 uses relations among the deployed containers
			wantExp:   []uuid.UUID{id(host), id(docker)},
		},
		{
			name:      "deployment scoped to one node",
			v:         scoped(view.KindDeployment, nginx),
			wantRoots: "Nginx,Web",
			wantElems: "Nginx,Web",
			wantRels:  1,
		},
		{
			name:      "custom: manual members, unknown ids ignored, nested members collapse into one root",
			v:         view.View{Kind: view.KindCustom},
			members:   []uuid.UUID{id(api), id(shop), id(99), id(customer)},
			wantRoots: "Customer,Shop",
			wantElems: "API,Customer,PostgreSQL,Queue,Shop,Web,Worker",
			wantRels:  6,
		},
		{
			name: "custom without members is empty",
			v:    view.View{Kind: view.KindCustom},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := view.Compute(tc.v, tc.members, snap)
			if r.ScopeMissing {
				t.Fatal("unexpected scope_missing")
			}
			if got := rootNames(r, snap); got != tc.wantRoots {
				t.Errorf("roots = %q, want %q", got, tc.wantRoots)
			}
			if got := names(r); got != tc.wantElems {
				t.Errorf("elements = %q, want %q", got, tc.wantElems)
			}
			if len(r.Relations) != tc.wantRels {
				t.Errorf("relations = %d, want %d", len(r.Relations), tc.wantRels)
			}
			if !sameSet(r.Expanded, tc.wantExp) {
				t.Errorf("expanded = %v, want %v", r.Expanded, tc.wantExp)
			}
			// Every relation connects two shown elements; every non-root has its parent shown.
			shown := map[uuid.UUID]bool{}
			for _, e := range r.Elements {
				shown[e.ID] = true
			}
			for _, rl := range r.Relations {
				if !shown[rl.SourceID] || !shown[rl.TargetID] {
					t.Errorf("relation %v leaves the view", rl.ID)
				}
			}
		})
	}
}

func sameSet(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}

func TestComputeScopeMissing(t *testing.T) {
	snap := sample()
	for _, kind := range []view.Kind{view.KindContext, view.KindContainer, view.KindComponent, view.KindDeployment} {
		r := view.Compute(scoped(kind, 99), nil, snap)
		if !r.ScopeMissing || len(r.Elements) != 0 {
			t.Errorf("%s: result = %+v, want scope_missing", kind, r)
		}
	}
	// A view without scope never reports a missing scope.
	if r := view.Compute(view.View{Kind: view.KindLandscape}, nil, snap); r.ScopeMissing {
		t.Error("landscape must not report a missing scope")
	}
}

func TestComputeIsDeterministicAndHandlesBrokenData(t *testing.T) {
	snap := sample()
	a := view.Compute(view.View{Kind: view.KindLandscape}, nil, snap)
	b := view.Compute(view.View{Kind: view.KindLandscape}, nil, snap)
	if names(a) != names(b) || len(a.Roots) != len(b.Roots) {
		t.Fatal("not deterministic")
	}
	// A cycle in parent links and a dangling parent must not hang or panic.
	cyc := model.Snapshot{Elements: []model.Element{
		func() model.Element { e := el(1, model.KindSystem, "A", 2); return e }(),
		func() model.Element { e := el(2, model.KindSystem, "B", 1); return e }(),
		el(3, model.KindSystem, "Dangling", 99),
	}}
	_ = view.Compute(view.View{Kind: view.KindLandscape}, nil, cyc)
	_ = view.Compute(scoped(view.KindContainer, 1), nil, cyc)
}

func TestKindScopeRules(t *testing.T) {
	cases := map[view.Kind]view.ScopeRule{
		view.KindLandscape:  view.ScopeForbidden,
		view.KindCustom:     view.ScopeForbidden,
		view.KindLayers:     view.ScopeForbidden,
		view.KindContext:    view.ScopeRequired,
		view.KindContainer:  view.ScopeRequired,
		view.KindComponent:  view.ScopeRequired,
		view.KindDeployment: view.ScopeOptional,
	}
	for k, want := range cases {
		if k.ScopeRule() != want || !k.Valid() {
			t.Errorf("%s: rule/valid mismatch", k)
		}
	}
	if view.Kind("bogus").Valid() {
		t.Error("bogus kind must be invalid")
	}
}
