package changeset_test

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	changesetrepo "archmap/api/internal/changeset/repository"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/testdb"
)

// land submits as the author, approves and merges as the maintainer.
func (it *integration) land(t *testing.T, cs changeset.Changeset, author authctx.Role) changeset.MergeResult {
	t.Helper()
	if _, err := it.svc.Submit(it.as(author), cs.ID); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := it.svc.Approve(it.as(authctx.RoleMaintainer), cs.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	res, err := it.svc.Merge(it.as(authctx.RoleMaintainer), cs.ID)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	return res
}

func (it *integration) draft(t *testing.T, title string, inputs ...changeset.ChangeInput) changeset.Changeset {
	t.Helper()
	editor := it.as(authctx.RoleEditor)
	cs, err := it.svc.Create(editor, title, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := it.svc.AddChanges(editor, cs.ID, inputs); err != nil {
		t.Fatal(err)
	}
	return cs
}

func (it *integration) mainNames(t *testing.T) []string {
	t.Helper()
	els, err := it.main.ListElements(it.as(authctx.RoleViewer), model.ElementFilter{})
	if err != nil {
		t.Fatal(err)
	}
	return elementNames(model.Snapshot{Elements: els})
}

func TestIntegrationMergeFastForwardAndRebase(t *testing.T) {
	it := newIntegration(t)
	api := it.mainElementByName(t, "API")
	worker := it.mainElementByName(t, "Worker")
	shop := it.mainElementByName(t, "Shop")

	a := it.draft(t, "Rename API + add cache",
		updateEl(api.ID, obj{"name": "Public API"}),
		addEl(uuid.New(), obj{"kind": "container", "name": "Cache", "technology": "Redis", "parent_id": shop.ID}),
	)
	b := it.draft(t, "Rename worker", updateEl(worker.ID, obj{"name": "Job Worker"}), updateEl(api.ID, obj{"description": "edge"}))

	// The sample draft has no lint errors (warnings for the undeployed cache are fine).
	report, err := it.svc.Lint(it.as(authctx.RoleViewer), a.ID)
	if err != nil || report.Errors() != 0 || report.Warnings() == 0 {
		t.Fatalf("lint: errors=%d warnings=%d err=%v", report.Errors(), report.Warnings(), err)
	}

	res := it.land(t, a, authctx.RoleEditor) // fast-forward: main was at v1
	if res.Version != 2 || res.Changeset.Status != changeset.StatusMerged || res.Changeset.MergedAt == nil {
		t.Fatalf("merge a = %+v", res)
	}

	// B is based on v1 while main is at v2: stale but conflict-free (disjoint edits).
	d, err := it.svc.Diff(it.as(authctx.RoleViewer), b.ID)
	if err != nil || d.BaseVersion != 1 || d.MainVersion != 2 {
		t.Fatalf("diff versions = %d/%d err=%v", d.BaseVersion, d.MainVersion, err)
	}
	res = it.land(t, b, authctx.RoleEditor) // 3-way rebase
	if res.Version != 3 {
		t.Fatalf("merge b version = %d", res.Version)
	}

	got := it.mainNames(t)
	want := []string{"Cache", "Customer", "Docker Host", "Job Worker", "Message Queue", "Nginx", "Orders DB", "Payment Provider",
		"Payments DB", "Physical Host", "PostgreSQL", "Public API", "Shop", "Traefik", "Web Frontend"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("main = %v\nwant   %v", got, want)
	}
	if e := it.mainElementByName(t, "Public API"); e.Description != "edge" || e.Version != 3 {
		t.Fatalf("API after both merges = %+v (want B's description and version 3)", e)
	}

	// History: three versions, each snapshot is its own point in time.
	vs, err := it.svc.Versions(it.as(authctx.RoleViewer))
	if err != nil || len(vs) != 3 || vs[0].Number != 3 || vs[2].Number != 1 {
		t.Fatalf("versions = %+v err=%v", vs, err)
	}
	for n, wantCache := range map[int]bool{1: false, 2: true, 3: true} {
		snap, err := it.svc.ModelAt(it.as(authctx.RoleViewer), n)
		if err != nil {
			t.Fatal(err)
		}
		if _, hasCache := findByName(snap, "Cache"); hasCache != wantCache {
			t.Fatalf("v%d has Cache = %v, want %v", n, hasCache, wantCache)
		}
	}
	if v1, _ := it.svc.ModelAt(it.as(authctx.RoleViewer), 1); len(v1.Elements) != 14 {
		t.Fatalf("v1 has %d elements", len(v1.Elements))
	}
	if _, err := it.svc.ModelAt(it.as(authctx.RoleViewer), 9); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("unknown version err = %v", err)
	}
	latest, _ := it.svc.ModelAt(it.as(authctx.RoleViewer), 3)
	mainEls, _ := it.main.ListElements(it.as(authctx.RoleViewer), model.ElementFilter{})
	if len(latest.Elements) != len(mainEls) {
		t.Fatalf("snapshot v3 has %d elements, main %d", len(latest.Elements), len(mainEls))
	}
}

func TestIntegrationConflictsAreResolvedAndPersisted(t *testing.T) {
	it := newIntegration(t)
	api := it.mainElementByName(t, "API")

	a := it.draft(t, "A", updateEl(api.ID, obj{"name": "Theirs"}))
	b := it.draft(t, "B", updateEl(api.ID, obj{"name": "Mine"}))
	it.land(t, a, authctx.RoleEditor)

	if _, err := it.svc.Submit(it.as(authctx.RoleEditor), b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := it.svc.Approve(it.as(authctx.RoleMaintainer), b.ID); err != nil {
		t.Fatal(err)
	}
	before := testdb.MainDump(t, it.pool)
	res, err := it.svc.Merge(it.as(authctx.RoleMaintainer), b.ID)
	if err != nil || len(res.Conflicts) != 1 || res.Conflicts[0].Field != "name" {
		t.Fatalf("merge = %+v err=%v", res, err)
	}
	if testdb.MainDump(t, it.pool) != before {
		t.Fatal("a conflicted merge must not change main")
	}

	key := changeset.ResolutionKey{TargetType: changeset.TargetElement, TargetID: api.ID, Field: "name"}
	remaining, err := it.svc.Resolve(it.as(authctx.RoleEditor), b.ID, []changeset.Resolution{{ResolutionKey: key, Choice: changeset.ChoiceMine}})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("resolve: %+v err=%v", remaining, err)
	}
	// A fresh service instance (no in-memory state) sees the stored resolution.
	fresh := changeset.NewService(changesetrepo.New(it.pool))
	if c, err := fresh.Conflicts(it.as(authctx.RoleViewer), b.ID); err != nil || len(c) != 0 {
		t.Fatalf("conflicts after resolve = %+v err=%v", c, err)
	}
	res, err = fresh.Merge(it.as(authctx.RoleMaintainer), b.ID)
	if err != nil || res.Version != 3 {
		t.Fatalf("merge after resolve = %+v err=%v", res, err)
	}
	if got := it.mainElementByName(t, "Mine"); got.ID != api.ID {
		t.Fatalf("resolved element = %+v", got)
	}
}

// Concurrent merges are serialized by the main lock: every draft lands with its own version.
func TestIntegrationConcurrentMerges(t *testing.T) {
	it := newIntegration(t)
	shop := it.mainElementByName(t, "Shop")

	const n = 4
	drafts := make([]changeset.Changeset, n)
	for i := range n {
		drafts[i] = it.draft(t, "add", addEl(uuid.New(), obj{"kind": "container", "name": string(rune('K' + i)), "parent_id": shop.ID}))
		if _, err := it.svc.Submit(it.as(authctx.RoleEditor), drafts[i].ID); err != nil {
			t.Fatal(err)
		}
		if _, err := it.svc.Approve(it.as(authctx.RoleMaintainer), drafts[i].ID); err != nil {
			t.Fatal(err)
		}
	}

	versions := make(chan int, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for _, d := range drafts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := it.svc.Merge(it.as(authctx.RoleMaintainer), d.ID)
			if err == nil && len(res.Conflicts) > 0 {
				err = apperr.Conflict("unexpected conflicts: %+v", res.Conflicts)
			}
			errs <- err
			versions <- res.Version
		}()
	}
	wg.Wait()
	close(errs)
	close(versions)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[int]bool{}
	for v := range versions {
		seen[v] = true
	}
	if len(seen) != n || !seen[2] || !seen[5] {
		t.Fatalf("versions = %v, want 2..5", seen)
	}
	if got := len(it.mainNames(t)); got != 14+n {
		t.Fatalf("main has %d elements, want %d", got, 14+n)
	}
}

func TestIntegrationProtectedElements(t *testing.T) {
	it := newIntegration(t)
	api := it.mainElementByName(t, "API")

	// A second maintainer, so one can author and another can review.
	m2Ctx := authctx.With(context.Background(), testdb.User(t, it.pool, "maintainer2@test.local", authctx.RoleMaintainer))

	// Editors cannot protect elements; a maintainer draft can, merged by another maintainer.
	_, err := it.svc.AddChanges(it.as(authctx.RoleEditor), it.draft(t, "x", updateEl(api.ID, obj{"name": "API"})).ID,
		[]changeset.ChangeInput{updateEl(api.ID, obj{"protected": true})})
	if apperr.KindOf(err) != apperr.KindForbidden {
		t.Fatalf("editor protecting err = %v", err)
	}
	mine, err := it.svc.Create(m2Ctx, "Protect API", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := it.svc.AddChanges(m2Ctx, mine.ID, []changeset.ChangeInput{updateEl(api.ID, obj{"protected": true})}); err != nil {
		t.Fatal(err)
	}
	if _, err := it.svc.Submit(m2Ctx, mine.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := it.svc.Approve(it.as(authctx.RoleMaintainer), mine.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := it.svc.Merge(it.as(authctx.RoleMaintainer), mine.ID); err != nil {
		t.Fatal(err)
	}
	if !it.mainElementByName(t, "API").Protected {
		t.Fatal("API should be protected now")
	}

	// An editor change to the protected element goes through approval and merge by a maintainer.
	edit := it.draft(t, "Edit protected", updateEl(api.ID, obj{"description": "reviewed"}))
	it.land(t, edit, authctx.RoleEditor)
	if it.mainElementByName(t, "API").Description != "reviewed" {
		t.Fatal("protected element not updated by the reviewed changeset")
	}

	// A maintainer cannot slip their own protected change through, even with self-merge enabled.
	selfSvc := changeset.NewService(changesetrepo.New(it.pool), changeset.WithSelfMerge(true))
	own, err := selfSvc.Create(m2Ctx, "Own edit", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selfSvc.AddChanges(m2Ctx, own.ID, []changeset.ChangeInput{updateEl(api.ID, obj{"description": "sneaky"})}); err != nil {
		t.Fatal(err)
	}
	if _, err := selfSvc.Submit(m2Ctx, own.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := selfSvc.Approve(m2Ctx, own.ID); err != nil {
		t.Fatalf("self-approve with flag: %v", err)
	}
	if _, err := selfSvc.Merge(m2Ctx, own.ID); apperr.KindOf(err) != apperr.KindForbidden {
		t.Fatalf("self-merge of protected change err = %v", err)
	}
	if it.mainElementByName(t, "API").Description != "reviewed" {
		t.Fatal("protected element changed by a blocked merge")
	}
}
