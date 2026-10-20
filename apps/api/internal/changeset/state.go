package changeset

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"

	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
)

// State is a mutable in-memory model: main plus applied changes. It is the
// "effective model" of a draft. Deleted entities are removed from the maps.
type State struct {
	elements   map[uuid.UUID]model.Element
	relations  map[uuid.UUID]model.Relation
	owners     map[uuid.UUID]model.Owner
	ownerships map[uuid.UUID]model.Ownership
}

func NewState(base model.Snapshot) *State {
	s := &State{
		elements:   make(map[uuid.UUID]model.Element, len(base.Elements)),
		relations:  make(map[uuid.UUID]model.Relation, len(base.Relations)),
		owners:     make(map[uuid.UUID]model.Owner, len(base.Owners)),
		ownerships: make(map[uuid.UUID]model.Ownership, len(base.Ownerships)),
	}
	for _, e := range base.Elements {
		s.elements[e.ID] = e.Normalized()
	}
	for _, r := range base.Relations {
		s.relations[r.ID] = r
	}
	for _, o := range base.Owners {
		s.owners[o.ID] = o
	}
	for _, o := range base.Ownerships {
		s.ownerships[o.ID] = o
	}
	return s
}

// Overlay returns the effective model: base with the given changes applied in order.
func Overlay(base model.Snapshot, changes []Change) (*State, error) {
	s := NewState(base)
	for _, c := range changes {
		if err := s.Apply(c); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// exists reports whether the state has an entity of the given type and id.
func (s *State) exists(tt TargetType, id uuid.UUID) bool {
	var ok bool
	switch tt {
	case TargetElement:
		_, ok = s.Element(id)
	case TargetRelation:
		_, ok = s.Relation(id)
	case TargetOwner:
		_, ok = s.Owner(id)
	case TargetOwnership:
		_, ok = s.Ownership(id)
	}
	return ok
}

func (s *State) Element(id uuid.UUID) (model.Element, bool) {
	e, ok := s.elements[id]
	return e, ok
}

func (s *State) Relation(id uuid.UUID) (model.Relation, bool) {
	r, ok := s.relations[id]
	return r, ok
}

func (s *State) Owner(id uuid.UUID) (model.Owner, bool) {
	o, ok := s.owners[id]
	return o, ok
}

func (s *State) Ownership(id uuid.UUID) (model.Ownership, bool) {
	o, ok := s.ownerships[id]
	return o, ok
}

// Apply applies one change; inconsistencies (unknown target, duplicate add) are errors.
func (s *State) Apply(c Change) error {
	switch c.TargetType {
	case TargetElement:
		return s.applyElement(c)
	case TargetRelation:
		return s.applyRelation(c)
	case TargetOwner:
		return applyEntity(s.owners, c, "owner",
			func() model.Owner { return model.Owner{ID: c.TargetID, Version: 1, CreatedAt: c.CreatedAt} },
			model.ApplyOwnerPatch, func(o *model.Owner) { o.UpdatedAt = c.CreatedAt })
	case TargetOwnership:
		return applyEntity(s.ownerships, c, "ownership",
			func() model.Ownership { return model.Ownership{ID: c.TargetID, Version: 1, CreatedAt: c.CreatedAt} },
			model.ApplyOwnershipPatch, func(o *model.Ownership) { o.UpdatedAt = c.CreatedAt })
	}
	return apperr.Invalid("unknown target type %q", c.TargetType)
}

func (s *State) applyElement(c Change) error {
	cur, exists := s.elements[c.TargetID]
	switch c.Op {
	case OpAdd:
		if exists {
			return apperr.Conflict("element %s already exists", c.TargetID)
		}
		e, err := model.ApplyElementPatch(model.Element{ID: c.TargetID, Version: 1, CreatedAt: c.CreatedAt, UpdatedAt: c.CreatedAt}, c.Patch)
		if err != nil {
			return err
		}
		s.elements[e.ID] = e
	case OpUpdate:
		if !exists {
			return apperr.NotFound("element %s not found", c.TargetID)
		}
		e, err := model.ApplyElementPatch(cur, c.Patch)
		if err != nil {
			return err
		}
		e.UpdatedAt = c.CreatedAt
		s.elements[e.ID] = e
	case OpDelete:
		if !exists {
			return apperr.NotFound("element %s not found", c.TargetID)
		}
		delete(s.elements, c.TargetID)
	default:
		return apperr.Invalid("unknown op %q", c.Op)
	}
	return nil
}

func (s *State) applyRelation(c Change) error {
	cur, exists := s.relations[c.TargetID]
	switch c.Op {
	case OpAdd:
		if exists {
			return apperr.Conflict("relation %s already exists", c.TargetID)
		}
		r, err := model.ApplyRelationPatch(model.Relation{ID: c.TargetID, Version: 1, CreatedAt: c.CreatedAt, UpdatedAt: c.CreatedAt}, c.Patch)
		if err != nil {
			return err
		}
		s.relations[r.ID] = r
	case OpUpdate:
		if !exists {
			return apperr.NotFound("relation %s not found", c.TargetID)
		}
		r, err := model.ApplyRelationPatch(cur, c.Patch)
		if err != nil {
			return err
		}
		r.UpdatedAt = c.CreatedAt
		s.relations[r.ID] = r
	case OpDelete:
		if !exists {
			return apperr.NotFound("relation %s not found", c.TargetID)
		}
		delete(s.relations, c.TargetID)
	default:
		return apperr.Invalid("unknown op %q", c.Op)
	}
	return nil
}

// applyEntity applies one change to a map of entities (owners, ownerships).
func applyEntity[T any](m map[uuid.UUID]T, c Change, what string, fresh func() T,
	apply func(T, json.RawMessage) (T, error), touch func(*T),
) error {
	cur, exists := m[c.TargetID]
	switch c.Op {
	case OpAdd:
		if exists {
			return apperr.Conflict("%s %s already exists", what, c.TargetID)
		}
		v, err := apply(fresh(), c.Patch)
		if err != nil {
			return err
		}
		touch(&v)
		m[c.TargetID] = v
	case OpUpdate:
		if !exists {
			return apperr.NotFound("%s %s not found", what, c.TargetID)
		}
		v, err := apply(cur, c.Patch)
		if err != nil {
			return err
		}
		touch(&v)
		m[c.TargetID] = v
	case OpDelete:
		if !exists {
			return apperr.NotFound("%s %s not found", what, c.TargetID)
		}
		delete(m, c.TargetID)
	default:
		return apperr.Invalid("unknown op %q", c.Op)
	}
	return nil
}

// Snapshot returns the state in the same order the main repository uses.
func (s *State) Snapshot() model.Snapshot {
	els := make([]model.Element, 0, len(s.elements))
	for _, e := range s.elements {
		els = append(els, e)
	}
	slices.SortFunc(els, func(a, b model.Element) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	rels := make([]model.Relation, 0, len(s.relations))
	for _, r := range s.relations {
		rels = append(rels, r)
	}
	slices.SortFunc(rels, func(a, b model.Relation) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	owners := make([]model.Owner, 0, len(s.owners))
	for _, o := range s.owners {
		owners = append(owners, o)
	}
	slices.SortFunc(owners, func(a, b model.Owner) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	ownerships := make([]model.Ownership, 0, len(s.ownerships))
	for _, o := range s.ownerships {
		ownerships = append(ownerships, o)
	}
	slices.SortFunc(ownerships, func(a, b model.Ownership) int {
		if c := strings.Compare(a.ElementID.String(), b.ElementID.String()); c != 0 {
			return c
		}
		return strings.Compare(string(a.Role), string(b.Role))
	})
	return model.Snapshot{Elements: els, Relations: rels, Owners: owners, Ownerships: ownerships}
}

// ownershipsOf returns the ids of ownerships of the given elements or owners.
func (s *State) ownershipsOf(elements, owners map[uuid.UUID]bool) []uuid.UUID {
	var out []uuid.UUID
	for _, o := range s.ownerships {
		if elements[o.ElementID] || owners[o.OwnerID] {
			out = append(out, o.ID)
		}
	}
	slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return out
}

// descendants returns all descendants of id, deepest first (safe delete order).
func (s *State) descendants(id uuid.UUID) []uuid.UUID {
	children := map[uuid.UUID][]uuid.UUID{}
	for _, e := range s.elements {
		if e.ParentID != nil {
			children[*e.ParentID] = append(children[*e.ParentID], e.ID)
		}
	}
	var out []uuid.UUID
	var walk func(uuid.UUID)
	walk = func(p uuid.UUID) {
		kids := children[p]
		slices.SortFunc(kids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		for _, k := range kids {
			walk(k)
			out = append(out, k) // post-order: children before their parent
		}
	}
	walk(id)
	return out
}

// relationsTouching returns the ids of relations whose source or target is in ids.
func (s *State) relationsTouching(ids map[uuid.UUID]bool) []uuid.UUID {
	var out []uuid.UUID
	for _, r := range s.relations {
		if ids[r.SourceID] || ids[r.TargetID] {
			out = append(out, r.ID)
		}
	}
	slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return out
}

// isDescendantOrSelf reports whether candidate is ancestor or one of its descendants.
func (s *State) isDescendantOrSelf(candidate, ancestor uuid.UUID) bool {
	cur := candidate
	for hops := 0; hops <= len(s.elements); hops++ {
		if cur == ancestor {
			return true
		}
		e, ok := s.elements[cur]
		if !ok || e.ParentID == nil {
			return false
		}
		cur = *e.ParentID
	}
	return true // cycle in existing data; treat as unsafe
}
