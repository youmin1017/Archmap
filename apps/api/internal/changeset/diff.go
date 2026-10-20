package changeset

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/model"
)

// FieldChange is one modified field of an entity (values are raw JSON).
type FieldChange struct {
	Field  string
	Before json.RawMessage
	After  json.RawMessage
}

type ElementMod struct {
	Before, After model.Element
	Fields        []FieldChange
}

type RelationMod struct {
	Before, After model.Relation
	Fields        []FieldChange
}

// Mod is a modified entity with the fields that changed.
type Mod[T any] struct {
	Before, After T
	Fields        []FieldChange
}

type (
	OwnerMod     = Mod[model.Owner]
	OwnershipMod = Mod[model.Ownership]
)

// Diff is the structured difference between two model snapshots.
type Diff struct {
	AddedElements      []model.Element
	RemovedElements    []model.Element
	ModifiedElements   []ElementMod
	AddedRelations     []model.Relation
	RemovedRelations   []model.Relation
	ModifiedRelations  []RelationMod
	AddedOwners        []model.Owner
	RemovedOwners      []model.Owner
	ModifiedOwners     []OwnerMod
	AddedOwnerships    []model.Ownership
	RemovedOwnerships  []model.Ownership
	ModifiedOwnerships []OwnershipMod
}

func (d Diff) Empty() bool {
	return len(d.AddedElements)+len(d.RemovedElements)+len(d.ModifiedElements)+
		len(d.AddedRelations)+len(d.RemovedRelations)+len(d.ModifiedRelations)+
		len(d.AddedOwners)+len(d.RemovedOwners)+len(d.ModifiedOwners)+
		len(d.AddedOwnerships)+len(d.RemovedOwnerships)+len(d.ModifiedOwnerships) == 0
}

// diffSet compares two entity lists by id; results are sorted by id.
func diffSet[T any](base, next []T, idOf func(T) uuid.UUID, fm func(T) map[string]json.RawMessage, order []string) (added, removed []T, modified []Mod[T]) {
	baseByID := make(map[uuid.UUID]T, len(base))
	for _, v := range base {
		baseByID[idOf(v)] = v
	}
	nextIDs := make(map[uuid.UUID]bool, len(next))
	for _, v := range next {
		nextIDs[idOf(v)] = true
		b, ok := baseByID[idOf(v)]
		if !ok {
			added = append(added, v)
			continue
		}
		if fields := diffFields(fm(b), fm(v), order); len(fields) > 0 {
			modified = append(modified, Mod[T]{Before: b, After: v, Fields: fields})
		}
	}
	for _, v := range base {
		if !nextIDs[idOf(v)] {
			removed = append(removed, v)
		}
	}
	cmp := func(a, b T) int { return strings.Compare(idOf(a).String(), idOf(b).String()) }
	slices.SortFunc(added, cmp)
	slices.SortFunc(removed, cmp)
	slices.SortFunc(modified, func(a, b Mod[T]) int { return cmp(a.After, b.After) })
	return added, removed, modified
}

// Between computes what changes when going from base to next.
func Between(base, next model.Snapshot) Diff {
	var d Diff

	baseEls := indexElements(base.Elements)
	nextEls := indexElements(next.Elements)
	for _, e := range next.Elements {
		b, ok := baseEls[e.ID]
		if !ok {
			d.AddedElements = append(d.AddedElements, e)
			continue
		}
		if fields := diffFields(b.FieldMap(), e.FieldMap(), model.ElementPatchFields); len(fields) > 0 {
			d.ModifiedElements = append(d.ModifiedElements, ElementMod{Before: b, After: e, Fields: fields})
		}
	}
	for _, e := range base.Elements {
		if _, ok := nextEls[e.ID]; !ok {
			d.RemovedElements = append(d.RemovedElements, e)
		}
	}

	baseRels := indexRelations(base.Relations)
	nextRels := indexRelations(next.Relations)
	for _, r := range next.Relations {
		b, ok := baseRels[r.ID]
		if !ok {
			d.AddedRelations = append(d.AddedRelations, r)
			continue
		}
		if fields := diffFields(b.FieldMap(), r.FieldMap(), model.RelationPatchFields); len(fields) > 0 {
			d.ModifiedRelations = append(d.ModifiedRelations, RelationMod{Before: b, After: r, Fields: fields})
		}
	}
	for _, r := range base.Relations {
		if _, ok := nextRels[r.ID]; !ok {
			d.RemovedRelations = append(d.RemovedRelations, r)
		}
	}

	byName := func(a, b model.Element) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	}
	byID := func(a, b model.Relation) int { return strings.Compare(a.ID.String(), b.ID.String()) }
	slices.SortFunc(d.AddedElements, byName)
	slices.SortFunc(d.RemovedElements, byName)
	slices.SortFunc(d.ModifiedElements, func(a, b ElementMod) int { return byName(a.After, b.After) })
	slices.SortFunc(d.AddedRelations, byID)
	slices.SortFunc(d.RemovedRelations, byID)
	slices.SortFunc(d.ModifiedRelations, func(a, b RelationMod) int { return byID(a.After, b.After) })

	d.AddedOwners, d.RemovedOwners, d.ModifiedOwners = diffSet(base.Owners, next.Owners,
		func(o model.Owner) uuid.UUID { return o.ID }, model.Owner.FieldMap, model.OwnerPatchFields)
	d.AddedOwnerships, d.RemovedOwnerships, d.ModifiedOwnerships = diffSet(base.Ownerships, next.Ownerships,
		func(o model.Ownership) uuid.UUID { return o.ID }, model.Ownership.FieldMap, model.OwnershipPatchFields)
	return d
}

func indexElements(els []model.Element) map[uuid.UUID]model.Element {
	m := make(map[uuid.UUID]model.Element, len(els))
	for _, e := range els {
		m[e.ID] = e
	}
	return m
}

func indexRelations(rels []model.Relation) map[uuid.UUID]model.Relation {
	m := make(map[uuid.UUID]model.Relation, len(rels))
	for _, r := range rels {
		m[r.ID] = r
	}
	return m
}

// diffFields lists fields whose raw JSON differs, in canonical field order.
func diffFields(before, after map[string]json.RawMessage, order []string) []FieldChange {
	var out []FieldChange
	for _, f := range order {
		if !bytes.Equal(before[f], after[f]) {
			out = append(out, FieldChange{Field: f, Before: before[f], After: after[f]})
		}
	}
	return out
}

// Changes converts the diff into changes that can be materialized into main
// (used by merge): adds (parents first, elements before relations), then
// updates, then deletes. Soft deletes do not violate foreign keys, so deletes
// need no particular order.
func (d Diff) Changes(now time.Time) ([]Change, error) {
	var out []Change
	emit := func(op Op, tt TargetType, id uuid.UUID, patch json.RawMessage) {
		if patch == nil {
			patch = json.RawMessage(`{}`)
		}
		out = append(out, Change{Op: op, TargetType: tt, TargetID: id, Patch: patch, CreatedAt: now})
	}

	for _, o := range d.AddedOwners {
		patch, err := model.FieldsOf(o, model.OwnerPatchFields)
		if err != nil {
			return nil, err
		}
		emit(OpAdd, TargetOwner, o.ID, patch)
	}
	for _, e := range parentsFirst(d.AddedElements) {
		patch, err := model.FieldsOf(e, model.ElementPatchFields)
		if err != nil {
			return nil, err
		}
		emit(OpAdd, TargetElement, e.ID, patch)
	}
	for _, r := range d.AddedRelations {
		patch, err := model.FieldsOf(r, model.RelationPatchFields)
		if err != nil {
			return nil, err
		}
		emit(OpAdd, TargetRelation, r.ID, patch)
	}
	for _, o := range d.AddedOwnerships {
		patch, err := model.FieldsOf(o, model.OwnershipPatchFields)
		if err != nil {
			return nil, err
		}
		emit(OpAdd, TargetOwnership, o.ID, patch)
	}
	for _, m := range d.ModifiedOwners {
		emit(OpUpdate, TargetOwner, m.After.ID, fieldsPatch(m.Fields))
	}
	for _, m := range d.ModifiedElements {
		emit(OpUpdate, TargetElement, m.After.ID, fieldsPatch(m.Fields))
	}
	for _, m := range d.ModifiedRelations {
		emit(OpUpdate, TargetRelation, m.After.ID, fieldsPatch(m.Fields))
	}
	for _, m := range d.ModifiedOwnerships {
		emit(OpUpdate, TargetOwnership, m.After.ID, fieldsPatch(m.Fields))
	}
	for _, o := range d.RemovedOwnerships {
		emit(OpDelete, TargetOwnership, o.ID, nil)
	}
	for _, r := range d.RemovedRelations {
		emit(OpDelete, TargetRelation, r.ID, nil)
	}
	for _, e := range d.RemovedElements {
		emit(OpDelete, TargetElement, e.ID, nil)
	}
	for _, o := range d.RemovedOwners {
		emit(OpDelete, TargetOwner, o.ID, nil)
	}
	return out, nil
}

func fieldsPatch(fields []FieldChange) json.RawMessage {
	m := make(map[string]json.RawMessage, len(fields))
	for _, f := range fields {
		m[f.Field] = f.After
	}
	raw, err := json.Marshal(m)
	if err != nil {
		panic(err) // raw JSON values always marshal
	}
	return raw
}

// parentsFirst orders elements so every element comes after its parent when
// the parent is part of the same set.
func parentsFirst(els []model.Element) []model.Element {
	inSet := make(map[uuid.UUID]bool, len(els))
	for _, e := range els {
		inSet[e.ID] = true
	}
	placed := make(map[uuid.UUID]bool, len(els))
	out := make([]model.Element, 0, len(els))
	remaining := slices.Clone(els)
	for len(remaining) > 0 {
		progress := false
		next := remaining[:0:0]
		for _, e := range remaining {
			if e.ParentID == nil || !inSet[*e.ParentID] || placed[*e.ParentID] {
				out = append(out, e)
				placed[e.ID] = true
				progress = true
			} else {
				next = append(next, e)
			}
		}
		if !progress { // cycle (invalid input); emit the rest as-is
			out = append(out, next...)
			break
		}
		remaining = next
	}
	return out
}
