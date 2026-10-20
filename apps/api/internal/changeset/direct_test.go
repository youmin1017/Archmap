package changeset_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

type fixedPolicy bool

func (p fixedPolicy) DirectEditsAllowed(context.Context) (bool, error) { return bool(p), nil }

func directWF(t *testing.T) *wf {
	return newWF(t, changeset.WithPolicy(fixedPolicy(true)))
}

func (w *wf) direct(by authctx.Actor, summary string, inputs ...changeset.ChangeInput) changeset.DirectResult {
	w.t.Helper()
	res, err := w.svc.ApplyDirect(as(by), summary, inputs)
	if err != nil {
		w.t.Fatal(err)
	}
	return res
}

func TestApplyDirectPolicyAndRoles(t *testing.T) {
	rename := updateEl(idAPI, obj{"name": "API v2"})
	cases := []struct {
		name   string
		opts   []changeset.Option
		role   authctx.Role
		inputs []changeset.ChangeInput
		want   apperr.Kind
	}{
		{"no policy source", nil, authctx.RoleEditor, []changeset.ChangeInput{rename}, apperr.KindForbidden},
		{"policy required", []changeset.Option{changeset.WithPolicy(fixedPolicy(false))}, authctx.RoleEditor, []changeset.ChangeInput{rename}, apperr.KindForbidden},
		{"viewer", []changeset.Option{changeset.WithPolicy(fixedPolicy(true))}, authctx.RoleViewer, []changeset.ChangeInput{rename}, apperr.KindForbidden},
		{"empty batch", []changeset.Option{changeset.WithPolicy(fixedPolicy(true))}, authctx.RoleEditor, nil, apperr.KindInvalid},
		{"no-op", []changeset.Option{changeset.WithPolicy(fixedPolicy(true))}, authctx.RoleEditor, []changeset.ChangeInput{updateEl(idAPI, obj{"name": "API"})}, apperr.KindInvalid},
		{"editor sets protected", []changeset.Option{changeset.WithPolicy(fixedPolicy(true))}, authctx.RoleEditor, []changeset.ChangeInput{updateEl(idAPI, obj{"protected": true})}, apperr.KindForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newWF(t, tc.opts...)
			_, err := w.svc.ApplyDirect(as(actor(tc.role)), "", tc.inputs)
			wantKind(t, err, tc.want)
			if w.repo.version != 0 {
				t.Fatalf("main changed: version %d", w.repo.version)
			}
		})
	}
}

func TestApplyDirectMerges(t *testing.T) {
	w := directWF(t)
	res := w.direct(w.author, "", updateEl(idAPI, obj{"name": "Orders API"}))
	if res.Outcome != changeset.DirectMerged || res.Version != 1 || res.Reason != "" {
		t.Fatalf("result = %+v", res)
	}
	if res.Changeset.Kind != changeset.KindDirect || res.Changeset.Status != changeset.StatusMerged || res.Changeset.Title != "Direct edit" {
		t.Fatalf("changeset = %+v", res.Changeset)
	}
	if w.mainName(idAPI) != "Orders API" {
		t.Fatalf("main not updated: %q", w.mainName(idAPI))
	}
	// Each save is a version, recorded with its kind; direct edits are not listed with the drafts.
	w.direct(w.other, "Rename DB", updateEl(idDB, obj{"name": "Orders DB"}))
	vs, _ := w.svc.Versions(as(w.viewer))
	if len(vs) != 2 || vs[0].Summary != "Rename DB" || vs[0].Kind != changeset.KindDirect || vs[0].MergedBy != w.other.ID {
		t.Fatalf("versions = %+v", vs)
	}
	list, _ := w.svc.List(as(w.viewer), changeset.Filter{})
	if len(list) != 0 {
		t.Fatalf("direct edits listed as drafts: %+v", list)
	}
}

func TestApplyDirectKeepsGovernance(t *testing.T) {
	t.Run("new lint errors keep the edit as a draft", func(t *testing.T) {
		w := directWF(t)
		// A second "API" under Shop is a duplicate sibling name (lint error).
		res := w.direct(w.author, "", addEl(newID(), obj{"kind": "container", "name": "api", "parent_id": idShop}))
		if res.Outcome != changeset.DirectDraft || res.Changeset.Status != changeset.StatusDraft || res.Reason == "" {
			t.Fatalf("result = %+v", res)
		}
		if w.repo.version != 0 {
			t.Fatal("main changed")
		}
		// The draft can be fixed and merged the usual way.
		if _, err := w.svc.AddChanges(as(w.author), res.Changeset.ID, []changeset.ChangeInput{
			updateEl(idAPI, obj{"name": "Public API"}),
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("protected elements go to review", func(t *testing.T) {
		w := directWF(t)
		w.direct(w.m1, "Protect API", updateEl(idAPI, obj{"protected": true}))
		res := w.direct(w.author, "", updateEl(idAPI, obj{"technology": "Rust"}))
		if res.Outcome != changeset.DirectInReview || res.Changeset.Status != changeset.StatusInReview {
			t.Fatalf("result = %+v", res)
		}
		if w.repo.version != 1 {
			t.Fatalf("version = %d, want 1 (only the protect edit)", w.repo.version)
		}
	})
}

func TestRevertDirect(t *testing.T) {
	w := directWF(t)
	edit := w.direct(w.author, "", updateEl(idAPI, obj{"name": "Orders API"}))

	// Only the author or a maintainer may undo it.
	_, err := w.svc.RevertDirect(as(w.other), edit.Changeset.ID)
	wantKind(t, err, apperr.KindForbidden)

	res, err := w.svc.RevertDirect(as(w.author), edit.Changeset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != changeset.DirectMerged || res.Version != 2 || res.Changeset.Reverts == nil || *res.Changeset.Reverts != edit.Changeset.ID {
		t.Fatalf("revert = %+v", res)
	}
	if w.mainName(idAPI) != "API" || res.Changeset.Title != "Undo: Direct edit" {
		t.Fatalf("main name %q, title %q", w.mainName(idAPI), res.Changeset.Title)
	}
	_, err = w.svc.RevertDirect(as(w.author), edit.Changeset.ID)
	wantKind(t, err, apperr.KindConflict)

	// A draft that was merged through review cannot be undone this way.
	cs := w.draft(w.author, "reviewed", updateEl(idDB, obj{"technology": "PostgreSQL"}))
	w.land(cs, w.author)
	_, err = w.svc.RevertDirect(as(w.m1), cs.ID)
	wantKind(t, err, apperr.KindConflict)
}

func TestRevertDirectConflictsWithLaterEdits(t *testing.T) {
	w := directWF(t)
	first := w.direct(w.author, "", updateEl(idAPI, obj{"name": "Orders API"}))
	w.direct(w.other, "", updateEl(idAPI, obj{"name": "Billing API"}))
	_, err := w.svc.RevertDirect(as(w.author), first.Changeset.ID)
	wantKind(t, err, apperr.KindConflict)

	// An unrelated later edit does not block the undo.
	w2 := directWF(t)
	first = w2.direct(w2.author, "", updateEl(idAPI, obj{"name": "Orders API"}))
	w2.direct(w2.other, "", updateEl(idDB, obj{"name": "Orders DB"}))
	if _, err := w2.svc.RevertDirect(as(w2.author), first.Changeset.ID); err != nil {
		t.Fatal(err)
	}
	if w2.mainName(idAPI) != "API" || w2.mainName(idDB) != "Orders DB" {
		t.Fatalf("API %q DB %q", w2.mainName(idAPI), w2.mainName(idDB))
	}
}

func TestRevertDirectRestoresDeletes(t *testing.T) {
	w := directWF(t)
	cache, rel := newID(), newID()
	w.direct(w.author, "", addEl(cache, obj{"kind": "container", "name": "Cache", "parent_id": idShop}),
		changeset.ChangeInput{Op: changeset.OpAdd, TargetType: changeset.TargetRelation, TargetID: rel,
			Patch: raw(obj{"source_id": idAPI, "target_id": cache, "kind": "uses"})})
	del := w.direct(w.m1, "", deleteEl(cache)) // also removes the API -> Cache relation
	if len(w.repo.main.Relations) != 1 {
		t.Fatalf("relation not removed: %+v", w.repo.main.Relations)
	}
	if _, err := w.svc.RevertDirect(as(w.m1), del.Changeset.ID); err != nil {
		t.Fatal(err)
	}
	if w.mainName(cache) != "Cache" || len(w.repo.main.Relations) != 2 {
		t.Fatalf("not restored: Cache %q, relations %+v", w.mainName(cache), w.repo.main.Relations)
	}

	// Undoing an add removes the element again (with the children added with it).
	child := newID()
	parent := newID()
	add := w.direct(w.author, "", addEl(parent, obj{"kind": "system", "name": "Billing"}),
		addEl(child, obj{"kind": "container", "name": "Invoices", "parent_id": parent}))
	if _, err := w.svc.RevertDirect(as(w.author), add.Changeset.ID); err != nil {
		t.Fatal(err)
	}
	if w.mainName(parent) != "" || w.mainName(child) != "" {
		t.Fatal("added elements still in main")
	}
}

func newID() uuid.UUID { return uuid.New() }
