package changeset_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

var (
	idShop = uuid.MustParse("00000000-0000-7000-8000-000000000001")
	idAPI  = uuid.MustParse("00000000-0000-7000-8000-000000000002")
	idDB   = uuid.MustParse("00000000-0000-7000-8000-000000000003")
	idRel  = uuid.MustParse("00000000-0000-7000-8000-000000000004")
	idNew  = uuid.MustParse("00000000-0000-7000-8000-0000000000aa")
	idNew2 = uuid.MustParse("00000000-0000-7000-8000-0000000000bb")
)

func mainModel() model.Snapshot {
	return model.Snapshot{
		Elements: []model.Element{
			{ID: idShop, Kind: model.KindSystem, Name: "Shop", Tags: []string{}, Version: 1},
			{ID: idAPI, ParentID: &idShop, Kind: model.KindContainer, Name: "API", Technology: "Go", Tags: []string{"backend"}, Version: 1},
			{ID: idDB, ParentID: &idShop, Kind: model.KindContainer, Name: "DB", Tags: []string{}, Version: 1},
		},
		Relations: []model.Relation{
			{ID: idRel, SourceID: idAPI, TargetID: idDB, Kind: model.RelUses, Technology: "SQL", Version: 1},
		},
	}
}

func actor(role authctx.Role) authctx.Actor {
	return authctx.Actor{ID: uuid.New(), Email: string(role) + "@test", Name: string(role), Role: role}
}

func as(a authctx.Actor) context.Context { return authctx.With(context.Background(), a) }

func raw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

type obj = map[string]any

func newDraft(t *testing.T) (*changeset.Service, *memRepo, authctx.Actor, changeset.Changeset) {
	t.Helper()
	repo := newMemRepo(mainModel())
	svc := changeset.NewService(repo)
	editor := actor(authctx.RoleEditor)
	cs, err := svc.Create(as(editor), "my draft", "")
	if err != nil {
		t.Fatal(err)
	}
	return svc, repo, editor, cs
}

func elementNames(s model.Snapshot) []string {
	names := make([]string, len(s.Elements))
	for i, e := range s.Elements {
		names[i] = e.Name
	}
	return names
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		title    string
		wantKind apperr.Kind
	}{
		{"editor ok", as(actor(authctx.RoleEditor)), "t", 0},
		{"maintainer ok", as(actor(authctx.RoleMaintainer)), "t", 0},
		{"viewer forbidden", as(actor(authctx.RoleViewer)), "t", apperr.KindForbidden},
		{"anonymous", context.Background(), "t", apperr.KindUnauthenticated},
		{"blank title", as(actor(authctx.RoleEditor)), "   ", apperr.KindInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := changeset.NewService(newMemRepo(mainModel()))
			cs, err := svc.Create(tc.ctx, tc.title, "")
			if got := apperr.KindOf(err); got != tc.wantKind {
				t.Fatalf("kind = %v, want %v (err=%v)", got, tc.wantKind, err)
			}
			if err == nil && (cs.Status != changeset.StatusDraft || cs.BaseVersion != 0) {
				t.Fatalf("unexpected changeset: %+v", cs)
			}
		})
	}
}

func TestAddChanges(t *testing.T) {
	tests := []struct {
		name      string
		inputs    []changeset.ChangeInput
		wantKind  apperr.Kind
		wantNames []string // element names of the effective model on success
		wantRels  int
	}{
		{
			name: "add element",
			inputs: []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetElement,
				Patch: raw(obj{"kind": "container", "name": "Worker", "parent_id": idShop})}},
			wantNames: []string{"API", "DB", "Shop", "Worker"}, wantRels: 1,
		},
		{
			name: "add child then relation in one batch using supplied ids",
			inputs: []changeset.ChangeInput{
				{Op: changeset.OpAdd, TargetType: changeset.TargetElement, TargetID: idNew,
					Patch: raw(obj{"kind": "container", "name": "Queue", "parent_id": idShop})},
				{Op: changeset.OpAdd, TargetType: changeset.TargetRelation,
					Patch: raw(obj{"source_id": idAPI, "target_id": idNew, "kind": "uses"})},
			},
			wantNames: []string{"API", "DB", "Queue", "Shop"}, wantRels: 2,
		},
		{
			name: "update element",
			inputs: []changeset.ChangeInput{{Op: changeset.OpUpdate, TargetType: changeset.TargetElement,
				TargetID: idAPI, Patch: raw(obj{"name": "Gateway"})}},
			wantNames: []string{"DB", "Gateway", "Shop"}, wantRels: 1,
		},
		{
			name:      "delete leaf element also deletes its relations",
			inputs:    []changeset.ChangeInput{{Op: changeset.OpDelete, TargetType: changeset.TargetElement, TargetID: idDB}},
			wantNames: []string{"API", "Shop"}, wantRels: 0,
		},
		{
			name:     "delete parent without cascade",
			inputs:   []changeset.ChangeInput{{Op: changeset.OpDelete, TargetType: changeset.TargetElement, TargetID: idShop}},
			wantKind: apperr.KindInvalid,
		},
		{
			name:      "delete parent with cascade",
			inputs:    []changeset.ChangeInput{{Op: changeset.OpDelete, TargetType: changeset.TargetElement, TargetID: idShop, Cascade: true}},
			wantNames: []string{}, wantRels: 0,
		},
		{
			name:      "delete relation",
			inputs:    []changeset.ChangeInput{{Op: changeset.OpDelete, TargetType: changeset.TargetRelation, TargetID: idRel}},
			wantNames: []string{"API", "DB", "Shop"}, wantRels: 0,
		},
		{
			name: "update relation",
			inputs: []changeset.ChangeInput{{Op: changeset.OpUpdate, TargetType: changeset.TargetRelation,
				TargetID: idRel, Patch: raw(obj{"technology": "gRPC"})}},
			wantNames: []string{"API", "DB", "Shop"}, wantRels: 1,
		},
		{
			name: "invalid kind",
			inputs: []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetElement,
				Patch: raw(obj{"kind": "banana", "name": "x"})}},
			wantKind: apperr.KindInvalid,
		},
		{
			name: "blank name",
			inputs: []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetElement,
				Patch: raw(obj{"kind": "system", "name": " "})}},
			wantKind: apperr.KindInvalid,
		},
		{
			name: "unknown parent",
			inputs: []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetElement,
				Patch: raw(obj{"kind": "system", "name": "x", "parent_id": idNew})}},
			wantKind: apperr.KindInvalid,
		},
		{
			name: "server-managed field in patch",
			inputs: []changeset.ChangeInput{{Op: changeset.OpUpdate, TargetType: changeset.TargetElement,
				TargetID: idAPI, Patch: raw(obj{"version": 9})}},
			wantKind: apperr.KindInvalid,
		},
		{
			name: "update unknown element",
			inputs: []changeset.ChangeInput{{Op: changeset.OpUpdate, TargetType: changeset.TargetElement,
				TargetID: idNew, Patch: raw(obj{"name": "x"})}},
			wantKind: apperr.KindNotFound,
		},
		{
			name: "move under own descendant",
			inputs: []changeset.ChangeInput{{Op: changeset.OpUpdate, TargetType: changeset.TargetElement,
				TargetID: idShop, Patch: raw(obj{"parent_id": idAPI})}},
			wantKind: apperr.KindInvalid,
		},
		{
			name: "add with existing id",
			inputs: []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetElement, TargetID: idAPI,
				Patch: raw(obj{"kind": "system", "name": "dup"})}},
			wantKind: apperr.KindConflict,
		},
		{
			name: "relation to missing target",
			inputs: []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetRelation,
				Patch: raw(obj{"source_id": idAPI, "target_id": idNew, "kind": "uses"})}},
			wantKind: apperr.KindInvalid,
		},
		{
			name: "self relation",
			inputs: []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetRelation,
				Patch: raw(obj{"source_id": idAPI, "target_id": idAPI, "kind": "uses"})}},
			wantKind: apperr.KindInvalid,
		},
		{
			name: "failure in second change stores nothing",
			inputs: []changeset.ChangeInput{
				{Op: changeset.OpAdd, TargetType: changeset.TargetElement, Patch: raw(obj{"kind": "system", "name": "ok"})},
				{Op: changeset.OpDelete, TargetType: changeset.TargetElement, TargetID: idNew},
			},
			wantKind: apperr.KindNotFound,
		},
		{name: "empty batch", inputs: nil, wantKind: apperr.KindInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, editor, cs := newDraft(t)
			ctx := as(editor)

			_, err := svc.AddChanges(ctx, cs.ID, tc.inputs)
			if got := apperr.KindOf(err); got != tc.wantKind {
				t.Fatalf("kind = %v, want %v (err=%v)", got, tc.wantKind, err)
			}
			eff, effErr := svc.EffectiveModel(ctx, cs.ID)
			if effErr != nil {
				t.Fatal(effErr)
			}
			if tc.wantKind != 0 { // failed: nothing stored, effective == main
				if n := len(repo.changes); n != 0 {
					t.Fatalf("%d changes stored after failure", n)
				}
				return
			}
			if got := elementNames(eff); !reflect.DeepEqual(got, tc.wantNames) {
				t.Fatalf("effective elements = %v, want %v", got, tc.wantNames)
			}
			if len(eff.Relations) != tc.wantRels {
				t.Fatalf("effective relations = %d, want %d", len(eff.Relations), tc.wantRels)
			}
			if !reflect.DeepEqual(repo.main, mainModel()) {
				t.Fatal("main was modified by a draft change")
			}
		})
	}
}

func TestAddChangesRecordsBeforeAndExplicitCascade(t *testing.T) {
	svc, _, editor, cs := newDraft(t)
	ctx := as(editor)

	upd, err := svc.AddChanges(ctx, cs.ID, []changeset.ChangeInput{{
		Op: changeset.OpUpdate, TargetType: changeset.TargetElement, TargetID: idAPI,
		Patch: raw(obj{"name": "Gateway", "technology": "Rust"}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(upd) != 1 || string(upd[0].Before) != `{"name":"API","technology":"Go"}` {
		t.Fatalf("before = %s", upd[0].Before)
	}

	del, err := svc.AddChanges(ctx, cs.ID, []changeset.ChangeInput{{
		Op: changeset.OpDelete, TargetType: changeset.TargetElement, TargetID: idShop, Cascade: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	// 1 relation + API + DB + Shop, children before parent, all explicit.
	if len(del) != 4 {
		t.Fatalf("cascade produced %d changes, want 4", len(del))
	}
	if del[0].TargetType != changeset.TargetRelation || del[len(del)-1].TargetID != idShop {
		t.Fatalf("unexpected order: first=%v last=%v", del[0].TargetType, del[len(del)-1].TargetID)
	}
	for i, c := range del {
		if c.Seq != 2+i || c.BatchID != del[0].BatchID || len(c.Before) == 0 {
			t.Fatalf("change %d malformed: %+v", i, c)
		}
	}
}

func TestEditPermissions(t *testing.T) {
	add := []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetElement,
		Patch: raw(obj{"kind": "system", "name": "x"})}}

	tests := []struct {
		name     string
		who      func(author authctx.Actor) authctx.Actor
		status   changeset.Status
		wantKind apperr.Kind
	}{
		{"author", func(a authctx.Actor) authctx.Actor { return a }, changeset.StatusDraft, 0},
		{"maintainer other", func(authctx.Actor) authctx.Actor { return actor(authctx.RoleMaintainer) }, changeset.StatusDraft, 0},
		{"other editor", func(authctx.Actor) authctx.Actor { return actor(authctx.RoleEditor) }, changeset.StatusDraft, apperr.KindForbidden},
		{"viewer", func(authctx.Actor) authctx.Actor { return actor(authctx.RoleViewer) }, changeset.StatusDraft, apperr.KindForbidden},
		{"in review is frozen", func(a authctx.Actor) authctx.Actor { return a }, changeset.StatusInReview, apperr.KindConflict},
		{"merged is frozen", func(a authctx.Actor) authctx.Actor { return a }, changeset.StatusMerged, apperr.KindConflict},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, author, cs := newDraft(t)
			c := repo.changesets[cs.ID]
			c.Status = tc.status
			repo.changesets[cs.ID] = c

			_, err := svc.AddChanges(as(tc.who(author)), cs.ID, add)
			if got := apperr.KindOf(err); got != tc.wantKind {
				t.Fatalf("add kind = %v, want %v (err=%v)", got, tc.wantKind, err)
			}
			if tc.wantKind != 0 {
				if _, err := svc.Undo(as(tc.who(author)), cs.ID); apperr.KindOf(err) == 0 {
					t.Fatalf("undo should be rejected too, got %v", err)
				}
			}
		})
	}

	t.Run("unknown changeset", func(t *testing.T) {
		svc, _, editor, _ := newDraft(t)
		_, err := svc.AddChanges(as(editor), idNew, add)
		if apperr.KindOf(err) != apperr.KindNotFound {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestUndoRedo(t *testing.T) {
	svc, repo, editor, cs := newDraft(t)
	ctx := as(editor)
	addNamed := func(id uuid.UUID, name string) {
		t.Helper()
		_, err := svc.AddChanges(ctx, cs.ID, []changeset.ChangeInput{{
			Op: changeset.OpAdd, TargetType: changeset.TargetElement, TargetID: id,
			Patch: raw(obj{"kind": "system", "name": name}),
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	names := func() []string {
		t.Helper()
		eff, err := svc.EffectiveModel(ctx, cs.ID)
		if err != nil {
			t.Fatal(err)
		}
		return elementNames(eff)
	}
	expect := func(step string, want ...string) {
		t.Helper()
		if got := names(); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: effective = %v, want %v", step, got, want)
		}
	}
	expectKind := func(step string, err error, want apperr.Kind) {
		t.Helper()
		if got := apperr.KindOf(err); got != want {
			t.Fatalf("%s: kind = %v, want %v (err=%v)", step, got, want, err)
		}
	}

	_, err := svc.Undo(ctx, cs.ID)
	expectKind("undo on empty draft", err, apperr.KindConflict)

	addNamed(idNew, "A")
	addNamed(idNew2, "B")
	expect("after A,B", "A", "API", "B", "DB", "Shop")

	if _, err := svc.Undo(ctx, cs.ID); err != nil {
		t.Fatal(err)
	}
	expect("undo B", "A", "API", "DB", "Shop")
	if _, err := svc.Undo(ctx, cs.ID); err != nil {
		t.Fatal(err)
	}
	expect("undo A", "API", "DB", "Shop")
	_, err = svc.Undo(ctx, cs.ID)
	expectKind("undo past start", err, apperr.KindConflict)

	if _, err := svc.Redo(ctx, cs.ID); err != nil {
		t.Fatal(err)
	}
	expect("redo A", "A", "API", "DB", "Shop")

	// A new batch discards the redo stack (B).
	addNamed(uuid.MustParse("00000000-0000-7000-8000-0000000000cc"), "C")
	expect("add C", "A", "API", "C", "DB", "Shop")
	_, err = svc.Redo(ctx, cs.ID)
	expectKind("redo after new batch", err, apperr.KindConflict)
	if n := len(repo.changes); n != 2 { // A and C; undone B was discarded
		t.Fatalf("%d changes stored, want 2", n)
	}

	// The audit trail keeps undone changes until the next batch replaces them.
	if _, err := svc.Undo(ctx, cs.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := svc.Get(ctx, cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Changes) != 2 || detail.Changes[1].UndoneAt == nil {
		t.Fatalf("detail changes = %+v", detail.Changes)
	}
	if !reflect.DeepEqual(repo.main, mainModel()) {
		t.Fatal("main was modified")
	}
}

func TestBootstrap(t *testing.T) {
	inputs := []changeset.ChangeInput{{Op: changeset.OpAdd, TargetType: changeset.TargetElement,
		Patch: raw(obj{"kind": "system", "name": "Seeded"})}}

	t.Run("only maintainer", func(t *testing.T) {
		svc := changeset.NewService(newMemRepo(model.Snapshot{}))
		_, err := svc.Bootstrap(as(actor(authctx.RoleEditor)), "init", inputs)
		if apperr.KindOf(err) != apperr.KindForbidden {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("creates version 1 once", func(t *testing.T) {
		repo := newMemRepo(model.Snapshot{})
		svc := changeset.NewService(repo)
		ctx := as(actor(authctx.RoleMaintainer))
		cs, err := svc.Bootstrap(ctx, "init", inputs)
		if err != nil {
			t.Fatal(err)
		}
		if cs.Status != changeset.StatusMerged || repo.version != 1 {
			t.Fatalf("status=%v version=%d", cs.Status, repo.version)
		}
		if _, err := svc.Bootstrap(ctx, "again", inputs); apperr.KindOf(err) != apperr.KindConflict {
			t.Fatalf("second bootstrap err = %v", err)
		}
	})
}
