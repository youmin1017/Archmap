package changeset

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
)

// builder turns ChangeInputs into recorded Changes. Each produced change is
// validated against, and applied to, the working State, so later inputs in the
// same batch see the effect of earlier ones. Validation here is structural;
// business rules (hierarchy, duplicates) belong to the lint engine.
type builder struct {
	st    *State
	batch uuid.UUID
	now   time.Time
	out   []Change
	rules []model.LayerRule // layer rules to enforce (nil = none)
}

func (b *builder) apply(in ChangeInput) error {
	switch {
	case in.TargetType == TargetElement && in.Op == OpAdd:
		return b.addElement(in)
	case in.TargetType == TargetElement && in.Op == OpUpdate:
		return b.updateElement(in)
	case in.TargetType == TargetElement && in.Op == OpDelete:
		return b.deleteElement(in)
	case in.TargetType == TargetRelation && in.Op == OpAdd:
		return b.addRelation(in)
	case in.TargetType == TargetRelation && in.Op == OpUpdate:
		return b.updateRelation(in)
	case in.TargetType == TargetRelation && in.Op == OpDelete:
		return b.deleteRelation(in)
	}
	return apperr.Invalid("unsupported change %q on %q", in.Op, in.TargetType)
}

func (b *builder) record(op Op, tt TargetType, id uuid.UUID, patch, before json.RawMessage) error {
	if patch == nil {
		patch = json.RawMessage(`{}`)
	}
	c := Change{
		BatchID: b.batch, Op: op, TargetType: tt, TargetID: id,
		Patch: patch, Before: before, CreatedAt: b.now,
	}
	if err := b.st.Apply(c); err != nil {
		return err
	}
	b.out = append(b.out, c)
	return nil
}

func (b *builder) newID(in ChangeInput) (uuid.UUID, error) {
	if in.TargetID != uuid.Nil {
		return in.TargetID, nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func requirePatch(in ChangeInput) error {
	if len(in.Patch) == 0 {
		return apperr.Invalid("%s %s requires a patch", in.Op, in.TargetType)
	}
	return nil
}

func requireTarget(in ChangeInput) error {
	if in.TargetID == uuid.Nil {
		return apperr.Invalid("%s %s requires target_id", in.Op, in.TargetType)
	}
	return nil
}

func (b *builder) requireElement(id uuid.UUID, what string) error {
	if _, ok := b.st.Element(id); !ok {
		return apperr.Invalid("%s %s does not exist", what, id)
	}
	return nil
}

func (b *builder) addElement(in ChangeInput) error {
	if err := requirePatch(in); err != nil {
		return err
	}
	id, err := b.newID(in)
	if err != nil {
		return err
	}
	if _, exists := b.st.Element(id); exists {
		return apperr.Conflict("element %s already exists", id)
	}
	e, err := model.ApplyElementPatch(model.Element{ID: id, Version: 1, Replicas: 1}, in.Patch)
	if err != nil {
		return err
	}
	if err := e.Validate(); err != nil {
		return err
	}
	if err := model.CheckLayer(b.rules, e); err != nil {
		return err
	}
	if e.ParentID != nil {
		if err := b.requireElement(*e.ParentID, "parent"); err != nil {
			return err
		}
	}
	full, err := model.FieldsOf(e, model.ElementPatchFields) // normalized: full new entity
	if err != nil {
		return err
	}
	return b.record(OpAdd, TargetElement, id, full, nil)
}

func (b *builder) updateElement(in ChangeInput) error {
	if err := requireTarget(in); err != nil {
		return err
	}
	if err := requirePatch(in); err != nil {
		return err
	}
	cur, ok := b.st.Element(in.TargetID)
	if !ok {
		return apperr.NotFound("element %s not found", in.TargetID)
	}
	next, err := model.ApplyElementPatch(cur, in.Patch)
	if err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if err := b.checkLayerOnUpdate(in, next); err != nil {
		return err
	}
	if next.ParentID != nil {
		if err := b.requireElement(*next.ParentID, "parent"); err != nil {
			return err
		}
		if b.st.isDescendantOrSelf(*next.ParentID, cur.ID) {
			return apperr.Invalid("element %s cannot be moved under itself or its descendants", cur.ID)
		}
	}
	keys, err := model.PatchKeys(in.Patch)
	if err != nil {
		return err
	}
	before, err := model.FieldsOf(cur, keys)
	if err != nil {
		return err
	}
	return b.record(OpUpdate, TargetElement, cur.ID, in.Patch, before)
}

func (b *builder) deleteElement(in ChangeInput) error {
	if err := requireTarget(in); err != nil {
		return err
	}
	cur, ok := b.st.Element(in.TargetID)
	if !ok {
		return apperr.NotFound("element %s not found", in.TargetID)
	}
	descendants := b.st.descendants(cur.ID)
	if len(descendants) > 0 && !in.Cascade {
		return apperr.Invalid("element %s has %d descendant(s); set cascade to delete them too", cur.ID, len(descendants))
	}
	doomed := map[uuid.UUID]bool{cur.ID: true}
	for _, d := range descendants {
		doomed[d] = true
	}
	// Relations first, then elements children-before-parent, all as explicit
	// changes so the diff shows exactly what a delete removes.
	for _, rid := range b.st.relationsTouching(doomed) {
		if err := b.deleteRelationByID(rid); err != nil {
			return err
		}
	}
	for _, id := range append(descendants, cur.ID) {
		el, _ := b.st.Element(id)
		before, err := json.Marshal(el)
		if err != nil {
			return err
		}
		if err := b.record(OpDelete, TargetElement, id, nil, before); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) addRelation(in ChangeInput) error {
	if err := requirePatch(in); err != nil {
		return err
	}
	id, err := b.newID(in)
	if err != nil {
		return err
	}
	if _, exists := b.st.Relation(id); exists {
		return apperr.Conflict("relation %s already exists", id)
	}
	r, err := model.ApplyRelationPatch(model.Relation{ID: id, Version: 1}, in.Patch)
	if err != nil {
		return err
	}
	if err := b.validateRelation(r); err != nil {
		return err
	}
	full, err := model.FieldsOf(r, model.RelationPatchFields)
	if err != nil {
		return err
	}
	return b.record(OpAdd, TargetRelation, id, full, nil)
}

func (b *builder) updateRelation(in ChangeInput) error {
	if err := requireTarget(in); err != nil {
		return err
	}
	if err := requirePatch(in); err != nil {
		return err
	}
	cur, ok := b.st.Relation(in.TargetID)
	if !ok {
		return apperr.NotFound("relation %s not found", in.TargetID)
	}
	next, err := model.ApplyRelationPatch(cur, in.Patch)
	if err != nil {
		return err
	}
	if err := b.validateRelation(next); err != nil {
		return err
	}
	keys, err := model.PatchKeys(in.Patch)
	if err != nil {
		return err
	}
	before, err := model.FieldsOf(cur, keys)
	if err != nil {
		return err
	}
	return b.record(OpUpdate, TargetRelation, cur.ID, in.Patch, before)
}

func (b *builder) deleteRelation(in ChangeInput) error {
	if err := requireTarget(in); err != nil {
		return err
	}
	return b.deleteRelationByID(in.TargetID)
}

func (b *builder) deleteRelationByID(id uuid.UUID) error {
	cur, ok := b.st.Relation(id)
	if !ok {
		return apperr.NotFound("relation %s not found", id)
	}
	before, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	return b.record(OpDelete, TargetRelation, id, nil, before)
}

func (b *builder) validateRelation(r model.Relation) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := b.requireElement(r.SourceID, "relation source"); err != nil {
		return err
	}
	return b.requireElement(r.TargetID, "relation target")
}

// checkLayerOnUpdate enforces the layer rules only when the patch touches what
// decides the layer or the class (layer, kind, tags, technology, name), so an
// element that already violates a newer rule can still be edited otherwise.
func (b *builder) checkLayerOnUpdate(in ChangeInput, next model.Element) error {
	if len(b.rules) == 0 {
		return nil
	}
	keys, err := model.PatchKeys(in.Patch)
	if err != nil {
		return err
	}
	for _, k := range keys {
		switch k {
		case "layer", "kind", "tags", "technology", "name":
			return model.CheckLayer(b.rules, next)
		}
	}
	return nil
}
