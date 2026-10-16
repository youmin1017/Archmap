package view_test

import (
	"context"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/view"
)

type memRepo struct {
	mu       sync.Mutex
	views    map[uuid.UUID]view.View
	elements map[uuid.UUID]map[uuid.UUID]view.Element
}

func newMemRepo() *memRepo {
	return &memRepo{views: map[uuid.UUID]view.View{}, elements: map[uuid.UUID]map[uuid.UUID]view.Element{}}
}

func (m *memRepo) InTx(_ context.Context, fn func(view.Repository) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fn(m)
}

func (m *memRepo) Insert(_ context.Context, v view.View) (view.View, error) {
	v.ID, _ = uuid.NewV7()
	v.CreatedAt = time.Now()
	m.views[v.ID] = v
	m.elements[v.ID] = map[uuid.UUID]view.Element{}
	return v, nil
}

func (m *memRepo) Get(_ context.Context, id uuid.UUID) (view.View, error) {
	v, ok := m.views[id]
	if !ok {
		return view.View{}, apperr.NotFound("view %s not found", id)
	}
	return v, nil
}

func (m *memRepo) List(context.Context) ([]view.View, error) {
	var out []view.View
	for _, v := range m.views {
		out = append(out, v)
	}
	return out, nil
}

func (m *memRepo) Update(_ context.Context, v view.View) (view.View, error) {
	m.views[v.ID] = v
	return v, nil
}

func (m *memRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(m.views, id)
	delete(m.elements, id)
	return nil
}

func (m *memRepo) Elements(_ context.Context, id uuid.UUID) ([]view.Element, error) {
	var out []view.Element
	for _, e := range m.elements[id] {
		out = append(out, e)
	}
	return out, nil
}

func (m *memRepo) SetMembers(_ context.Context, id uuid.UUID, ids []uuid.UUID) error {
	for k, e := range m.elements[id] {
		e.Included = false
		m.elements[id][k] = e
	}
	for _, mid := range ids {
		e := m.elements[id][mid]
		e.ElementID, e.Included, e.Collapsed = mid, true, true
		m.elements[id][mid] = e
	}
	return nil
}

func (m *memRepo) SetLayout(_ context.Context, id uuid.UUID, layout []view.Element) error {
	for k, e := range m.elements[id] {
		e.X, e.Y, e.W, e.H, e.Collapsed = nil, nil, nil, nil, true
		m.elements[id][k] = e
	}
	for _, l := range layout {
		e := m.elements[id][l.ElementID]
		l.Included = e.Included
		m.elements[id][l.ElementID] = l
	}
	return nil
}

type fakeSource struct {
	main   model.Snapshot
	drafts map[uuid.UUID]model.Snapshot
}

func (f fakeSource) Snapshot(_ context.Context, cs *uuid.UUID) (model.Snapshot, error) {
	if cs != nil {
		return f.drafts[*cs], nil
	}
	return f.main, nil
}

func actor(role authctx.Role) authctx.Actor {
	return authctx.Actor{ID: uuid.New(), Email: string(role) + "@t", Name: string(role), Role: role}
}

func as(a authctx.Actor) context.Context { return authctx.With(context.Background(), a) }

func wantKind(t *testing.T, err error, want apperr.Kind) {
	t.Helper()
	if got := apperr.KindOf(err); got != want {
		t.Fatalf("kind = %v, want %v (err=%v)", got, want, err)
	}
}

func fp(f float64) *float64 { return &f }

func TestCreateValidation(t *testing.T) {
	scope := id(shop)
	tests := []struct {
		name string
		in   view.Input
		want apperr.Kind
	}{
		{"landscape ok", view.Input{Name: "L", Kind: view.KindLandscape}, 0},
		{"context with scope ok", view.Input{Name: "C", Kind: view.KindContext, ScopeElementID: &scope}, 0},
		{"deployment scope optional", view.Input{Name: "D", Kind: view.KindDeployment}, 0},
		{"custom with members ok", view.Input{Name: "X", Kind: view.KindCustom, Members: []uuid.UUID{id(api), id(api)}}, 0},
		{"blank name", view.Input{Name: "  ", Kind: view.KindLandscape}, apperr.KindInvalid},
		{"name too long", view.Input{Name: string(make([]rune, 121)) + "x", Kind: view.KindLandscape}, apperr.KindInvalid},
		{"bad kind", view.Input{Name: "x", Kind: "bogus"}, apperr.KindInvalid},
		{"context needs scope", view.Input{Name: "x", Kind: view.KindContext}, apperr.KindInvalid},
		{"landscape forbids scope", view.Input{Name: "x", Kind: view.KindLandscape, ScopeElementID: &scope}, apperr.KindInvalid},
		{"members only for custom", view.Input{Name: "x", Kind: view.KindLandscape, Members: []uuid.UUID{id(api)}}, apperr.KindInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := view.NewService(newMemRepo(), fakeSource{})
			v, err := svc.Create(as(actor(authctx.RoleEditor)), tc.in)
			wantKind(t, err, tc.want)
			if err == nil && v.ID == uuid.Nil {
				t.Fatal("no id")
			}
		})
	}
}

func TestPermissions(t *testing.T) {
	repo := newMemRepo()
	svc := view.NewService(repo, fakeSource{main: sample()})
	creator, other, maintainer, viewer := actor(authctx.RoleEditor), actor(authctx.RoleEditor), actor(authctx.RoleMaintainer), actor(authctx.RoleViewer)

	v, err := svc.Create(as(creator), view.Input{Name: "mine", Kind: view.KindLandscape})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(as(viewer), view.Input{Name: "x", Kind: view.KindLandscape})
	wantKind(t, err, apperr.KindForbidden)
	_, err = svc.Create(context.Background(), view.Input{Name: "x", Kind: view.KindLandscape})
	wantKind(t, err, apperr.KindUnauthenticated)

	// Everyone signed in can read; only editors write layout.
	if list, err := svc.List(as(viewer)); err != nil || len(list) != 1 {
		t.Fatalf("list = %v err=%v", list, err)
	}
	if _, err := svc.Model(as(viewer), v.ID, nil); err != nil {
		t.Fatal(err)
	}
	_, err = svc.SetLayout(as(viewer), v.ID, nil)
	wantKind(t, err, apperr.KindForbidden)
	if _, err := svc.SetLayout(as(other), v.ID, nil); err != nil {
		t.Fatalf("any editor may save layout: %v", err)
	}

	// Rename / delete: creator or maintainer.
	upd := view.Input{Name: "renamed", Kind: view.KindLandscape}
	_, err = svc.Update(as(other), v.ID, upd)
	wantKind(t, err, apperr.KindForbidden)
	if got, err := svc.Update(as(maintainer), v.ID, upd); err != nil || got.Name != "renamed" {
		t.Fatalf("maintainer update = %+v err=%v", got, err)
	}
	wantKind(t, svc.Delete(as(other), v.ID), apperr.KindForbidden)
	if err := svc.Delete(as(creator), v.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Model(as(creator), v.ID, nil)
	wantKind(t, err, apperr.KindNotFound)
}

func TestLayoutValidationAndReplace(t *testing.T) {
	svc := view.NewService(newMemRepo(), fakeSource{main: sample()})
	ctx := as(actor(authctx.RoleEditor))
	v, _ := svc.Create(ctx, view.Input{Name: "L", Kind: view.KindLandscape})

	bad := []struct {
		name   string
		layout []view.Element
	}{
		{"nil id", []view.Element{{}}},
		{"duplicate", []view.Element{{ElementID: id(api)}, {ElementID: id(api)}}},
		{"NaN", []view.Element{{ElementID: id(api), X: fp(math.NaN())}}},
		{"Inf", []view.Element{{ElementID: id(api), Y: fp(math.Inf(1))}}},
		{"negative size", []view.Element{{ElementID: id(api), W: fp(-1)}}},
	}
	for _, tc := range bad {
		_, err := svc.SetLayout(ctx, v.ID, tc.layout)
		if apperr.KindOf(err) != apperr.KindInvalid {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}

	if _, err := svc.SetLayout(ctx, v.ID, []view.Element{
		{ElementID: id(api), X: fp(10), Y: fp(20), Collapsed: true},
		{ElementID: id(shop), Collapsed: false},
	}); err != nil {
		t.Fatal(err)
	}
	// A second save replaces the first: api is gone, web is new.
	saved, err := svc.SetLayout(ctx, v.ID, []view.Element{{ElementID: id(web), X: fp(1), Y: fp(2), Collapsed: true}})
	if err != nil {
		t.Fatal(err)
	}
	withX := 0
	for _, e := range saved {
		if e.X != nil {
			withX++
			if e.ElementID != id(web) {
				t.Fatalf("stale layout kept for %v", e.ElementID)
			}
		}
	}
	if withX != 1 {
		t.Fatalf("saved = %+v", saved)
	}
	_, err = svc.SetLayout(ctx, uuid.New(), nil)
	wantKind(t, err, apperr.KindNotFound)
}

func TestModelUsesMainOrDraftAndCustomMembers(t *testing.T) {
	main := sample()
	// The draft adds a top-level system; main does not have it.
	draft := model.Snapshot{Elements: append(slices.Clone(main.Elements), el(50, model.KindSystem, "DraftOnly", 0)), Relations: main.Relations}
	draftID := uuid.New()
	svc := view.NewService(newMemRepo(), fakeSource{main: main, drafts: map[uuid.UUID]model.Snapshot{draftID: draft}})
	ctx := as(actor(authctx.RoleEditor))

	landscape, _ := svc.Create(ctx, view.Input{Name: "L", Kind: view.KindLandscape})
	inMain, err := svc.Model(ctx, landscape.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	inDraft, err := svc.Model(ctx, landscape.ID, &draftID)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(inMain.Result.Elements, func(e model.Element) bool { return e.Name == "DraftOnly" }) ||
		!slices.ContainsFunc(inDraft.Result.Elements, func(e model.Element) bool { return e.Name == "DraftOnly" }) {
		t.Fatal("main and draft must produce different results")
	}

	custom, err := svc.Create(ctx, view.Input{Name: "C", Kind: view.KindCustom, Members: []uuid.UUID{id(api), id(99)}})
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Model(ctx, custom.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := rootNames(d.Result, main); got != "API" {
		t.Fatalf("custom roots = %q (unknown member ids must be ignored)", got)
	}
	// Updating without members keeps them; with members replaces them.
	if _, err := svc.Update(ctx, custom.ID, view.Input{Name: "C2", Kind: view.KindCustom}); err != nil {
		t.Fatal(err)
	}
	d, _ = svc.Model(ctx, custom.ID, nil)
	if rootNames(d.Result, main) != "API" {
		t.Fatal("members lost by an update without elements")
	}
	if _, err := svc.Update(ctx, custom.ID, view.Input{Name: "C2", Kind: view.KindCustom, Members: []uuid.UUID{id(web)}}); err != nil {
		t.Fatal(err)
	}
	d, _ = svc.Model(ctx, custom.ID, nil)
	if rootNames(d.Result, main) != "Web" {
		t.Fatalf("members not replaced: %q", rootNames(d.Result, main))
	}
}

func TestLayersViews(t *testing.T) {
	svc := view.NewService(newMemRepo(), fakeSource{main: sample()})
	ctx := as(actor(authctx.RoleEditor))

	// Names are trimmed and keep their order (top to bottom).
	v, err := svc.Create(ctx, view.Input{Name: "Stack", Kind: view.KindLayers, Layers: []string{" User ", "Services", "Physical"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(v.Layers, []string{"User", "Services", "Physical"}) {
		t.Fatalf("layers = %v", v.Layers)
	}

	tooMany := make([]string, 13)
	for i := range tooMany {
		tooMany[i] = string(rune('A' + i))
	}
	bad := []struct {
		name string
		in   view.Input
	}{
		{"needs layers", view.Input{Name: "x", Kind: view.KindLayers}},
		{"blank layer", view.Input{Name: "x", Kind: view.KindLayers, Layers: []string{"A", " "}}},
		{"duplicate ignoring case", view.Input{Name: "x", Kind: view.KindLayers, Layers: []string{"Services", "services"}}},
		{"multi-line", view.Input{Name: "x", Kind: view.KindLayers, Layers: []string{"a\nb"}}},
		{"too long", view.Input{Name: "x", Kind: view.KindLayers, Layers: []string{string(make([]rune, 61)) + "x"}}},
		{"too many", view.Input{Name: "x", Kind: view.KindLayers, Layers: tooMany}},
		{"layers only for layers views", view.Input{Name: "x", Kind: view.KindLandscape, Layers: []string{"A"}}},
		{"layers views take no scope", view.Input{Name: "x", Kind: view.KindLayers, Layers: []string{"A"}, ScopeElementID: ptrID(id(shop))}},
	}
	for _, tc := range bad {
		_, err := svc.Create(ctx, tc.in)
		if apperr.KindOf(err) != apperr.KindInvalid {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}

	// A layers view shows every element and relation (nesting is flattened); layers come from the view.
	d, err := svc.Model(ctx, v.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Result.Elements) != len(sample().Elements) || len(d.Result.Relations) != len(sample().Relations) {
		t.Fatalf("elements=%d relations=%d", len(d.Result.Elements), len(d.Result.Relations))
	}

	// Layers can be edited, and a kind change away from layers clears them.
	upd, err := svc.Update(ctx, v.ID, view.Input{Name: "Stack", Kind: view.KindLayers, Layers: []string{"Services", "Data"}})
	if err != nil || !slices.Equal(upd.Layers, []string{"Services", "Data"}) {
		t.Fatalf("update: %+v err=%v", upd, err)
	}
	upd, err = svc.Update(ctx, v.ID, view.Input{Name: "Flat", Kind: view.KindLandscape})
	if err != nil || len(upd.Layers) != 0 {
		t.Fatalf("kind change: %+v err=%v", upd, err)
	}
}

func ptrID(u uuid.UUID) *uuid.UUID { return &u }
