// Package lint is the rule engine that checks a model snapshot (a draft's
// effective model). Errors block a merge; warnings are shown in review.
package lint

import (
	"cmp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"archmap/api/internal/model"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

const (
	TargetElement  = "element"
	TargetRelation = "relation"
)

type Finding struct {
	Rule       string
	Severity   Severity
	Message    string
	TargetType string
	TargetID   uuid.UUID
}

// Rule inspects a whole snapshot and reports findings.
type Rule interface {
	ID() string
	Check(s model.Snapshot) []Finding
}

type Report struct {
	Findings []Finding
}

func (r Report) Errors() int   { return r.count(SeverityError) }
func (r Report) Warnings() int { return r.count(SeverityWarning) }

func (r Report) count(sev Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == sev {
			n++
		}
	}
	return n
}

// DefaultRules is the MVP rule set (brief section 5.2).
func DefaultRules() []Rule {
	return []Rule{
		hierarchyRule{},
		relationRule{},
		duplicateSiblingRule{},
		orphanRule{},
		usesTechnologyRule{},
		deploymentRule{},
	}
}

// Run applies rules (DefaultRules when none are given) and returns findings
// ordered errors-first, then by rule and target for stable output.
func Run(s model.Snapshot, rules ...Rule) Report {
	if len(rules) == 0 {
		rules = DefaultRules()
	}
	var all []Finding
	for _, r := range rules {
		all = append(all, r.Check(s)...)
	}
	slices.SortStableFunc(all, func(a, b Finding) int {
		if c := cmp.Compare(severityRank(a.Severity), severityRank(b.Severity)); c != 0 {
			return c
		}
		if c := strings.Compare(a.Rule, b.Rule); c != 0 {
			return c
		}
		return strings.Compare(a.TargetID.String(), b.TargetID.String())
	})
	return Report{Findings: all}
}

func severityRank(s Severity) int {
	if s == SeverityError {
		return 0
	}
	return 1
}

func elementIndex(s model.Snapshot) map[uuid.UUID]model.Element {
	idx := make(map[uuid.UUID]model.Element, len(s.Elements))
	for _, e := range s.Elements {
		idx[e.ID] = e
	}
	return idx
}

// allowedParents lists, per kind, the parent kinds that are valid. A kind with
// requiresParent must have one; others may be roots.
var allowedParents = map[model.ElementKind]struct {
	parents        []model.ElementKind
	requiresParent bool
}{
	model.KindPerson:    {nil, false},
	model.KindSystem:    {nil, false},
	model.KindContainer: {[]model.ElementKind{model.KindSystem}, true},
	model.KindComponent: {[]model.ElementKind{model.KindContainer}, true},
	model.KindNode:      {[]model.ElementKind{model.KindNode}, false},
	model.KindDatabase:  {[]model.ElementKind{model.KindSystem}, true},
}

// ERROR: parent kind hierarchy (component -> container -> system; database -> system; node nests in node).
type hierarchyRule struct{}

func (hierarchyRule) ID() string { return "hierarchy" }

func (hierarchyRule) Check(s model.Snapshot) []Finding {
	idx := elementIndex(s)
	var out []Finding
	for _, e := range s.Elements {
		rule := allowedParents[e.Kind]
		if e.ParentID == nil {
			if rule.requiresParent {
				out = append(out, Finding{"hierarchy", SeverityError,
					string(e.Kind) + " \"" + e.Name + "\" must have a parent " + string(rule.parents[0]),
					TargetElement, e.ID})
			}
			continue
		}
		parent, ok := idx[*e.ParentID]
		switch {
		case !ok:
			out = append(out, Finding{"hierarchy", SeverityError,
				"\"" + e.Name + "\" has a parent that does not exist", TargetElement, e.ID})
		case !slices.Contains(rule.parents, parent.Kind):
			out = append(out, Finding{"hierarchy", SeverityError,
				string(e.Kind) + " \"" + e.Name + "\" cannot be nested under " + string(parent.Kind) + " \"" + parent.Name + "\"",
				TargetElement, e.ID})
		}
	}
	return out
}

// ERROR: relation endpoints exist, deployed_on goes container/component -> node,
// and (source, target, kind) is unique.
type relationRule struct{}

func (relationRule) ID() string { return "relation" }

func (relationRule) Check(s model.Snapshot) []Finding {
	idx := elementIndex(s)
	type key struct {
		src, dst uuid.UUID
		kind     model.RelationKind
	}
	seen := map[key]bool{}
	rels := slices.Clone(s.Relations)
	slices.SortFunc(rels, func(a, b model.Relation) int { return strings.Compare(a.ID.String(), b.ID.String()) })

	var out []Finding
	for _, r := range rels {
		src, srcOK := idx[r.SourceID]
		dst, dstOK := idx[r.TargetID]
		if !srcOK || !dstOK {
			out = append(out, Finding{"relation", SeverityError,
				"relation points to an element that does not exist or was deleted", TargetRelation, r.ID})
			continue
		}
		k := key{r.SourceID, r.TargetID, r.Kind}
		if seen[k] {
			out = append(out, Finding{"relation", SeverityError,
				"duplicate " + string(r.Kind) + " relation from \"" + src.Name + "\" to \"" + dst.Name + "\"", TargetRelation, r.ID})
		}
		seen[k] = true
		if r.Kind == model.RelDeployedOn &&
			((src.Kind != model.KindContainer && src.Kind != model.KindComponent && src.Kind != model.KindDatabase) || dst.Kind != model.KindNode) {
			out = append(out, Finding{"relation", SeverityError,
				"deployed_on must link a container, component or database to a node (\"" + src.Name + "\" -> \"" + dst.Name + "\")",
				TargetRelation, r.ID})
		}
	}
	return out
}

// ERROR: sibling elements under the same parent must have distinct names.
type duplicateSiblingRule struct{}

func (duplicateSiblingRule) ID() string { return "duplicate-sibling-name" }

func (duplicateSiblingRule) Check(s model.Snapshot) []Finding {
	type key struct {
		parent uuid.UUID // uuid.Nil = root
		name   string
	}
	els := slices.Clone(s.Elements)
	slices.SortFunc(els, func(a, b model.Element) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	seen := map[key]bool{}
	var out []Finding
	for _, e := range els {
		var parent uuid.UUID
		if e.ParentID != nil {
			parent = *e.ParentID
		}
		k := key{parent, strings.ToLower(strings.TrimSpace(e.Name))}
		if seen[k] {
			out = append(out, Finding{"duplicate-sibling-name", SeverityError,
				"another element named \"" + e.Name + "\" already exists under the same parent", TargetElement, e.ID})
		}
		seen[k] = true
	}
	return out
}

// WARN: an element with no parent, no children and no relations (other than person / system).
type orphanRule struct{}

func (orphanRule) ID() string { return "orphan" }

func (orphanRule) Check(s model.Snapshot) []Finding {
	used := map[uuid.UUID]bool{}
	for _, e := range s.Elements {
		if e.ParentID != nil {
			used[e.ID] = true
			used[*e.ParentID] = true
		}
	}
	for _, r := range s.Relations {
		used[r.SourceID] = true
		used[r.TargetID] = true
	}
	var out []Finding
	for _, e := range s.Elements {
		if e.Kind == model.KindPerson || e.Kind == model.KindSystem || used[e.ID] {
			continue
		}
		out = append(out, Finding{"orphan", SeverityWarning,
			string(e.Kind) + " \"" + e.Name + "\" has no parent, children or relations", TargetElement, e.ID})
	}
	return out
}

// WARN: a uses relation without technology.
type usesTechnologyRule struct{}

func (usesTechnologyRule) ID() string { return "uses-technology" }

func (usesTechnologyRule) Check(s model.Snapshot) []Finding {
	var out []Finding
	for _, r := range s.Relations {
		if r.Kind == model.RelUses && strings.TrimSpace(r.Technology) == "" {
			out = append(out, Finding{"uses-technology", SeverityWarning,
				"uses relation has no technology", TargetRelation, r.ID})
		}
	}
	return out
}

// WARN: a container / component without any deployed_on relation.
type deploymentRule struct{}

func (deploymentRule) ID() string { return "deployment" }

func (deploymentRule) Check(s model.Snapshot) []Finding {
	deployed := map[uuid.UUID]bool{}
	for _, r := range s.Relations {
		if r.Kind == model.RelDeployedOn {
			deployed[r.SourceID] = true
		}
	}
	var out []Finding
	for _, e := range s.Elements {
		if (e.Kind == model.KindContainer || e.Kind == model.KindComponent || e.Kind == model.KindDatabase) && !deployed[e.ID] {
			out = append(out, Finding{"deployment", SeverityWarning,
				string(e.Kind) + " \"" + e.Name + "\" is not deployed on any node", TargetElement, e.ID})
		}
	}
	return out
}
