package changeset_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

var (
	idAtlas    = uuid.MustParse("00000000-0000-7000-8000-0000000000c1")
	idBorealis = uuid.MustParse("00000000-0000-7000-8000-0000000000c2")
)

func addOwner(id uuid.UUID, name string) changeset.ChangeInput {
	return changeset.ChangeInput{Op: changeset.OpAdd, TargetType: changeset.TargetOwner, TargetID: id,
		Patch: raw(obj{"kind": "team", "name": name, "contact": name + "@team.example", "oncall_url": ""})}
}

func assign(element, owner uuid.UUID, role model.OwnerRole) changeset.ChangeInput {
	return changeset.ChangeInput{Op: changeset.OpAdd, TargetType: changeset.TargetOwnership,
		Patch: raw(obj{"element_id": element, "owner_id": owner, "role": role})}
}

func TestOwnershipChanges(t *testing.T) {
	svc, _, editor, cs := newDraft(t)
	ctx := as(editor)
	add := func(in ...changeset.ChangeInput) error {
		_, err := svc.AddChanges(ctx, cs.ID, in)
		return err
	}
	if err := add(addOwner(idAtlas, "Team Atlas"), addOwner(idBorealis, "Team Borealis"),
		assign(idShop, idAtlas, model.RoleOwner), assign(idDB, idBorealis, model.RoleOwner)); err != nil {
		t.Fatal(err)
	}
	dbOwnership := model.OwnershipID(idDB, model.RoleOwner)

	tests := []struct {
		name string
		in   changeset.ChangeInput
		want apperr.Kind
	}{
		{"second owner in the same role", assign(idShop, idBorealis, model.RoleOwner), apperr.KindConflict},
		{"unknown owner", assign(idAPI, uuid.New(), model.RoleOwner), apperr.KindInvalid},
		{"unknown element", assign(uuid.New(), idAtlas, model.RoleOwner), apperr.KindInvalid},
		{"bad role", changeset.ChangeInput{Op: changeset.OpAdd, TargetType: changeset.TargetOwnership,
			Patch: raw(obj{"element_id": idAPI, "owner_id": idAtlas, "role": "boss"})}, apperr.KindInvalid},
		{"foreign id", changeset.ChangeInput{Op: changeset.OpAdd, TargetType: changeset.TargetOwnership, TargetID: uuid.New(),
			Patch: raw(obj{"element_id": idAPI, "owner_id": idAtlas, "role": "oncall"})}, apperr.KindInvalid},
		{"moving an assignment", changeset.ChangeInput{Op: changeset.OpUpdate, TargetType: changeset.TargetOwnership,
			TargetID: dbOwnership, Patch: raw(obj{"element_id": idAPI})}, apperr.KindInvalid},
		{"duplicate owner name", addOwner(uuid.New(), "team atlas"), apperr.KindInvalid},
		{"delete an assigned owner without cascade", changeset.ChangeInput{Op: changeset.OpDelete,
			TargetType: changeset.TargetOwner, TargetID: idBorealis}, apperr.KindInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := add(tc.in); apperr.KindOf(err) != tc.want {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}

	// Replacing the owner of a role is an update of owner_id.
	if err := add(changeset.ChangeInput{Op: changeset.OpUpdate, TargetType: changeset.TargetOwnership,
		TargetID: dbOwnership, Patch: raw(obj{"owner_id": idAtlas})}); err != nil {
		t.Fatal(err)
	}
	// Deleting an element takes its ownerships along; deleting an owner with cascade too.
	if err := add(changeset.ChangeInput{Op: changeset.OpDelete, TargetType: changeset.TargetElement, TargetID: idDB}); err != nil {
		t.Fatal(err)
	}
	if err := add(changeset.ChangeInput{Op: changeset.OpDelete, TargetType: changeset.TargetOwner, TargetID: idAtlas, Cascade: true}); err != nil {
		t.Fatal(err)
	}

	d, err := svc.Diff(ctx, cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Diff.AddedOwners) != 1 || d.Diff.AddedOwners[0].Name != "Team Borealis" || len(d.Diff.AddedOwnerships) != 0 {
		t.Fatalf("diff owners = %+v, ownerships = %+v", d.Diff.AddedOwners, d.Diff.AddedOwnerships)
	}
}

func TestOwnershipDiffAndMerge(t *testing.T) {
	repo := newMemRepo(mainModel())
	svc := changeset.NewService(repo, changeset.WithSelfMerge(true))
	m := actor(authctx.RoleMaintainer)
	ctx := as(m)
	cs, err := svc.Create(ctx, "owners", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddChanges(ctx, cs.ID, []changeset.ChangeInput{
		addOwner(idAtlas, "Team Atlas"), assign(idShop, idAtlas, model.RoleOwner),
	}); err != nil {
		t.Fatal(err)
	}
	d, err := svc.Diff(ctx, cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Diff.AddedOwners) != 1 || len(d.Diff.AddedOwnerships) != 1 || d.Diff.AddedOwnerships[0].ElementID != idShop {
		t.Fatalf("diff = %+v", d.Diff)
	}
	for _, step := range []func() error{
		func() error { _, err := svc.Submit(ctx, cs.ID); return err },
		func() error { _, err := svc.Approve(ctx, cs.ID); return err },
		func() error { _, err := svc.Merge(ctx, cs.ID); return err },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.main.Owners) != 1 || len(repo.main.Ownerships) != 1 {
		t.Fatalf("main after merge: %d owners, %d ownerships", len(repo.main.Owners), len(repo.main.Ownerships))
	}
	eff := model.ComputeEffective(repo.main)
	if o, ok := eff[idAPI].Owner(model.RoleOwner); !ok || o.OwnerID != idAtlas || o.Source != idShop {
		t.Fatalf("API's effective owner = %+v, %v", o, ok)
	}
}

func TestProtectedElementOwnershipNeedsSecondMaintainer(t *testing.T) {
	base := mainModel()
	base.Elements[0].Protected = true // Shop
	repo := newMemRepo(base)
	svc := changeset.NewService(repo, changeset.WithSelfMerge(true))
	ctx := as(actor(authctx.RoleMaintainer))
	cs, err := svc.Create(ctx, "own the shop", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddChanges(ctx, cs.ID, []changeset.ChangeInput{addOwner(idAtlas, "Team Atlas"), assign(idShop, idAtlas, model.RoleOwner)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, cs.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Approve(ctx, cs.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Merge(ctx, cs.ID); apperr.KindOf(err) != apperr.KindForbidden {
		t.Fatalf("self-merged ownership change on a protected element: err = %v", err)
	}
}

// mergeFixture: element Shop (1) and owners Atlas (100), Borealis (101) in base.
func mergeFixture() model.Snapshot {
	return model.Snapshot{
		Elements: []model.Element{tel(1, "Shop", 0)},
		Owners: []model.Owner{
			{ID: tid(100), Kind: model.OwnerTeam, Name: "Atlas", Version: 1},
			{ID: tid(101), Kind: model.OwnerTeam, Name: "Borealis", Version: 1},
		},
	}
}

func ownership(element, owner byte, role model.OwnerRole) model.Ownership {
	return model.Ownership{ID: model.OwnershipID(tid(element), role), ElementID: tid(element), OwnerID: tid(owner), Role: role, Version: 1}
}

func withOwnerships(s model.Snapshot, os ...model.Ownership) model.Snapshot {
	s.Ownerships = append(slices.Clone(s.Ownerships), os...)
	return s
}

func TestMerge3Ownership(t *testing.T) {
	base := mergeFixture()
	id := model.OwnershipID(tid(1), model.RoleOwner)
	noOwner := func(s model.Snapshot) model.Snapshot {
		s.Owners = slices.DeleteFunc(slices.Clone(s.Owners), func(o model.Owner) bool { return o.ID == tid(100) })
		return s
	}
	noElement := func(s model.Snapshot) model.Snapshot { s.Elements = nil; return s }

	tests := []struct {
		name         string
		mine, theirs model.Snapshot
		res          map[changeset.ResolutionKey]changeset.Choice
		conflicts    []string // "field" or "*" per conflict on the ownership
		wantOwner    byte     // 0 = no ownership in the result
	}{
		{"added on one side", withOwnerships(base, ownership(1, 100, model.RoleOwner)), base, nil, nil, 100},
		{"same owner on both sides", withOwnerships(base, ownership(1, 100, model.RoleOwner)), withOwnerships(base, ownership(1, 100, model.RoleOwner)), nil, nil, 100},
		{"same role, different owners", withOwnerships(base, ownership(1, 100, model.RoleOwner)), withOwnerships(base, ownership(1, 101, model.RoleOwner)), nil, []string{"owner_id"}, 101},
		{"same role, different owners, mine chosen", withOwnerships(base, ownership(1, 100, model.RoleOwner)), withOwnerships(base, ownership(1, 101, model.RoleOwner)),
			map[changeset.ResolutionKey]changeset.Choice{{TargetType: changeset.TargetOwnership, TargetID: id, Field: "owner_id"}: changeset.ChoiceMine}, nil, 100},
		{"different roles do not conflict", withOwnerships(base, ownership(1, 100, model.RoleOwner)), withOwnerships(base, ownership(1, 101, model.RoleOncall)), nil, nil, 100},
		{"owner deleted on the other side", withOwnerships(base, ownership(1, 100, model.RoleOwner)), noOwner(base), nil, []string{"*"}, 100},
		{"owner deleted; theirs wins drops the assignment", withOwnerships(base, ownership(1, 100, model.RoleOwner)), noOwner(base),
			map[changeset.ResolutionKey]changeset.Choice{{TargetType: changeset.TargetOwnership, TargetID: id, Field: changeset.WholeTarget}: changeset.ChoiceTheirs}, nil, 0},
		{"owner deleted; mine wins restores the owner", withOwnerships(base, ownership(1, 100, model.RoleOwner)), noOwner(base),
			map[changeset.ResolutionKey]changeset.Choice{{TargetType: changeset.TargetOwnership, TargetID: id, Field: changeset.WholeTarget}: changeset.ChoiceMine}, nil, 100},
		{"element deleted on the other side", withOwnerships(base, ownership(1, 100, model.RoleOwner)), noElement(base), nil, []string{"*"}, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := changeset.Merge3(base, tc.mine, tc.theirs, tc.res)
			var got []string
			for _, c := range out.Conflicts {
				if c.TargetType == changeset.TargetOwnership {
					got = append(got, c.Field)
				}
			}
			if !slices.Equal(got, tc.conflicts) {
				t.Fatalf("ownership conflicts = %v, want %v (all: %+v)", got, tc.conflicts, out.Conflicts)
			}
			if len(tc.conflicts) > 0 {
				return // the result is provisional
			}
			var owner byte
			for _, o := range out.Result.Ownerships {
				if o.ID == id {
					owner = o.OwnerID[15]
				}
			}
			if owner != tc.wantOwner {
				t.Fatalf("owner = %d, want %d (result %+v)", owner, tc.wantOwner, out.Result.Ownerships)
			}
			if tc.wantOwner != 0 && !slices.ContainsFunc(out.Result.Owners, func(o model.Owner) bool { return o.ID == tid(tc.wantOwner) }) {
				t.Fatalf("owner %d missing from the result", tc.wantOwner)
			}
		})
	}
}

type fakePrincipals model.Principals

func (f fakePrincipals) Principals(context.Context) (model.Principals, error) {
	return model.Principals(f), nil
}

var (
	idUserAda      = uuid.MustParse("00000000-0000-7000-8000-0000000000d1")
	idUserOff      = uuid.MustParse("00000000-0000-7000-8000-0000000000d2")
	idGroupPayment = uuid.MustParse("00000000-0000-7000-8000-0000000000d3")
)

func directory() fakePrincipals {
	return fakePrincipals{
		idUserAda:      {ID: idUserAda, Kind: model.OwnerUser, Name: "Ada"},
		idUserOff:      {ID: idUserOff, Kind: model.OwnerUser, Name: "Old Account", Inactive: true},
		idGroupPayment: {ID: idGroupPayment, Kind: model.OwnerGroup, Name: "Payments"},
	}
}

func principalOwner(kind model.OwnerKind, principal uuid.UUID) changeset.ChangeInput {
	return changeset.ChangeInput{Op: changeset.OpAdd, TargetType: changeset.TargetOwner,
		Patch: raw(obj{"kind": kind, "name": "", "contact": "", "oncall_url": "", "principal_id": principal})}
}

func TestPrincipalOwnerChanges(t *testing.T) {
	repo := newMemRepo(mainModel())
	svc := changeset.NewService(repo, changeset.WithPrincipals(directory()))
	ctx := as(actor(authctx.RoleEditor))
	cs, err := svc.Create(ctx, "principals", "")
	if err != nil {
		t.Fatal(err)
	}
	add := func(in ...changeset.ChangeInput) error { _, err := svc.AddChanges(ctx, cs.ID, in); return err }

	// A user owner gets the derived id and the user's name, and can be assigned in the same batch.
	adaOwner := model.PrincipalOwnerID(idUserAda)
	if err := add(principalOwner(model.OwnerUser, idUserAda), assign(idShop, adaOwner, model.RoleOwner)); err != nil {
		t.Fatal(err)
	}
	m, err := svc.EffectiveModel(ctx, cs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Owners) != 1 || m.Owners[0].ID != adaOwner || m.Owners[0].Name != "Ada" {
		t.Fatalf("owners = %+v", m.Owners)
	}

	tests := []struct {
		name string
		in   changeset.ChangeInput
		want apperr.Kind
	}{
		{"same user again", principalOwner(model.OwnerUser, idUserAda), apperr.KindConflict},
		{"disabled user", principalOwner(model.OwnerUser, idUserOff), apperr.KindInvalid},
		{"unknown principal", principalOwner(model.OwnerUser, uuid.New()), apperr.KindInvalid},
		{"group id as a user", principalOwner(model.OwnerUser, idGroupPayment), apperr.KindInvalid},
		{"foreign id for a user owner", changeset.ChangeInput{Op: changeset.OpAdd, TargetType: changeset.TargetOwner, TargetID: uuid.New(),
			Patch: raw(obj{"kind": "user", "name": "", "contact": "", "oncall_url": "", "principal_id": idUserAda})}, apperr.KindInvalid},
		{"group owner", principalOwner(model.OwnerGroup, idGroupPayment), 0},
		{"a named team may share a user's name", addOwner(uuid.New(), "Ada"), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := add(tc.in); apperr.KindOf(err) != tc.want {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMerge3PrincipalOwners(t *testing.T) {
	user := tid(42)
	base := model.Snapshot{Elements: []model.Element{tel(1, "Shop", 0)}}
	owner := func(contact string) model.Owner {
		return model.Owner{ID: model.PrincipalOwnerID(user), Kind: model.OwnerUser, Name: "Ada", Contact: contact, PrincipalID: &user, Version: 1}
	}
	with := func(o model.Owner) model.Snapshot { s := base; s.Owners = []model.Owner{o}; return s }

	// Both drafts introduce the owner for the same user: identical, no conflict.
	out := changeset.Merge3(base, with(owner("")), with(owner("")), nil)
	if len(out.Conflicts) != 0 || len(out.Result.Owners) != 1 {
		t.Fatalf("identical add/add: conflicts %+v, owners %+v", out.Conflicts, out.Result.Owners)
	}
	// Different contact on the same user owner: a field conflict, not a silent pick.
	out = changeset.Merge3(base, with(owner("a@x.example")), with(owner("b@x.example")), nil)
	if len(out.Conflicts) != 1 || out.Conflicts[0].TargetType != changeset.TargetOwner || out.Conflicts[0].Field != "contact" {
		t.Fatalf("conflicts = %+v", out.Conflicts)
	}
}
