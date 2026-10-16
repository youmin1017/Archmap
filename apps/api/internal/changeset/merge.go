package changeset

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"

	"archmap/api/internal/model"
)

// ConflictKind says why a merge cannot be resolved automatically.
type ConflictKind string

const (
	// ConflictField: both sides changed the same field to different values.
	ConflictField ConflictKind = "field"
	// ConflictDeleteVsUpdate: one side deleted a target the other side modified.
	ConflictDeleteVsUpdate ConflictKind = "delete_vs_update"
	// ConflictDanglingReference: after merging, an entity references a deleted
	// one (e.g. a new child under a parent the other side deleted).
	ConflictDanglingReference ConflictKind = "dangling_reference"
)

// WholeTarget is the Field value of conflicts that concern a target as a whole.
const WholeTarget = "*"

type Choice string

const (
	ChoiceMine   Choice = "mine"
	ChoiceTheirs Choice = "theirs"
)

func (c Choice) Valid() bool { return c == ChoiceMine || c == ChoiceTheirs }

// ResolutionKey identifies what a resolution applies to.
type ResolutionKey struct {
	TargetType TargetType
	TargetID   uuid.UUID
	Field      string // field name or WholeTarget
}

type Resolution struct {
	ResolutionKey
	Choice Choice
}

type Conflict struct {
	Kind       ConflictKind
	TargetType TargetType
	TargetID   uuid.UUID
	Field      string
	Label      string // human name of the target
	Message    string
	// Base, Mine, Theirs are raw JSON values: the field value for field
	// conflicts, the entity's fields (or null when deleted) for whole-target ones.
	Base, Mine, Theirs json.RawMessage
}

func (c Conflict) Key() ResolutionKey {
	return ResolutionKey{TargetType: c.TargetType, TargetID: c.TargetID, Field: c.Field}
}

// MergeOutcome is the result of a 3-way merge. When Conflicts is non-empty the
// Result is provisional and must not be applied.
type MergeOutcome struct {
	Result    model.Snapshot
	Conflicts []Conflict
}

// Merge3 merges mine (base + a draft's changes) with theirs (main since base)
// per target and per field. Disjoint edits merge automatically; same-field
// edits and delete-vs-update become conflicts unless a resolution says whose
// side wins. Nothing is ever silently overwritten.
func Merge3(base, mine, theirs model.Snapshot, resolutions map[ResolutionKey]Choice) MergeOutcome {
	m := &merger{
		res:           resolutions,
		mineEls:       indexElements(mine.Elements),
		theirEls:      indexElements(theirs.Elements),
		inherit:       map[uuid.UUID]Choice{},
		dropped:       map[uuid.UUID]bool{},
		seenConflicts: map[ResolutionKey]bool{},
	}

	m.els = mergeSet(m, TargetElement,
		indexElements(base.Elements), m.mineEls, m.theirEls,
		model.Element.FieldMap, model.ApplyElementPatch,
		func(e model.Element) string { return e.Name }, func(e model.Element) uuid.UUID { return e.ID })
	m.rels = mergeSet(m, TargetRelation,
		indexRelations(base.Relations), indexRelations(mine.Relations), indexRelations(theirs.Relations),
		model.Relation.FieldMap, model.ApplyRelationPatch,
		func(r model.Relation) string { return string(r.Kind) + " relation" }, func(r model.Relation) uuid.UUID { return r.ID })
	m.mineRels = indexRelations(mine.Relations)
	m.theirRels = indexRelations(theirs.Relations)

	m.fixReferences()

	st := &State{elements: m.els, relations: m.rels}
	slices.SortFunc(m.conflicts, func(a, b Conflict) int {
		if c := strings.Compare(string(a.TargetType), string(b.TargetType)); c != 0 {
			return c
		}
		if c := strings.Compare(a.TargetID.String(), b.TargetID.String()); c != 0 {
			return c
		}
		return strings.Compare(a.Field, b.Field)
	})
	return MergeOutcome{Result: st.Snapshot(), Conflicts: m.conflicts}
}

type merger struct {
	res                 map[ResolutionKey]Choice
	mineEls, theirEls   map[uuid.UUID]model.Element
	mineRels, theirRels map[uuid.UUID]model.Relation

	els  map[uuid.UUID]model.Element
	rels map[uuid.UUID]model.Relation

	conflicts     []Conflict
	seenConflicts map[ResolutionKey]bool
	inherit       map[uuid.UUID]Choice // choice carried to entities restored by a resolution
	dropped       map[uuid.UUID]bool
}

func (m *merger) conflict(c Conflict) {
	if m.seenConflicts[c.Key()] {
		return
	}
	m.seenConflicts[c.Key()] = true
	m.conflicts = append(m.conflicts, c)
}

func rawEqual(a, b json.RawMessage) bool { return bytes.Equal(a, b) }

func fieldMapsEqual(a, b map[string]json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !rawEqual(v, b[k]) {
			return false
		}
	}
	return true
}

func wholeJSON(fm map[string]json.RawMessage, present bool) json.RawMessage {
	if !present {
		return json.RawMessage("null")
	}
	raw, err := json.Marshal(fm)
	if err != nil {
		panic(err) // raw JSON values always marshal
	}
	return raw
}

// mergeSet merges one entity kind. fm extracts the comparable fields, apply
// overlays a field patch onto an entity.
func mergeSet[T any](
	m *merger, tt TargetType,
	base, mine, theirs map[uuid.UUID]T,
	fm func(T) map[string]json.RawMessage,
	apply func(T, json.RawMessage) (T, error),
	label func(T) string, idOf func(T) uuid.UUID,
) map[uuid.UUID]T {
	ids := map[uuid.UUID]bool{}
	for _, s := range []map[uuid.UUID]T{base, mine, theirs} {
		for id := range s {
			ids[id] = true
		}
	}
	sorted := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })

	out := map[uuid.UUID]T{}
	put := func(v T) { out[idOf(v)] = v }

	for _, id := range sorted {
		b, bok := base[id]
		mv, mok := mine[id]
		t, tok := theirs[id]

		switch {
		case !bok: // added on one side (or, impossibly, both: theirs wins)
			if tok {
				put(t)
			} else if mok {
				put(mv)
			}
		case !mok && !tok: // deleted on both sides
		default:
			mUnchanged := mok && fieldMapsEqual(fm(mv), fm(b))
			tUnchanged := tok && fieldMapsEqual(fm(t), fm(b))
			switch {
			case mUnchanged:
				if tok {
					put(t)
				}
			case tUnchanged:
				if mok {
					put(mv)
				}
			case mok != tok: // one side deleted, the other modified
				k := ResolutionKey{tt, id, WholeTarget}
				c, resolved := m.res[k]
				switch {
				case resolved && c == ChoiceMine:
					if mok {
						put(mv)
					}
				case resolved && c == ChoiceTheirs:
					if tok {
						put(t)
					}
				default:
					m.conflict(Conflict{
						Kind: ConflictDeleteVsUpdate, TargetType: tt, TargetID: id, Field: WholeTarget, Label: label(b),
						Message: "deleted on one side and modified on the other",
						Base:    wholeJSON(fm(b), true),
						Mine:    wholeJSON(fieldsOrNil(fm, mv, mok), mok),
						Theirs:  wholeJSON(fieldsOrNil(fm, t, tok), tok),
					})
					if tok {
						put(t) // provisional
					}
				}
			default: // both modified: field-level 3-way
				merged, err := mergeFields(m, tt, id, label(t), fm(b), fm(mv), fm(t))
				if err == nil {
					raw, _ := json.Marshal(merged)
					if res, aerr := apply(t, raw); aerr == nil {
						put(res)
						break
					}
				}
				put(t)
			}
		}
	}
	return out
}

func fieldsOrNil[T any](fm func(T) map[string]json.RawMessage, v T, present bool) map[string]json.RawMessage {
	if !present {
		return nil
	}
	return fm(v)
}

// mergeFields does the per-field 3-way merge of two modified versions.
func mergeFields(m *merger, tt TargetType, id uuid.UUID, label string, b, mv, t map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	fields := make([]string, 0, len(t))
	for f := range t {
		fields = append(fields, f)
	}
	slices.Sort(fields)

	out := make(map[string]json.RawMessage, len(fields))
	for _, f := range fields {
		bv, mf, tf := b[f], mv[f], t[f]
		switch {
		case rawEqual(mf, bv):
			out[f] = tf
		case rawEqual(tf, bv), rawEqual(mf, tf):
			out[f] = mf
		default:
			k := ResolutionKey{tt, id, f}
			switch m.res[k] {
			case ChoiceMine:
				out[f] = mf
			case ChoiceTheirs:
				out[f] = tf
			default:
				m.conflict(Conflict{
					Kind: ConflictField, TargetType: tt, TargetID: id, Field: f, Label: label,
					Message: "both sides changed " + f, Base: bv, Mine: mf, Theirs: tf,
				})
				out[f] = tf // provisional
			}
		}
	}
	return out, nil
}

// fixReferences repairs or reports entities that reference something the merge
// removed. A resolution chooses whose world wins: if the referenced entity is
// alive on the chosen side it is restored from there, otherwise the dependent
// entity is dropped (and anything depending on it, silently).
func (m *merger) fixReferences() {
	for range len(m.els) + len(m.rels) + 8 {
		if !m.fixOnce() {
			return
		}
	}
}

func (m *merger) fixOnce() bool {
	changed := false

	for _, id := range sortedIDs(m.els) {
		e, ok := m.els[id]
		if !ok || e.ParentID == nil {
			continue
		}
		if _, present := m.els[*e.ParentID]; present {
			continue
		}
		changed = m.handleDangling(TargetElement, e.ID, e.Name, *e.ParentID, "parent") || changed
	}
	for _, id := range sortedIDs(m.rels) {
		r, ok := m.rels[id]
		if !ok {
			continue
		}
		for _, ref := range []struct {
			id   uuid.UUID
			what string
		}{{r.SourceID, "source"}, {r.TargetID, "target"}} {
			if _, present := m.els[ref.id]; present {
				continue
			}
			if m.handleDangling(TargetRelation, r.ID, string(r.Kind)+" relation", ref.id, ref.what) {
				changed = true
			}
			break // the relation may be gone now; re-evaluate next pass
		}
	}
	return changed
}

func sortedIDs[T any](mp map[uuid.UUID]T) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(mp))
	for id := range mp {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return ids
}

// handleDangling returns true when it changed the result.
func (m *merger) handleDangling(tt TargetType, id uuid.UUID, label string, missing uuid.UUID, what string) bool {
	drop := func() bool {
		if tt == TargetElement {
			delete(m.els, id)
		} else {
			delete(m.rels, id)
		}
		m.dropped[id] = true
		return true
	}
	if m.dropped[missing] {
		return drop()
	}

	key := ResolutionKey{tt, id, WholeTarget}
	choice, ok := m.res[key]
	if !ok {
		choice, ok = m.inherit[id]
	}
	if !ok {
		m.conflict(Conflict{
			Kind: ConflictDanglingReference, TargetType: tt, TargetID: id, Field: WholeTarget, Label: label,
			Message: "its " + what + " (" + missing.String() + ") was deleted by the merge; choose whose version of the " + what + " wins",
			Base:    json.RawMessage("null"), Mine: json.RawMessage("null"), Theirs: json.RawMessage("null"),
		})
		return false
	}

	world := m.theirEls
	if choice == ChoiceMine {
		world = m.mineEls
	}
	if ref, alive := world[missing]; alive {
		m.els[missing] = ref
		m.inherit[missing] = choice
		return true
	}
	return drop()
}
