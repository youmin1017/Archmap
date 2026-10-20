package lint

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/model"
)

// Rules over the M10 fields: types, ownership, environment, lifecycle and reviews (ADR-066).

// ERROR: an element type must fit the element kind (database-server is a container, ...).
type typeKindRule struct{}

func (typeKindRule) ID() string { return "type-kind" }

func (typeKindRule) Check(s model.Snapshot) []Finding {
	var out []Finding
	for _, e := range s.Elements {
		if e.Type == model.TypeNone {
			continue
		}
		kinds := model.TypeKinds[e.Type]
		if !slices.Contains(kinds, e.Kind) {
			names := make([]string, len(kinds))
			for i, k := range kinds {
				names[i] = string(k)
			}
			out = append(out, Finding{"type-kind", SeverityError,
				fmt.Sprintf("%q is a %s but has type %s, which fits a %s", e.Name, e.Kind, e.Type, strings.Join(names, " or ")),
				TargetElement, e.ID})
		}
	}
	return out
}

// ERROR: ownerships must point at an existing element and owner; owner names are unique.
type ownershipRule struct{}

func (ownershipRule) ID() string { return "ownership" }

func (ownershipRule) Check(s model.Snapshot) []Finding {
	els := elementIndex(s)
	owners := map[uuid.UUID]bool{}
	byName := map[string]uuid.UUID{}
	var out []Finding
	sorted := slices.Clone(s.Owners)
	slices.SortFunc(sorted, func(a, b model.Owner) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	for _, o := range sorted {
		owners[o.ID] = true
		key := strings.ToLower(strings.TrimSpace(o.Name))
		if _, dup := byName[key]; dup {
			out = append(out, Finding{"ownership", SeverityError,
				fmt.Sprintf("two owners are named %q", o.Name), TargetOwner, o.ID})
			continue
		}
		byName[key] = o.ID
	}
	for _, o := range s.Ownerships {
		if _, ok := els[o.ElementID]; !ok {
			out = append(out, Finding{"ownership", SeverityError,
				"an ownership points at a missing element", TargetOwnership, o.ID})
		} else if !owners[o.OwnerID] {
			out = append(out, Finding{"ownership", SeverityError,
				fmt.Sprintf("the %s of %q is a missing owner", o.Role, els[o.ElementID].Name), TargetOwnership, o.ID})
		}
	}
	return out
}

// ERROR: an element in production (effective environment prod) needs an effective owner.
type ownerRequiredRule struct{}

func (ownerRequiredRule) ID() string { return "owner-required" }

func (ownerRequiredRule) Check(s model.Snapshot) []Finding {
	eff := model.ComputeEffective(s)
	var out []Finding
	for _, e := range s.Elements {
		ef := eff[e.ID]
		if ef.Environment != model.EnvProd {
			continue
		}
		if _, ok := ef.Owner(model.RoleOwner); !ok {
			out = append(out, Finding{"owner-required", SeverityError,
				fmt.Sprintf("%q runs in prod but has no owner (assign one here or on a parent)", e.Name), TargetElement, e.ID})
		}
	}
	return out
}

// WARN: reviews older than the threshold; never-reviewed only for production elements.
type staleReviewRule struct {
	now    func() time.Time
	maxAge time.Duration
}

func (staleReviewRule) ID() string { return "stale-review" }

func (r staleReviewRule) Check(s model.Snapshot) []Finding {
	eff := model.ComputeEffective(s)
	cutoff := r.now().Add(-r.maxAge)
	days := int(r.maxAge.Hours() / 24)
	var out []Finding
	for _, e := range s.Elements {
		if e.Kind == model.KindPerson {
			continue
		}
		reviewed, ok := e.ReviewedAt()
		switch {
		case ok && reviewed.Before(cutoff):
			out = append(out, Finding{"stale-review", SeverityWarning,
				fmt.Sprintf("%q was last reviewed on %s, more than %d days ago", e.Name, *e.LastReviewedAt, days), TargetElement, e.ID})
		case !ok && eff[e.ID].Environment == model.EnvProd:
			out = append(out, Finding{"stale-review", SeverityWarning,
				fmt.Sprintf("%q runs in prod and has never been reviewed", e.Name), TargetElement, e.ID})
		}
	}
	return out
}

// WARN: an active element depends on (uses, or is deployed on) a deprecated one.
type deprecatedDependencyRule struct{}

func (deprecatedDependencyRule) ID() string { return "deprecated-dependency" }

func (deprecatedDependencyRule) Check(s model.Snapshot) []Finding {
	els := elementIndex(s)
	var out []Finding
	for _, r := range s.Relations {
		src, sok := els[r.SourceID]
		dst, dok := els[r.TargetID]
		if !sok || !dok || src.Normalized().Lifecycle != model.LifecycleActive || dst.Lifecycle != model.LifecycleDeprecated {
			continue
		}
		verb := "uses"
		if r.Kind == model.RelDeployedOn {
			verb = "is deployed on"
		}
		out = append(out, Finding{"deprecated-dependency", SeverityWarning,
			fmt.Sprintf("%q %s %q, which is deprecated", src.Name, verb, dst.Name), TargetRelation, r.ID})
	}
	return out
}

// WARN: an element is assigned to a disabled user or a deleted group (ADR-069).
// Reported where the assignment is, not on every element that inherits it.
type ownerInactiveRule struct{ principals model.Principals }

func (ownerInactiveRule) ID() string { return "owner-inactive" }

func (r ownerInactiveRule) Check(s model.Snapshot) []Finding {
	if r.principals == nil {
		return nil
	}
	els := elementIndex(s)
	owners := make(map[uuid.UUID]model.Owner, len(s.Owners))
	for _, o := range s.Owners {
		owners[o.ID] = o
	}
	var out []Finding
	for _, a := range s.Ownerships {
		o, ok := owners[a.OwnerID]
		if !ok || o.PrincipalID == nil {
			continue
		}
		p, known := r.principals[*o.PrincipalID]
		if known && !p.Inactive {
			continue
		}
		what := "a disabled user"
		if o.Kind == model.OwnerGroup {
			what = "a deleted group"
		}
		out = append(out, Finding{"owner-inactive", SeverityWarning,
			fmt.Sprintf("the %s of %q is %s (%s); assign someone else", a.Role, els[a.ElementID].Name, what, o.Name),
			TargetElement, a.ElementID})
	}
	return out
}
