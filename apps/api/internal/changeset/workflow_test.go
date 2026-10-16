package changeset_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/lint"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

// wf is a workflow test bench: one in-memory repo with named actors.
type wf struct {
	t      *testing.T
	repo   *memRepo
	svc    *changeset.Service
	author authctx.Actor // editor
	other  authctx.Actor // another editor
	m1, m2 authctx.Actor // maintainers
	viewer authctx.Actor
}

func newWF(t *testing.T, opts ...changeset.Option) *wf {
	t.Helper()
	repo := newMemRepo(mainModel())
	return &wf{
		t: t, repo: repo, svc: changeset.NewService(repo, opts...),
		author: actor(authctx.RoleEditor), other: actor(authctx.RoleEditor),
		m1: actor(authctx.RoleMaintainer), m2: actor(authctx.RoleMaintainer), viewer: actor(authctx.RoleViewer),
	}
}

func (w *wf) draft(by authctx.Actor, title string, inputs ...changeset.ChangeInput) changeset.Changeset {
	w.t.Helper()
	cs, err := w.svc.Create(as(by), title, "")
	if err != nil {
		w.t.Fatal(err)
	}
	if len(inputs) > 0 {
		if _, err := w.svc.AddChanges(as(by), cs.ID, inputs); err != nil {
			w.t.Fatal(err)
		}
	}
	return cs
}

// land submits, approves (m2) and merges (m1) a changeset, expecting success.
func (w *wf) land(cs changeset.Changeset, author authctx.Actor) changeset.MergeResult {
	w.t.Helper()
	res := w.tryLand(cs, author)
	if len(res.Conflicts) > 0 {
		w.t.Fatalf("unexpected conflicts: %+v", res.Conflicts)
	}
	return res
}

func (w *wf) tryLand(cs changeset.Changeset, author authctx.Actor) changeset.MergeResult {
	w.t.Helper()
	if _, err := w.svc.Submit(as(author), cs.ID); err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.svc.Approve(as(w.m2), cs.ID); err != nil {
		w.t.Fatal(err)
	}
	res, err := w.svc.Merge(as(w.m1), cs.ID)
	if err != nil {
		w.t.Fatal(err)
	}
	return res
}

func updateEl(id uuid.UUID, patch obj) changeset.ChangeInput {
	return changeset.ChangeInput{Op: changeset.OpUpdate, TargetType: changeset.TargetElement, TargetID: id, Patch: raw(patch)}
}

func addEl(id uuid.UUID, patch obj) changeset.ChangeInput {
	return changeset.ChangeInput{Op: changeset.OpAdd, TargetType: changeset.TargetElement, TargetID: id, Patch: raw(patch)}
}

func deleteEl(id uuid.UUID) changeset.ChangeInput {
	return changeset.ChangeInput{Op: changeset.OpDelete, TargetType: changeset.TargetElement, TargetID: id}
}

func (w *wf) mainName(id uuid.UUID) string {
	w.t.Helper()
	for _, e := range w.repo.main.Elements {
		if e.ID == id {
			return e.Name
		}
	}
	return ""
}

func wantKind(t *testing.T, err error, want apperr.Kind) {
	t.Helper()
	if got := apperr.KindOf(err); got != want {
		t.Fatalf("kind = %v, want %v (err=%v)", got, want, err)
	}
}

func TestSubmitApproveRejectAbandon(t *testing.T) {
	rename := updateEl(idAPI, obj{"name": "Gateway"})

	t.Run("submit", func(t *testing.T) {
		w := newWF(t)
		cs := w.draft(w.author, "t", rename)
		_, err := w.svc.Submit(as(w.other), cs.ID)
		wantKind(t, err, apperr.KindForbidden)
		_, err = w.svc.Submit(as(w.viewer), cs.ID)
		wantKind(t, err, apperr.KindForbidden)
		got, err := w.svc.Submit(as(w.author), cs.ID)
		if err != nil || got.Status != changeset.StatusInReview {
			t.Fatalf("submit: %+v %v", got, err)
		}
		_, err = w.svc.Submit(as(w.author), cs.ID)
		wantKind(t, err, apperr.KindConflict)
		_, err = w.svc.AddChanges(as(w.author), cs.ID, []changeset.ChangeInput{rename})
		wantKind(t, err, apperr.KindConflict) // frozen while in review
	})

	t.Run("approve", func(t *testing.T) {
		w := newWF(t)
		cs := w.draft(w.author, "t", rename)
		_, err := w.svc.Approve(as(w.m2), cs.ID)
		wantKind(t, err, apperr.KindConflict) // still a draft
		if _, err := w.svc.Submit(as(w.author), cs.ID); err != nil {
			t.Fatal(err)
		}
		_, err = w.svc.Approve(as(w.author), cs.ID)
		wantKind(t, err, apperr.KindForbidden) // editors cannot approve
		got, err := w.svc.Approve(as(w.m2), cs.ID)
		if err != nil || got.ApprovedBy == nil || *got.ApprovedBy != w.m2.ID {
			t.Fatalf("approve: %+v %v", got, err)
		}
	})

	t.Run("reject", func(t *testing.T) {
		w := newWF(t)
		cs := w.draft(w.author, "t", rename)
		_, err := w.svc.Reject(as(w.m1), cs.ID)
		wantKind(t, err, apperr.KindConflict)
		if _, err := w.svc.Submit(as(w.author), cs.ID); err != nil {
			t.Fatal(err)
		}
		_, err = w.svc.Reject(as(w.author), cs.ID)
		wantKind(t, err, apperr.KindForbidden)
		got, err := w.svc.Reject(as(w.m1), cs.ID)
		if err != nil || got.Status != changeset.StatusRejected {
			t.Fatalf("reject: %+v %v", got, err)
		}
		_, err = w.svc.Merge(as(w.m1), cs.ID)
		wantKind(t, err, apperr.KindConflict)
	})

	t.Run("abandon", func(t *testing.T) {
		w := newWF(t)
		cs := w.draft(w.author, "t", rename)
		_, err := w.svc.Abandon(as(w.other), cs.ID)
		wantKind(t, err, apperr.KindForbidden)
		got, err := w.svc.Abandon(as(w.author), cs.ID)
		if err != nil || got.Status != changeset.StatusAbandoned {
			t.Fatalf("abandon: %+v %v", got, err)
		}
		_, err = w.svc.Abandon(as(w.author), cs.ID)
		wantKind(t, err, apperr.KindConflict)
		if _, err := w.svc.EffectiveModel(as(w.author), cs.ID); apperr.KindOf(err) != apperr.KindConflict {
			t.Fatalf("effective model of abandoned changeset: %v", err)
		}
	})

	t.Run("resubmit clears approval", func(t *testing.T) {
		w := newWF(t)
		cs := w.draft(w.author, "t", rename)
		_, _ = w.svc.Submit(as(w.author), cs.ID)
		_, _ = w.svc.Approve(as(w.m2), cs.ID)
		if got := w.repo.changesets[cs.ID]; got.ApprovedBy == nil {
			t.Fatal("approval not recorded")
		}
	})
}

func TestMergeRequiresApprovalAndMaintainer(t *testing.T) {
	w := newWF(t)
	cs := w.draft(w.author, "t", updateEl(idAPI, obj{"name": "Gateway"}))
	_, _ = w.svc.Submit(as(w.author), cs.ID)

	_, err := w.svc.Merge(as(w.m1), cs.ID)
	wantKind(t, err, apperr.KindConflict) // not approved
	_, _ = w.svc.Approve(as(w.m2), cs.ID)
	_, err = w.svc.Merge(as(w.author), cs.ID)
	wantKind(t, err, apperr.KindForbidden) // editors cannot merge
	_, err = w.svc.Merge(context.Background(), cs.ID)
	wantKind(t, err, apperr.KindUnauthenticated)

	if w.mainName(idAPI) != "API" {
		t.Fatal("main changed before a successful merge")
	}
}

func TestSelfMergeFlag(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[allow], func(t *testing.T) {
			w := newWF(t, changeset.WithSelfMerge(allow))
			cs := w.draft(w.m1, "mine", updateEl(idAPI, obj{"name": "Gateway"}))
			if _, err := w.svc.Submit(as(w.m1), cs.ID); err != nil {
				t.Fatal(err)
			}
			_, err := w.svc.Approve(as(w.m1), cs.ID)
			if !allow {
				wantKind(t, err, apperr.KindForbidden)
				// Another maintainer approves; the author still cannot merge.
				if _, err := w.svc.Approve(as(w.m2), cs.ID); err != nil {
					t.Fatal(err)
				}
				_, err = w.svc.Merge(as(w.m1), cs.ID)
				wantKind(t, err, apperr.KindForbidden)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.svc.Merge(as(w.m1), cs.ID); err != nil {
				t.Fatal(err)
			}
			if w.mainName(idAPI) != "Gateway" {
				t.Fatal("self-merge did not apply")
			}
		})
	}
}

func TestDiff(t *testing.T) {
	w := newWF(t)
	cs := w.draft(w.author, "t",
		updateEl(idAPI, obj{"name": "Gateway", "technology": "Rust"}),
		deleteEl(idDB),
		addEl(idNew, obj{"kind": "container", "name": "Cache", "parent_id": idShop}),
	)
	res, err := w.svc.Diff(as(w.viewer), cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	d := res.Diff
	if len(d.AddedElements) != 1 || d.AddedElements[0].Name != "Cache" {
		t.Fatalf("added = %+v", d.AddedElements)
	}
	if len(d.RemovedElements) != 1 || d.RemovedElements[0].Name != "DB" || len(d.RemovedRelations) != 1 {
		t.Fatalf("removed = %+v / %+v", d.RemovedElements, d.RemovedRelations)
	}
	if len(d.ModifiedElements) != 1 || len(d.ModifiedElements[0].Fields) != 2 {
		t.Fatalf("modified = %+v", d.ModifiedElements)
	}
	f := d.ModifiedElements[0].Fields
	if f[0].Field != "name" || string(f[0].After) != `"Gateway"` || f[1].Field != "technology" || string(f[1].Before) != `"Go"` {
		t.Fatalf("fields = %+v", f)
	}
	if res.BaseVersion != 0 || res.MainVersion != 0 {
		t.Fatalf("versions = %d / %d", res.BaseVersion, res.MainVersion)
	}

	// A draft that changes nothing has an empty diff; undone changes do not count.
	empty := w.draft(w.author, "empty", updateEl(idAPI, obj{"name": "Other"}))
	if _, err := w.svc.Undo(as(w.author), empty.ID); err != nil {
		t.Fatal(err)
	}
	if res, err := w.svc.Diff(as(w.author), empty.ID); err != nil || !res.Diff.Empty() {
		t.Fatalf("diff = %+v err=%v", res, err)
	}
}

func TestLintOnDraft(t *testing.T) {
	w := newWF(t)
	cs := w.draft(w.author, "t",
		addEl(idNew, obj{"kind": "container", "name": "API", "parent_id": idShop}), // duplicate sibling name
		addEl(idNew2, obj{"kind": "component", "name": "Loose"}),                   // component without parent
	)
	report, err := w.svc.Lint(as(w.viewer), cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]int{}
	for _, f := range report.Findings {
		if f.Severity == lint.SeverityError {
			rules[f.Rule]++
		}
	}
	if rules["duplicate-sibling-name"] != 1 || rules["hierarchy"] != 1 {
		t.Fatalf("errors = %v", rules)
	}

	// Errors block the merge and nothing is applied.
	_, _ = w.svc.Submit(as(w.author), cs.ID)
	_, _ = w.svc.Approve(as(w.m2), cs.ID)
	_, err = w.svc.Merge(as(w.m1), cs.ID)
	wantKind(t, err, apperr.KindConflict)
	if len(w.repo.main.Elements) != 3 || w.repo.version != 0 {
		t.Fatal("main changed despite lint errors")
	}
}

func TestMergeFastForward(t *testing.T) {
	w := newWF(t)
	cs := w.draft(w.author, "Rename API",
		updateEl(idAPI, obj{"name": "Gateway"}),
		addEl(idNew, obj{"kind": "container", "name": "Cache", "parent_id": idShop}),
		deleteEl(idDB),
	)
	res := w.land(cs, w.author)
	if res.Version != 1 || res.Changeset.Status != changeset.StatusMerged {
		t.Fatalf("result = %+v", res)
	}
	if w.mainName(idAPI) != "Gateway" || w.mainName(idNew) != "Cache" || w.mainName(idDB) != "" {
		t.Fatalf("main = %v", names(w.repo.main))
	}
	if len(w.repo.main.Relations) != 0 {
		t.Fatalf("relations of the deleted element must be gone: %+v", w.repo.main.Relations)
	}

	versions, err := w.svc.Versions(as(w.viewer))
	if err != nil || len(versions) != 1 || versions[0].Number != 1 || versions[0].Summary != "Rename API" {
		t.Fatalf("versions = %+v err=%v", versions, err)
	}
	snap, err := w.svc.ModelAt(as(w.viewer), 1)
	if err != nil || names(snap) != "Cache,Gateway,Shop" {
		t.Fatalf("snapshot = %v err=%v", names(snap), err)
	}
	if _, err := w.svc.ModelAt(as(w.viewer), 2); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("unknown version err = %v", err)
	}
	if _, err := w.svc.ModelAt(as(w.viewer), 0); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("version 0 err = %v", err)
	}
	// A merged changeset cannot be merged twice.
	_, err = w.svc.Merge(as(w.m1), cs.ID)
	wantKind(t, err, apperr.KindConflict)
}

func TestMergeRebaseDisjointEdits(t *testing.T) {
	w := newWF(t)
	a := w.draft(w.author, "A", updateEl(idAPI, obj{"name": "Gateway"}))
	b := w.draft(w.other, "B", updateEl(idDB, obj{"name": "Data"}), updateEl(idAPI, obj{"description": "edge"}))

	w.land(a, w.author)
	// B is stale now; its view still shows the world it was based on.
	eff, err := w.svc.EffectiveModel(as(w.other), b.ID)
	if err != nil || names(eff) != "API,Data,Shop" {
		t.Fatalf("stale effective model = %v err=%v", names(eff), err)
	}
	if d, _ := w.svc.Diff(as(w.other), b.ID); d.MainVersion != 1 || d.BaseVersion != 0 {
		t.Fatalf("diff versions = %d/%d", d.BaseVersion, d.MainVersion)
	}
	if c, err := w.svc.Conflicts(as(w.other), b.ID); err != nil || len(c) != 0 {
		t.Fatalf("conflicts = %+v err=%v", c, err)
	}

	res := w.land(b, w.other)
	if res.Version != 2 {
		t.Fatalf("version = %d", res.Version)
	}
	// Both edits are present: A's rename and B's rename + description on the same element.
	var api model.Element
	for _, e := range w.repo.main.Elements {
		if e.ID == idAPI {
			api = e
		}
	}
	if api.Name != "Gateway" || api.Description != "edge" || w.mainName(idDB) != "Data" {
		t.Fatalf("main = %v api=%+v", names(w.repo.main), api)
	}
	if v1, _ := w.svc.ModelAt(as(w.viewer), 1); names(v1) != "DB,Gateway,Shop" {
		t.Fatalf("v1 must be untouched: %v", names(v1))
	}
}

func TestMergeConflictResolution(t *testing.T) {
	setup := func(t *testing.T) (*wf, changeset.Changeset) {
		w := newWF(t)
		a := w.draft(w.author, "A", updateEl(idAPI, obj{"name": "Theirs"}))
		b := w.draft(w.other, "B", updateEl(idAPI, obj{"name": "Mine"}))
		w.land(a, w.author)
		return w, b
	}
	nameKey := changeset.ResolutionKey{TargetType: changeset.TargetElement, TargetID: idAPI, Field: "name"}

	t.Run("reported, nothing applied", func(t *testing.T) {
		w, b := setup(t)
		res := w.tryLand(b, w.other)
		if len(res.Conflicts) != 1 || res.Conflicts[0].Field != "name" || res.Version != 0 {
			t.Fatalf("result = %+v", res)
		}
		if w.mainName(idAPI) != "Theirs" || w.repo.version != 1 {
			t.Fatal("main must be untouched by a conflicted merge")
		}
		if cs := w.repo.changesets[b.ID]; cs.Status != changeset.StatusInReview {
			t.Fatalf("status = %s", cs.Status)
		}
	})

	for _, tc := range []struct {
		choice changeset.Choice
		want   string
	}{{changeset.ChoiceMine, "Mine"}, {changeset.ChoiceTheirs, "Theirs"}} {
		t.Run("resolve "+string(tc.choice), func(t *testing.T) {
			w, b := setup(t)
			w.tryLand(b, w.other)
			remaining, err := w.svc.Resolve(as(w.other), b.ID, []changeset.Resolution{{ResolutionKey: nameKey, Choice: tc.choice}})
			if err != nil || len(remaining) != 0 {
				t.Fatalf("remaining = %+v err=%v", remaining, err)
			}
			res, err := w.svc.Merge(as(w.m1), b.ID)
			if err != nil || res.Version != 2 || len(res.Conflicts) != 0 {
				t.Fatalf("merge = %+v err=%v", res, err)
			}
			if got := w.mainName(idAPI); got != tc.want {
				t.Fatalf("name = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("resolve validation and permissions", func(t *testing.T) {
		w, b := setup(t)
		_, err := w.svc.Resolve(as(w.other), b.ID, nil)
		wantKind(t, err, apperr.KindInvalid)
		_, err = w.svc.Resolve(as(w.other), b.ID, []changeset.Resolution{{ResolutionKey: nameKey, Choice: "ours"}})
		wantKind(t, err, apperr.KindInvalid)
		_, err = w.svc.Resolve(as(w.viewer), b.ID, []changeset.Resolution{{ResolutionKey: nameKey, Choice: changeset.ChoiceMine}})
		wantKind(t, err, apperr.KindForbidden)
		third := actor(authctx.RoleEditor)
		_, err = w.svc.Resolve(as(third), b.ID, []changeset.Resolution{{ResolutionKey: nameKey, Choice: changeset.ChoiceMine}})
		wantKind(t, err, apperr.KindForbidden)
	})
}

func TestMergeDeleteVsUpdateAndDangling(t *testing.T) {
	t.Run("delete vs update", func(t *testing.T) {
		w := newWF(t)
		a := w.draft(w.author, "A", deleteEl(idDB))
		b := w.draft(w.other, "B", updateEl(idDB, obj{"name": "Data"}))
		w.land(a, w.author)
		res := w.tryLand(b, w.other)
		if len(res.Conflicts) != 1 || res.Conflicts[0].Kind != changeset.ConflictDeleteVsUpdate {
			t.Fatalf("conflicts = %+v", res.Conflicts)
		}
		key := changeset.ResolutionKey{TargetType: changeset.TargetElement, TargetID: idDB, Field: changeset.WholeTarget}
		// "theirs" = the deletion stays; main is unchanged apart from the version bump.
		if _, err := w.svc.Resolve(as(w.other), b.ID, []changeset.Resolution{{ResolutionKey: key, Choice: changeset.ChoiceTheirs}}); err != nil {
			t.Fatal(err)
		}
		if res, err := w.svc.Merge(as(w.m1), b.ID); err != nil || res.Version != 2 {
			t.Fatalf("merge = %+v err=%v", res, err)
		}
		if w.mainName(idDB) != "" {
			t.Fatal("DB should stay deleted")
		}
	})

	t.Run("new child under a deleted parent", func(t *testing.T) {
		w := newWF(t)
		a := w.draft(w.author, "A", deleteEl(idDB))
		b := w.draft(w.other, "B", addEl(idNew, obj{"kind": "component", "name": "Pool", "parent_id": idDB}))
		w.land(a, w.author)
		res := w.tryLand(b, w.other)
		if len(res.Conflicts) != 1 || res.Conflicts[0].Kind != changeset.ConflictDanglingReference || res.Conflicts[0].TargetID != idNew {
			t.Fatalf("conflicts = %+v", res.Conflicts)
		}
		key := changeset.ResolutionKey{TargetType: changeset.TargetElement, TargetID: idNew, Field: changeset.WholeTarget}
		// "mine" restores the parent from B's world so the child survives.
		if _, err := w.svc.Resolve(as(w.other), b.ID, []changeset.Resolution{{ResolutionKey: key, Choice: changeset.ChoiceMine}}); err != nil {
			t.Fatal(err)
		}
		if res, err := w.svc.Merge(as(w.m1), b.ID); err != nil || res.Version != 2 {
			t.Fatalf("merge = %+v err=%v", res, err)
		}
		if w.mainName(idDB) != "DB" || w.mainName(idNew) != "Pool" {
			t.Fatalf("main = %v", names(w.repo.main))
		}
	})
}

func TestProtectedElements(t *testing.T) {
	protectedMain := func() *wf {
		w := newWF(t, changeset.WithSelfMerge(true))
		for i := range w.repo.main.Elements {
			if w.repo.main.Elements[i].ID == idAPI {
				w.repo.main.Elements[i].Protected = true
			}
		}
		return w
	}

	t.Run("only maintainers change the protected flag", func(t *testing.T) {
		w := newWF(t)
		_, err := w.svc.AddChanges(as(w.author), w.draft(w.author, "t").ID, []changeset.ChangeInput{updateEl(idAPI, obj{"protected": true})})
		wantKind(t, err, apperr.KindForbidden)
		_, err = w.svc.AddChanges(as(w.author), w.draft(w.author, "t2").ID, []changeset.ChangeInput{
			addEl(idNew, obj{"kind": "system", "name": "S", "protected": true})})
		wantKind(t, err, apperr.KindForbidden)
		// Adding with protected=false is fine, and maintainers may set it.
		if _, err := w.svc.AddChanges(as(w.author), w.draft(w.author, "t3").ID, []changeset.ChangeInput{
			addEl(idNew, obj{"kind": "system", "name": "S", "protected": false})}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.svc.AddChanges(as(w.m1), w.draft(w.m1, "t4").ID, []changeset.ChangeInput{updateEl(idAPI, obj{"protected": true})}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("author cannot merge a protected change even with self-merge on", func(t *testing.T) {
		w := protectedMain()
		cs := w.draft(w.m1, "t", updateEl(idAPI, obj{"name": "Gateway"}))
		_, _ = w.svc.Submit(as(w.m1), cs.ID)
		if _, err := w.svc.Approve(as(w.m1), cs.ID); err != nil { // self-approval allowed by the flag
			t.Fatal(err)
		}
		_, err := w.svc.Merge(as(w.m1), cs.ID)
		wantKind(t, err, apperr.KindForbidden)
		if w.mainName(idAPI) != "API" {
			t.Fatal("main changed")
		}
		// An independent maintainer approving and merging works.
		if _, err := w.svc.Approve(as(w.m2), cs.ID); err != nil {
			t.Fatal(err)
		}
		_, err = w.svc.Merge(as(w.m1), cs.ID)
		wantKind(t, err, apperr.KindForbidden) // merger is still the author
		res, err := w.svc.Merge(as(w.m2), cs.ID)
		if err != nil || res.Version != 1 || w.mainName(idAPI) != "Gateway" {
			t.Fatalf("merge = %+v err=%v", res, err)
		}
	})

	t.Run("editor's protected change merges with two maintainers", func(t *testing.T) {
		w := protectedMain()
		cs := w.draft(w.author, "t", deleteEl(idAPI))
		res := w.land(cs, w.author)
		if res.Version != 1 || w.mainName(idAPI) != "" {
			t.Fatalf("merge = %+v", res)
		}
	})
}
