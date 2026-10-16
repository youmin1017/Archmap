package changeset_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"archmap/api/internal/changeset"
	changesetrepo "archmap/api/internal/changeset/repository"
	"archmap/api/internal/model"
	modelrepo "archmap/api/internal/model/repository"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/seed"
	"archmap/api/internal/testdb"
)

type integration struct {
	pool   *pgxpool.Pool
	svc    *changeset.Service
	main   *model.Service
	actors map[authctx.Role]authctx.Actor
}

// newIntegration starts a migrated database with the sample model as main version 1.
func newIntegration(t *testing.T) *integration {
	t.Helper()
	pool := testdb.Pool(t)
	it := &integration{
		pool:   pool,
		svc:    changeset.NewService(changesetrepo.New(pool)),
		main:   model.NewService(modelrepo.New(pool)),
		actors: testdb.Actors(t, pool),
	}
	cs, err := it.svc.Bootstrap(it.as(authctx.RoleMaintainer), "Sample model", seed.SampleModel())
	if err != nil {
		t.Fatal(err)
	}
	if cs.Status != changeset.StatusMerged {
		t.Fatalf("bootstrap status = %s", cs.Status)
	}
	return it
}

func (it *integration) as(role authctx.Role) context.Context {
	return authctx.With(context.Background(), it.actors[role])
}

func (it *integration) mainElementByName(t *testing.T, name string) model.Element {
	t.Helper()
	els, err := it.main.ListElements(it.as(authctx.RoleViewer), model.ElementFilter{Query: name})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range els {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("element %q not found in main", name)
	return model.Element{}
}

func findByName(s model.Snapshot, name string) (model.Element, bool) {
	for _, e := range s.Elements {
		if e.Name == name {
			return e, true
		}
	}
	return model.Element{}, false
}

func TestIntegrationBootstrapSample(t *testing.T) {
	it := newIntegration(t)
	ctx := it.as(authctx.RoleViewer)

	els, err := it.main.ListElements(ctx, model.ElementFilter{})
	if err != nil {
		t.Fatal(err)
	}
	rels, err := it.main.ListRelations(ctx, model.RelationFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(els) != 12 || len(rels) != 12 {
		t.Fatalf("sample has %d elements / %d relations, want 12 / 12", len(els), len(rels))
	}

	// Deployment tree: physical host -> docker host -> nginx.
	nginx := it.mainElementByName(t, "Nginx")
	docker := it.mainElementByName(t, "Docker Host")
	host := it.mainElementByName(t, "Physical Host")
	if nginx.ParentID == nil || *nginx.ParentID != docker.ID || docker.ParentID == nil || *docker.ParentID != host.ID {
		t.Fatal("deployment tree is not host -> docker -> nginx")
	}

	// Filtering by parent.
	shop := it.mainElementByName(t, "Shop")
	children, err := it.main.ListElements(ctx, model.ElementFilter{ParentID: &shop.ID})
	if err != nil || len(children) != 5 {
		t.Fatalf("shop children = %d (err=%v), want 5", len(children), err)
	}

	// A second bootstrap is refused.
	if _, err := it.svc.Bootstrap(it.as(authctx.RoleMaintainer), "again", seed.SampleModel()); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("second bootstrap err = %v", err)
	}

	// Main reads require an actor.
	if _, err := it.main.ListElements(context.Background(), model.ElementFilter{}); apperr.KindOf(err) != apperr.KindUnauthenticated {
		t.Fatalf("anonymous read err = %v", err)
	}
}

// DoD: add / update / delete inside a draft never touch main.
func TestIntegrationDraftDoesNotTouchMain(t *testing.T) {
	it := newIntegration(t)
	editor := it.as(authctx.RoleEditor)
	before := testdb.MainDump(t, it.pool)

	shop := it.mainElementByName(t, "Shop")
	api := it.mainElementByName(t, "API")
	worker := it.mainElementByName(t, "Worker")
	host := it.mainElementByName(t, "Physical Host")

	cs, err := it.svc.Create(editor, "Rework", "integration test")
	if err != nil {
		t.Fatal(err)
	}
	if cs.BaseVersion != 1 || cs.Status != changeset.StatusDraft {
		t.Fatalf("draft = %+v", cs)
	}

	cacheID := uuid.Must(uuid.NewV7())
	batches := [][]changeset.ChangeInput{
		{ // add
			{Op: changeset.OpAdd, TargetType: changeset.TargetElement, TargetID: cacheID,
				Patch: raw(obj{"kind": "container", "name": "Cache", "technology": "Redis", "parent_id": shop.ID})},
			{Op: changeset.OpAdd, TargetType: changeset.TargetRelation,
				Patch: raw(obj{"kind": "uses", "source_id": api.ID, "target_id": cacheID, "technology": "RESP"})},
		},
		{ // update
			{Op: changeset.OpUpdate, TargetType: changeset.TargetElement, TargetID: api.ID,
				Patch: raw(obj{"name": "Public API", "tags": []string{"backend", "public"}})},
		},
		{ // delete (relations of the worker go with it)
			{Op: changeset.OpDelete, TargetType: changeset.TargetElement, TargetID: worker.ID},
		},
		{ // cascade delete of the whole deployment tree
			{Op: changeset.OpDelete, TargetType: changeset.TargetElement, TargetID: host.ID, Cascade: true},
		},
	}
	for i, b := range batches {
		if _, err := it.svc.AddChanges(editor, cs.ID, b); err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}
	}

	// Main is byte-identical.
	if after := testdb.MainDump(t, it.pool); after != before {
		t.Fatalf("main changed by draft edits:\nbefore=%s\nafter=%s", before, after)
	}
	if e := it.mainElementByName(t, "API"); e.Name != "API" {
		t.Fatal("main API was renamed")
	}

	// The effective model reflects the draft.
	eff, err := it.svc.EffectiveModel(editor, cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findByName(eff, "Cache"); !ok {
		t.Error("Cache missing from effective model")
	}
	if e, ok := findByName(eff, "Public API"); !ok || len(e.Tags) != 2 {
		t.Errorf("renamed API missing or wrong tags: %+v", e)
	}
	for _, gone := range []string{"API", "Worker", "Physical Host", "Docker Host", "Nginx", "Traefik"} {
		if _, ok := findByName(eff, gone); ok {
			t.Errorf("%q should be deleted in the effective model", gone)
		}
	}
	for _, r := range eff.Relations {
		if r.SourceID == worker.ID || r.TargetID == worker.ID || r.Kind == model.RelDeployedOn {
			t.Errorf("dangling or deployment relation survived: %+v", r)
		}
	}

	// The change log recorded explicit deletes with before-snapshots.
	detail, err := it.svc.Get(editor, cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	var deletes int
	for _, c := range detail.Changes {
		if c.Op == changeset.OpDelete {
			deletes++
			if len(c.Before) == 0 {
				t.Errorf("delete change %d has no before snapshot", c.Seq)
			}
		}
	}
	if deletes < 10 { // worker + its 2 relations + host tree (4 nodes, 5 deployed_on relations)
		t.Errorf("only %d delete changes recorded", deletes)
	}
}

func TestIntegrationUndoRedoPersists(t *testing.T) {
	it := newIntegration(t)
	editor := it.as(authctx.RoleEditor)
	api := it.mainElementByName(t, "API")

	cs, err := it.svc.Create(editor, "Undo me", "")
	if err != nil {
		t.Fatal(err)
	}
	rename := func(name string) {
		t.Helper()
		if _, err := it.svc.AddChanges(editor, cs.ID, []changeset.ChangeInput{{
			Op: changeset.OpUpdate, TargetType: changeset.TargetElement, TargetID: api.ID, Patch: raw(obj{"name": name}),
		}}); err != nil {
			t.Fatal(err)
		}
	}
	apiName := func() string {
		t.Helper()
		eff, err := it.svc.EffectiveModel(editor, cs.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range eff.Elements {
			if e.ID == api.ID {
				return e.Name
			}
		}
		t.Fatal("API vanished")
		return ""
	}

	rename("V1")
	rename("V2")
	if got := apiName(); got != "V2" {
		t.Fatalf("name = %q", got)
	}
	if _, err := it.svc.Undo(editor, cs.ID); err != nil {
		t.Fatal(err)
	}
	if got := apiName(); got != "V1" {
		t.Fatalf("after undo name = %q", got)
	}
	if _, err := it.svc.Redo(editor, cs.ID); err != nil {
		t.Fatal(err)
	}
	if got := apiName(); got != "V2" {
		t.Fatalf("after redo name = %q", got)
	}
	if _, err := it.svc.Undo(editor, cs.ID); err != nil {
		t.Fatal(err)
	}
	rename("V3") // discards the undone V2
	detail, err := it.svc.Get(editor, cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Changes) != 2 || apiName() != "V3" {
		t.Fatalf("changes=%d name=%q", len(detail.Changes), apiName())
	}
	if _, err := it.svc.Redo(editor, cs.ID); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("redo after new batch err = %v", err)
	}
	if got := it.mainElementByName(t, "API").Name; got != "API" {
		t.Fatalf("main API name = %q", got)
	}
}

func TestIntegrationPermissions(t *testing.T) {
	it := newIntegration(t)

	if _, err := it.svc.Create(it.as(authctx.RoleViewer), "nope", ""); apperr.KindOf(err) != apperr.KindForbidden {
		t.Fatalf("viewer create err = %v", err)
	}
	cs, err := it.svc.Create(it.as(authctx.RoleEditor), "mine", "")
	if err != nil {
		t.Fatal(err)
	}
	add := []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetElement,
		Patch: raw(obj{"kind": "system", "name": "X"})}}
	if _, err := it.svc.AddChanges(it.as(authctx.RoleViewer), cs.ID, add); apperr.KindOf(err) != apperr.KindForbidden {
		t.Fatalf("viewer edit err = %v", err)
	}
	if _, err := it.svc.AddChanges(it.as(authctx.RoleMaintainer), cs.ID, add); err != nil {
		t.Fatalf("maintainer edit err = %v", err)
	}
	// Viewers can read drafts.
	if _, err := it.svc.EffectiveModel(it.as(authctx.RoleViewer), cs.ID); err != nil {
		t.Fatalf("viewer read err = %v", err)
	}
	if _, err := it.svc.Get(it.as(authctx.RoleViewer), uuid.New()); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("unknown changeset err = %v", err)
	}
}

// Concurrent edits of one draft must serialize on the changeset row lock and
// produce a gap-free, duplicate-free seq.
func TestIntegrationConcurrentAddChanges(t *testing.T) {
	it := newIntegration(t)
	editor := it.as(authctx.RoleEditor)
	cs, err := it.svc.Create(editor, "parallel", "")
	if err != nil {
		t.Fatal(err)
	}

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := it.svc.AddChanges(editor, cs.ID, []changeset.ChangeInput{{
				Op: changeset.OpAdd, TargetType: changeset.TargetElement,
				Patch: raw(obj{"kind": "system", "name": string(rune('A' + i))}),
			}})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	detail, err := it.svc.Get(editor, cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Changes) != n {
		t.Fatalf("%d changes, want %d", len(detail.Changes), n)
	}
	for i, c := range detail.Changes {
		if c.Seq != i+1 {
			t.Fatalf("seq[%d] = %d", i, c.Seq)
		}
	}
}

func TestIntegrationListChangesetsFilters(t *testing.T) {
	it := newIntegration(t)
	editor := it.as(authctx.RoleEditor)
	if _, err := it.svc.Create(editor, "one", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := it.svc.Create(it.as(authctx.RoleMaintainer), "two", ""); err != nil {
		t.Fatal(err)
	}
	draft, merged := changeset.StatusDraft, changeset.StatusMerged
	authorID := it.actors[authctx.RoleEditor].ID

	tests := []struct {
		name string
		f    changeset.Filter
		want int
	}{
		{"all", changeset.Filter{}, 3}, // + the bootstrap changeset
		{"drafts", changeset.Filter{Status: &draft}, 2},
		{"merged", changeset.Filter{Status: &merged}, 1},
		{"by author", changeset.Filter{AuthorID: &authorID}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := it.svc.List(editor, tc.f)
			if err != nil || len(got) != tc.want {
				t.Fatalf("got %d (err=%v), want %d", len(got), err, tc.want)
			}
		})
	}
}
