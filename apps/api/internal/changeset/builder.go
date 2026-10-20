package changeset

import (
	"encoding/json"
	"strings"
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
	// principals are the users and groups owners may point at (nil = none).
	principals model.Principals
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
	case in.TargetType == TargetOwner && in.Op == OpAdd:
		return b.addOwner(in)
	case in.TargetType == TargetOwner && in.Op == OpUpdate:
		return b.updateOwner(in)
	case in.TargetType == TargetOwner && in.Op == OpDelete:
		return b.deleteOwner(in)
	case in.TargetType == TargetOwnership && in.Op == OpAdd:
		return b.addOwnership(in)
	case in.TargetType == TargetOwnership && in.Op == OpUpdate:
		return b.updateOwnership(in)
	case in.TargetType == TargetOwnership && in.Op == OpDelete:
		if err := requireTarget(in); err != nil {
			return err
		}
		return b.deleteOwnershipByID(in.TargetID)
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
	// Relations and ownerships first, then elements children-before-parent, all as
	// explicit changes so the diff shows exactly what a delete removes.
	for _, rid := range b.st.relationsTouching(doomed) {
		if err := b.deleteRelationByID(rid); err != nil {
			return err
		}
	}
	for _, oid := range b.st.ownershipsOf(doomed, nil) {
		if err := b.deleteOwnershipByID(oid); err != nil {
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
		case "layer", "kind", "type", "tags", "technology", "name":
			return model.CheckLayer(b.rules, next)
		}
	}
	return nil
}

func (b *builder) addOwner(in ChangeInput) error {
	if err := requirePatch(in); err != nil {
		return err
	}
	o, err := model.ApplyOwnerPatch(model.Owner{Version: 1}, in.Patch)
	if err != nil {
		return err
	}
	if o.PrincipalID != nil {
		// One owner per user / group, with an id derived from it (ADR-069).
		o.ID = model.PrincipalOwnerID(*o.PrincipalID)
		if in.TargetID != uuid.Nil && in.TargetID != o.ID {
			return apperr.Invalid("a %s owner's id must be derived from principal_id (expected %s)", o.Kind, o.ID)
		}
		if p, ok := b.principals[*o.PrincipalID]; ok && strings.TrimSpace(o.Name) == "" {
			o.Name = p.Name // the name at the time; reads show the live one
		}
	} else if o.ID, err = b.newID(in); err != nil {
		return err
	}
	if _, exists := b.st.Owner(o.ID); exists {
		return apperr.Conflict("owner %s already exists", o.ID)
	}
	if err := b.validateOwner(o); err != nil {
		return err
	}
	full, err := model.FieldsOf(o, model.OwnerPatchFields)
	if err != nil {
		return err
	}
	return b.record(OpAdd, TargetOwner, o.ID, full, nil)
}

func (b *builder) updateOwner(in ChangeInput) error {
	if err := requireTarget(in); err != nil {
		return err
	}
	if err := requirePatch(in); err != nil {
		return err
	}
	cur, ok := b.st.Owner(in.TargetID)
	if !ok {
		return apperr.NotFound("owner %s not found", in.TargetID)
	}
	next, err := model.ApplyOwnerPatch(cur, in.Patch)
	if err != nil {
		return err
	}
	if err := b.validateOwner(next); err != nil {
		return err
	}
	return b.recordUpdate(TargetOwner, cur.ID, cur, in.Patch)
}

// deleteOwner removes an owner. Its assignments go with it only when the caller
// says so (cascade), so a team is never unassigned from elements by accident.
func (b *builder) deleteOwner(in ChangeInput) error {
	if err := requireTarget(in); err != nil {
		return err
	}
	cur, ok := b.st.Owner(in.TargetID)
	if !ok {
		return apperr.NotFound("owner %s not found", in.TargetID)
	}
	assigned := b.st.ownershipsOf(nil, map[uuid.UUID]bool{cur.ID: true})
	if len(assigned) > 0 && !in.Cascade {
		return apperr.Invalid("owner %q is assigned to %d element role(s); set cascade to remove the assignments too", cur.Name, len(assigned))
	}
	for _, oid := range assigned {
		if err := b.deleteOwnershipByID(oid); err != nil {
			return err
		}
	}
	before, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	return b.record(OpDelete, TargetOwner, cur.ID, nil, before)
}

// validateOwner checks the owner. Named owners need a unique name (ignoring case);
// user and group owners need a usable principal of the matching kind.
func (b *builder) validateOwner(o model.Owner) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if o.Kind.IsPrincipal() {
		p, ok := b.principals[*o.PrincipalID]
		switch {
		case !ok:
			return apperr.Invalid("%s %s does not exist", o.Kind, *o.PrincipalID)
		case p.Kind != o.Kind:
			return apperr.Invalid("%s is a %s, not a %s", *o.PrincipalID, p.Kind, o.Kind)
		case p.Inactive:
			return apperr.Invalid("%s %q is disabled or deleted and cannot become an owner", o.Kind, p.Name)
		}
		return nil
	}
	for _, other := range b.st.owners {
		if other.ID != o.ID && !other.Kind.IsPrincipal() && strings.EqualFold(strings.TrimSpace(other.Name), o.Name) {
			return apperr.Invalid("an owner named %q already exists", other.Name)
		}
	}
	return nil
}

// addOwnership assigns an owner to an element in a role. The id is derived from
// (element, role), so a role holds one owner; replacing it is an update.
func (b *builder) addOwnership(in ChangeInput) error {
	if err := requirePatch(in); err != nil {
		return err
	}
	o, err := model.ApplyOwnershipPatch(model.Ownership{Version: 1}, in.Patch)
	if err != nil {
		return err
	}
	o.ID = model.OwnershipID(o.ElementID, o.Role)
	if in.TargetID != uuid.Nil && in.TargetID != o.ID {
		return apperr.Invalid("ownership id must be derived from element_id and role (expected %s)", o.ID)
	}
	if err := b.validateOwnership(o); err != nil {
		return err
	}
	if _, exists := b.st.Ownership(o.ID); exists {
		el, _ := b.st.Element(o.ElementID)
		return apperr.Conflict("%q already has an owner in role %s; change that assignment instead", el.Name, o.Role)
	}
	full, err := model.FieldsOf(o, model.OwnershipPatchFields)
	if err != nil {
		return err
	}
	return b.record(OpAdd, TargetOwnership, o.ID, full, nil)
}

// updateOwnership replaces the owner of an assignment; element and role are fixed.
func (b *builder) updateOwnership(in ChangeInput) error {
	if err := requireTarget(in); err != nil {
		return err
	}
	if err := requirePatch(in); err != nil {
		return err
	}
	cur, ok := b.st.Ownership(in.TargetID)
	if !ok {
		return apperr.NotFound("ownership %s not found", in.TargetID)
	}
	keys, err := model.PatchKeys(in.Patch)
	if err != nil {
		return err
	}
	if len(keys) != 1 || keys[0] != "owner_id" {
		return apperr.Invalid("an ownership update may only change owner_id; delete and add to move it")
	}
	next, err := model.ApplyOwnershipPatch(cur, in.Patch)
	if err != nil {
		return err
	}
	if err := b.validateOwnership(next); err != nil {
		return err
	}
	return b.recordUpdate(TargetOwnership, cur.ID, cur, in.Patch)
}

func (b *builder) deleteOwnershipByID(id uuid.UUID) error {
	cur, ok := b.st.Ownership(id)
	if !ok {
		return apperr.NotFound("ownership %s not found", id)
	}
	before, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	return b.record(OpDelete, TargetOwnership, id, nil, before)
}

func (b *builder) validateOwnership(o model.Ownership) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if err := b.requireElement(o.ElementID, "element"); err != nil {
		return err
	}
	if _, ok := b.st.Owner(o.OwnerID); !ok {
		return apperr.Invalid("owner %s does not exist", o.OwnerID)
	}
	return nil
}

// recordUpdate records an update with the previous values of the patched fields.
func (b *builder) recordUpdate(tt TargetType, id uuid.UUID, cur any, patch json.RawMessage) error {
	keys, err := model.PatchKeys(patch)
	if err != nil {
		return err
	}
	before, err := model.FieldsOf(cur, keys)
	if err != nil {
		return err
	}
	return b.record(OpUpdate, tt, id, patch, before)
}
