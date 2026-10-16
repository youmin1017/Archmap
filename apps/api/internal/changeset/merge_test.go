package changeset_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/model"
)

var timeZero = time.Time{}

func tid(n byte) uuid.UUID { return uuid.UUID{15: n} }

func tel(n byte, name string, parent byte) model.Element {
	e := model.Element{ID: tid(n), Kind: model.KindContainer, Name: name, Tags: []string{}, Version: 1}
	if parent != 0 {
		p := tid(parent)
		e.ParentID = &p
	}
	return e
}

func trel(n, src, dst byte, tech string) model.Relation {
	return model.Relation{ID: tid(n), SourceID: tid(src), TargetID: tid(dst), Kind: model.RelUses, Technology: tech, Version: 1}
}

func snap(els []model.Element, rels ...model.Relation) model.Snapshot {
	return model.Snapshot{Elements: els, Relations: rels}
}

func with(e model.Element, f func(*model.Element)) model.Element {
	f(&e)
	return e
}

func names(s model.Snapshot) string {
	var n []string
	for _, e := range s.Elements {
		n = append(n, e.Name)
	}
	slices.Sort(n)
	return strings.Join(n, ",")
}

func elByID(s model.Snapshot, id uuid.UUID) (model.Element, bool) {
	for _, e := range s.Elements {
		if e.ID == id {
			return e, true
		}
	}
	return model.Element{}, false
}

type conflictKey struct {
	kind  changeset.ConflictKind
	id    uuid.UUID
	field string
}

func conflictKeys(cs []changeset.Conflict) []conflictKey {
	out := make([]conflictKey, len(cs))
	for i, c := range cs {
		out[i] = conflictKey{c.Kind, c.TargetID, c.Field}
	}
	return out
}

func TestMerge3(t *testing.T) {
	base := snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL"))
	rename := func(n string) func(*model.Element) { return func(e *model.Element) { e.Name = n } }
	describe := func(d string) func(*model.Element) { return func(e *model.Element) { e.Description = d } }

	tests := []struct {
		name          string
		mine, theirs  model.Snapshot
		res           map[changeset.ResolutionKey]changeset.Choice
		wantConflicts []conflictKey
		check         func(t *testing.T, r model.Snapshot)
	}{
		{
			name: "no changes", mine: base, theirs: base,
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "API,DB,Shop" || len(r.Relations) != 1 {
					t.Fatalf("result = %v", names(r))
				}
			},
		},
		{
			name:   "disjoint fields of one element auto-merge",
			mine:   snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Gateway")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			theirs: snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), describe("edge")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			check: func(t *testing.T, r model.Snapshot) {
				e, _ := elByID(r, tid(2))
				if e.Name != "Gateway" || e.Description != "edge" {
					t.Fatalf("merged = %+v", e)
				}
			},
		},
		{
			name:   "different elements edited on each side",
			mine:   snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("A2")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			theirs: snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), with(tel(3, "DB", 1), rename("D2"))}, trel(10, 2, 3, "SQL")),
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "A2,D2,Shop" {
					t.Fatalf("names = %s", names(r))
				}
			},
		},
		{
			name:   "same value on both sides is not a conflict",
			mine:   snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Same")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			theirs: snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Same")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "DB,Same,Shop" {
					t.Fatalf("names = %s", names(r))
				}
			},
		},
		{
			name:          "same field, different values conflicts",
			mine:          snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Mine")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			theirs:        snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Theirs")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			wantConflicts: []conflictKey{{changeset.ConflictField, tid(2), "name"}},
		},
		{
			name:   "field conflict resolved with mine",
			mine:   snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Mine")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			theirs: snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Theirs")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			res:    map[changeset.ResolutionKey]changeset.Choice{{TargetType: changeset.TargetElement, TargetID: tid(2), Field: "name"}: changeset.ChoiceMine},
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "DB,Mine,Shop" {
					t.Fatalf("names = %s", names(r))
				}
			},
		},
		{
			name:   "field conflict resolved with theirs",
			mine:   snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Mine")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			theirs: snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Theirs")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			res:    map[changeset.ResolutionKey]changeset.Choice{{TargetType: changeset.TargetElement, TargetID: tid(2), Field: "name"}: changeset.ChoiceTheirs},
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "DB,Shop,Theirs" {
					t.Fatalf("names = %s", names(r))
				}
			},
		},
		{
			name:          "mine deletes, theirs modifies",
			mine:          snap([]model.Element{tel(1, "Shop", 0), tel(3, "DB", 1)}),
			theirs:        snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Edited")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			wantConflicts: []conflictKey{{changeset.ConflictDeleteVsUpdate, tid(2), changeset.WholeTarget}},
		},
		{
			name:   "delete-vs-update resolved with mine keeps it deleted",
			mine:   snap([]model.Element{tel(1, "Shop", 0), tel(3, "DB", 1)}),
			theirs: snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Edited")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			res: map[changeset.ResolutionKey]changeset.Choice{
				{TargetType: changeset.TargetElement, TargetID: tid(2), Field: changeset.WholeTarget}: changeset.ChoiceMine,
			},
			check: func(t *testing.T, r model.Snapshot) {
				// The relation touching API was removed on mine's side; it is unchanged by theirs.
				if names(r) != "DB,Shop" || len(r.Relations) != 0 {
					t.Fatalf("result = %s / %d relations", names(r), len(r.Relations))
				}
			},
		},
		{
			name:   "delete-vs-update resolved with theirs keeps the edit",
			mine:   snap([]model.Element{tel(1, "Shop", 0), tel(3, "DB", 1)}),
			theirs: snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Edited")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			res: map[changeset.ResolutionKey]changeset.Choice{
				{TargetType: changeset.TargetElement, TargetID: tid(2), Field: changeset.WholeTarget}: changeset.ChoiceTheirs,
			},
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "DB,Edited,Shop" {
					t.Fatalf("names = %s", names(r))
				}
			},
		},
		{
			name:          "theirs deletes, mine modifies",
			mine:          snap([]model.Element{tel(1, "Shop", 0), with(tel(2, "API", 1), rename("Edited")), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL")),
			theirs:        snap([]model.Element{tel(1, "Shop", 0), tel(3, "DB", 1)}),
			wantConflicts: []conflictKey{{changeset.ConflictDeleteVsUpdate, tid(2), changeset.WholeTarget}},
		},
		{
			name:   "theirs deletes something mine did not touch",
			mine:   base,
			theirs: snap([]model.Element{tel(1, "Shop", 0), tel(3, "DB", 1)}),
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "DB,Shop" || len(r.Relations) != 0 {
					t.Fatalf("result = %s / %d relations", names(r), len(r.Relations))
				}
			},
		},
		{
			name:   "both delete the same element",
			mine:   snap([]model.Element{tel(1, "Shop", 0), tel(3, "DB", 1)}),
			theirs: snap([]model.Element{tel(1, "Shop", 0), tel(3, "DB", 1)}),
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "DB,Shop" {
					t.Fatalf("names = %s", names(r))
				}
			},
		},
		{
			name:   "additions on both sides combine",
			mine:   snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1), tel(20, "MineNew", 1)}, trel(10, 2, 3, "SQL")),
			theirs: snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1), tel(21, "TheirsNew", 1)}, trel(10, 2, 3, "SQL"), trel(11, 21, 3, "SQL")),
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "API,DB,MineNew,Shop,TheirsNew" || len(r.Relations) != 2 {
					t.Fatalf("result = %s / %d relations", names(r), len(r.Relations))
				}
			},
		},
		{
			name:          "mine adds a child under a parent theirs deleted",
			mine:          snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1), tel(20, "Cache", 3)}, trel(10, 2, 3, "SQL")),
			theirs:        snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1)}),
			wantConflicts: []conflictKey{{changeset.ConflictDanglingReference, tid(20), changeset.WholeTarget}},
		},
		{
			name:   "dangling child, resolved mine: parent is restored",
			mine:   snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1), tel(20, "Cache", 3)}, trel(10, 2, 3, "SQL")),
			theirs: snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1)}),
			res: map[changeset.ResolutionKey]changeset.Choice{
				{TargetType: changeset.TargetElement, TargetID: tid(20), Field: changeset.WholeTarget}: changeset.ChoiceMine,
			},
			check: func(t *testing.T, r model.Snapshot) {
				// DB is restored; the relation (unchanged by mine, deleted implicitly by theirs) is gone only if theirs removed it.
				if names(r) != "API,Cache,DB,Shop" {
					t.Fatalf("names = %s", names(r))
				}
			},
		},
		{
			name:   "dangling child, resolved theirs: child is dropped",
			mine:   snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1), tel(20, "Cache", 3), tel(21, "Nested", 20)}, trel(10, 2, 3, "SQL")),
			theirs: snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1)}),
			res: map[changeset.ResolutionKey]changeset.Choice{
				{TargetType: changeset.TargetElement, TargetID: tid(20), Field: changeset.WholeTarget}: changeset.ChoiceTheirs,
			},
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "API,Shop" { // Nested goes with its dropped parent
					t.Fatalf("names = %s", names(r))
				}
			},
		},
		{
			name:   "theirs adds a child under a parent mine deleted",
			mine:   snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1)}),
			theirs: snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1), tel(30, "Replica", 3)}, trel(10, 2, 3, "SQL")),
			// DB itself is unchanged by theirs and deleted by mine -> gone; Replica dangles.
			wantConflicts: []conflictKey{{changeset.ConflictDanglingReference, tid(30), changeset.WholeTarget}},
		},
		{
			name:          "mine adds a relation to an element theirs deleted",
			mine:          snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL"), trel(11, 3, 2, "events")),
			theirs:        snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1)}),
			wantConflicts: []conflictKey{{changeset.ConflictDanglingReference, tid(11), changeset.WholeTarget}},
		},
		{
			name:   "dangling relation resolved theirs: relation dropped",
			mine:   snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL"), trel(11, 3, 2, "events")),
			theirs: snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1)}),
			res: map[changeset.ResolutionKey]changeset.Choice{
				{TargetType: changeset.TargetRelation, TargetID: tid(11), Field: changeset.WholeTarget}: changeset.ChoiceTheirs,
			},
			check: func(t *testing.T, r model.Snapshot) {
				if names(r) != "API,Shop" || len(r.Relations) != 0 {
					t.Fatalf("result = %s / %d relations", names(r), len(r.Relations))
				}
			},
		},
		{
			name:   "relation field edits merge too",
			mine:   snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1)}, with2(trel(10, 2, 3, "SQL"), func(r *model.Relation) { r.Description = "reads" })),
			theirs: snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1)}, trel(10, 2, 3, "pgx")),
			check: func(t *testing.T, r model.Snapshot) {
				if len(r.Relations) != 1 || r.Relations[0].Description != "reads" || r.Relations[0].Technology != "pgx" {
					t.Fatalf("relation = %+v", r.Relations)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := changeset.Merge3(base, tc.mine, tc.theirs, tc.res)
			if got := conflictKeys(out.Conflicts); !slices.Equal(got, tc.wantConflicts) {
				t.Fatalf("conflicts = %+v, want %+v", out.Conflicts, tc.wantConflicts)
			}
			if tc.check != nil && len(tc.wantConflicts) == 0 {
				tc.check(t, out.Result)
			}
		})
	}
}

func with2(r model.Relation, f func(*model.Relation)) model.Relation {
	f(&r)
	return r
}

func TestMerge3ConflictDetails(t *testing.T) {
	base := snap([]model.Element{tel(1, "API", 0)})
	mine := snap([]model.Element{with(tel(1, "API", 0), func(e *model.Element) { e.Name = "Mine" })})
	theirs := snap([]model.Element{with(tel(1, "API", 0), func(e *model.Element) { e.Name = "Theirs" })})

	out := changeset.Merge3(base, mine, theirs, nil)
	if len(out.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v", out.Conflicts)
	}
	c := out.Conflicts[0]
	if c.Label != "Theirs" || string(c.Base) != `"API"` || string(c.Mine) != `"Mine"` || string(c.Theirs) != `"Theirs"` {
		t.Fatalf("conflict = %+v (%s %s %s)", c, c.Base, c.Mine, c.Theirs)
	}
	var js json.RawMessage = c.Mine
	if !json.Valid(js) {
		t.Fatal("mine is not valid JSON")
	}
}

func TestBetweenAndChanges(t *testing.T) {
	base := snap([]model.Element{tel(1, "Shop", 0), tel(2, "API", 1), tel(3, "DB", 1)}, trel(10, 2, 3, "SQL"))
	next := snap([]model.Element{
		tel(1, "Shop", 0),
		with(tel(2, "API", 1), func(e *model.Element) { e.Name = "Gateway"; e.Tags = []string{"edge"} }),
		tel(20, "Child", 21), tel(21, "Parent", 1), // child listed before its parent on purpose
	}, trel(11, 20, 21, "x"))

	d := changeset.Between(base, next)
	if len(d.AddedElements) != 2 || len(d.RemovedElements) != 1 || d.RemovedElements[0].Name != "DB" {
		t.Fatalf("elements: +%d -%d", len(d.AddedElements), len(d.RemovedElements))
	}
	if len(d.ModifiedElements) != 1 || len(d.ModifiedElements[0].Fields) != 2 {
		t.Fatalf("modified = %+v", d.ModifiedElements)
	}
	if d.ModifiedElements[0].Fields[0].Field != "name" || string(d.ModifiedElements[0].Fields[0].Before) != `"API"` {
		t.Fatalf("fields = %+v", d.ModifiedElements[0].Fields)
	}
	if len(d.AddedRelations) != 1 || len(d.RemovedRelations) != 1 || len(d.ModifiedRelations) != 0 {
		t.Fatalf("relations: +%d -%d ~%d", len(d.AddedRelations), len(d.RemovedRelations), len(d.ModifiedRelations))
	}
	if changeset.Between(base, base).Empty() != true {
		t.Fatal("identical snapshots must produce an empty diff")
	}

	// Changes: parents are added before children, adds before updates before deletes.
	changes, err := d.Changes(timeZero)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[uuid.UUID]int{}
	for i, c := range changes {
		pos[c.TargetID] = i
	}
	if pos[tid(21)] > pos[tid(20)] {
		t.Fatal("parent must be added before its child")
	}
	if changes[len(changes)-1].Op != changeset.OpDelete || changes[0].Op != changeset.OpAdd {
		t.Fatalf("unexpected order: %+v", changes)
	}
	// Replaying the changes on base yields next.
	st, err := changeset.Overlay(base, changes)
	if err != nil {
		t.Fatal(err)
	}
	if got := changeset.Between(st.Snapshot(), next); !got.Empty() {
		t.Fatalf("replay differs from target: %+v", got)
	}
}
