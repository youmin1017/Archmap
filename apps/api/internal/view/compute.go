package view

import (
	"slices"
	"strings"

	"github.com/google/uuid"

	"archmap/api/internal/model"
)

// Result is the part of a model a view shows. Elements holds every root and
// all their descendants (so the canvas can expand them and roll relations up);
// a root's parent is not in Elements, which makes it a top-level node there.
type Result struct {
	Elements  []model.Element
	Relations []model.Relation
	Roots     []uuid.UUID
	// Expanded lists elements that start expanded (the scope of scoped views, the node tree of deployment views).
	Expanded []uuid.UUID
	// ScopeMissing: the view's scope element does not exist in the model being shown.
	ScopeMissing bool
}

type index struct {
	byID     map[uuid.UUID]model.Element
	children map[uuid.UUID][]uuid.UUID
}

func newIndex(s model.Snapshot) index {
	ix := index{byID: make(map[uuid.UUID]model.Element, len(s.Elements)), children: map[uuid.UUID][]uuid.UUID{}}
	for _, e := range s.Elements {
		ix.byID[e.ID] = e
	}
	for _, e := range s.Elements {
		if e.ParentID != nil {
			if _, ok := ix.byID[*e.ParentID]; ok {
				ix.children[*e.ParentID] = append(ix.children[*e.ParentID], e.ID)
			}
		}
	}
	return ix
}

// parent returns the id of an existing parent.
func (ix index) parent(id uuid.UUID) (uuid.UUID, bool) {
	e, ok := ix.byID[id]
	if !ok || e.ParentID == nil {
		return uuid.Nil, false
	}
	if _, ok := ix.byID[*e.ParentID]; !ok {
		return uuid.Nil, false
	}
	return *e.ParentID, true
}

// ancestors of id (nearest first), guarded against cycles.
func (ix index) ancestors(id uuid.UUID) []uuid.UUID {
	var out []uuid.UUID
	seen := map[uuid.UUID]bool{id: true}
	for cur := id; ; {
		p, ok := ix.parent(cur)
		if !ok || seen[p] {
			return out
		}
		out = append(out, p)
		seen[p] = true
		cur = p
	}
}

// subtree returns id and all its descendants.
func (ix index) subtree(id uuid.UUID) []uuid.UUID {
	out := []uuid.UUID{id}
	seen := map[uuid.UUID]bool{id: true}
	for i := 0; i < len(out); i++ {
		for _, c := range ix.children[out[i]] {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

func (ix index) topLevel(id uuid.UUID) uuid.UUID {
	anc := ix.ancestors(id)
	if len(anc) == 0 {
		return id
	}
	return anc[len(anc)-1]
}

// Compute decides what a view shows in the given model snapshot. members are
// the saved members of a custom view; they are ignored by the other kinds.
func Compute(v View, members []uuid.UUID, snap model.Snapshot) Result {
	ix := newIndex(snap)

	var scope uuid.UUID
	if v.ScopeElementID != nil {
		scope = *v.ScopeElementID
		if _, ok := ix.byID[scope]; !ok && v.Kind.ScopeRule() != ScopeForbidden {
			return Result{ScopeMissing: true}
		}
	}

	var roots []uuid.UUID
	var expandScope bool

	switch v.Kind {
	case KindLandscape:
		for _, e := range snap.Elements {
			if _, hasParent := ix.parent(e.ID); !hasParent && (e.Kind == model.KindPerson || e.Kind == model.KindSystem) {
				roots = append(roots, e.ID)
			}
		}
	case KindContext:
		roots = append([]uuid.UUID{scope}, neighbours(ix, snap, scope, func(other uuid.UUID) uuid.UUID { return ix.topLevel(other) },
			func(e model.Element) bool { return e.Kind == model.KindPerson || e.Kind == model.KindSystem })...)
	case KindContainer, KindComponent:
		scopeAnc := ix.ancestors(scope)
		roots = append([]uuid.UUID{scope}, neighbours(ix, snap, scope, func(other uuid.UUID) uuid.UUID {
			// Highest ancestor of the other end that does not contain the scope.
			best := other
			for _, a := range ix.ancestors(other) {
				if slices.Contains(scopeAnc, a) {
					break
				}
				best = a
			}
			return best
		}, func(model.Element) bool { return true })...)
		expandScope = true
	case KindDeployment:
		roots = deploymentRoots(ix, snap, scope, v.ScopeElementID != nil)
	case KindLayers:
		// Every element is placed by its own layer; nesting is flattened, so everything is a root.
		for _, e := range snap.Elements {
			roots = append(roots, e.ID)
		}
	case KindCustom:
		for _, m := range members {
			if _, ok := ix.byID[m]; ok {
				roots = append(roots, m)
			}
		}
	}

	roots = dedupe(roots)
	// Roots that sit inside another root are not roots of the view.
	inSubtree := map[uuid.UUID]bool{}
	for _, r := range roots {
		for _, d := range ix.subtree(r)[1:] {
			inSubtree[d] = true
		}
	}
	roots = slices.DeleteFunc(roots, func(r uuid.UUID) bool { return inSubtree[r] })

	shown := map[uuid.UUID]bool{}
	for _, r := range roots {
		for _, id := range ix.subtree(r) {
			shown[id] = true
		}
	}

	res := Result{Roots: roots}
	for _, e := range snap.Elements {
		if shown[e.ID] {
			res.Elements = append(res.Elements, e)
		}
	}
	slices.SortFunc(res.Elements, func(a, b model.Element) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	for _, r := range snap.Relations {
		if shown[r.SourceID] && shown[r.TargetID] {
			res.Relations = append(res.Relations, r)
		}
	}

	switch {
	case expandScope:
		res.Expanded = []uuid.UUID{scope}
	case v.Kind == KindDeployment:
		for _, r := range roots {
			for _, id := range ix.subtree(r) {
				if len(ix.children[id]) > 0 && ix.byID[id].Kind == model.KindNode {
					res.Expanded = append(res.Expanded, id)
				}
			}
		}
	}
	return res
}

// neighbours returns the elements to show next to scope: for every `uses`
// relation crossing the scope's subtree boundary, the other end mapped through
// pick, filtered by keep.
func neighbours(ix index, snap model.Snapshot, scope uuid.UUID, pick func(other uuid.UUID) uuid.UUID, keep func(model.Element) bool) []uuid.UUID {
	inside := map[uuid.UUID]bool{}
	for _, id := range ix.subtree(scope) {
		inside[id] = true
	}
	scopeAnc := ix.ancestors(scope)

	var out []uuid.UUID
	for _, r := range snap.Relations {
		if r.Kind != model.RelUses {
			continue
		}
		var other uuid.UUID
		switch {
		case inside[r.SourceID] && !inside[r.TargetID]:
			other = r.TargetID
		case inside[r.TargetID] && !inside[r.SourceID]:
			other = r.SourceID
		default:
			continue
		}
		if _, ok := ix.byID[other]; !ok || slices.Contains(scopeAnc, other) {
			continue
		}
		n := pick(other)
		if ne, ok := ix.byID[n]; ok && !inside[n] && keep(ne) {
			out = append(out, n)
		}
	}
	return out
}

// deploymentRoots: the node tree(s) plus the software deployed on them.
func deploymentRoots(ix index, snap model.Snapshot, scope uuid.UUID, scoped bool) []uuid.UUID {
	var nodes []uuid.UUID
	if scoped {
		nodes = []uuid.UUID{scope}
	} else {
		for _, e := range snap.Elements {
			if _, hasParent := ix.parent(e.ID); !hasParent && e.Kind == model.KindNode {
				nodes = append(nodes, e.ID)
			}
		}
	}
	onNodes := map[uuid.UUID]bool{}
	for _, n := range nodes {
		for _, id := range ix.subtree(n) {
			onNodes[id] = true
		}
	}
	roots := slices.Clone(nodes)
	for _, r := range snap.Relations {
		if r.Kind == model.RelDeployedOn && onNodes[r.TargetID] && !onNodes[r.SourceID] {
			if _, ok := ix.byID[r.SourceID]; ok {
				roots = append(roots, r.SourceID)
			}
		}
	}
	return roots
}

func dedupe(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := ids[:0:0]
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
