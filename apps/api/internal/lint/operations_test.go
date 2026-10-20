package lint_test

import (
	"testing"
	"time"

	"archmap/api/internal/lint"
	"archmap/api/internal/model"
)

func owned(element, owner byte) model.Ownership {
	return model.Ownership{ID: model.OwnershipID(id(element), model.RoleOwner), ElementID: id(element), OwnerID: id(owner), Role: model.RoleOwner}
}

func date(t time.Time) *string {
	s := t.Format(time.DateOnly)
	return &s
}

func TestEnrichmentRules(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	team := model.Owner{ID: id(100), Kind: model.OwnerTeam, Name: "Atlas"}
	with := func(e model.Element, f func(*model.Element)) model.Element { f(&e); return e }
	prod := func(e *model.Element) { e.Environment = model.EnvProd }
	reviewed := func(days int) func(*model.Element) {
		return func(e *model.Element) { e.LastReviewedAt = date(now.AddDate(0, 0, -days)) }
	}
	// sys -> server (deployed) -> logical database; host node.
	base := func() []model.Element {
		return []model.Element{
			el(1, model.KindSystem, "Shop", 0),
			with(el(2, model.KindContainer, "PG", 1), func(e *model.Element) { e.Type = model.TypeDatabaseServer }),
			with(el(3, model.KindComponent, "Orders DB", 2), func(e *model.Element) { e.Type = model.TypeDatabase }),
			el(4, model.KindNode, "Host", 0),
		}
	}
	deployed := []model.Relation{rel(10, model.RelDeployedOn, 2, 4, ""), rel(11, model.RelUses, 3, 2, "SQL")}

	tests := []struct {
		name string
		snap model.Snapshot
		// want maps rule id to the expected number of findings of that rule (others ignored).
		want map[string]int
	}{
		{
			name: "logical database inherits the server's deployment",
			snap: model.Snapshot{Elements: base(), Relations: deployed},
			want: map[string]int{"deployment": 0, "type-kind": 0},
		},
		{
			name: "nothing deployed: both container and component warn",
			snap: model.Snapshot{Elements: base()},
			want: map[string]int{"deployment": 2},
		},
		{
			name: "type that does not fit the kind",
			snap: model.Snapshot{Elements: []model.Element{
				el(1, model.KindSystem, "Shop", 0),
				with(el(2, model.KindComponent, "Srv", 1), func(e *model.Element) { e.Type = model.TypeDatabaseServer }),
				with(el(3, model.KindSystem, "Schema", 0), func(e *model.Element) { e.Type = model.TypeSchema }),
			}},
			want: map[string]int{"type-kind": 2},
		},
		{
			name: "prod without owner: every prod element errors",
			snap: model.Snapshot{Elements: func() []model.Element {
				els := base()
				prod(&els[0])
				return els
			}(), Relations: deployed},
			want: map[string]int{"owner-required": 3},
		},
		{
			name: "prod owned at the top: inherited owner satisfies everything",
			snap: model.Snapshot{Elements: func() []model.Element {
				els := base()
				prod(&els[0])
				return els
			}(), Relations: deployed, Owners: []model.Owner{team}, Ownerships: []model.Ownership{owned(1, 100)}},
			want: map[string]int{"owner-required": 0},
		},
		{
			name: "prod via a parent on a node tree; staging override is exempt",
			snap: model.Snapshot{Elements: []model.Element{
				with(el(4, model.KindNode, "Host", 0), prod),
				with(el(5, model.KindNode, "VM", 4), func(e *model.Element) { e.Environment = model.EnvStaging }),
			}},
			want: map[string]int{"owner-required": 1},
		},
		{
			name: "assignment to a deleted owner does not count and is reported",
			snap: model.Snapshot{Elements: []model.Element{with(el(1, model.KindSystem, "Shop", 0), prod)},
				Ownerships: []model.Ownership{owned(1, 100)}},
			want: map[string]int{"owner-required": 1, "ownership": 1},
		},
		{
			name: "duplicate owner names",
			snap: model.Snapshot{Owners: []model.Owner{team, {ID: id(101), Kind: model.OwnerPerson, Name: " atlas "}}},
			want: map[string]int{"ownership": 1},
		},
		{
			name: "stale reviews: old date warns; never reviewed only in prod",
			snap: model.Snapshot{Elements: []model.Element{
				with(el(1, model.KindSystem, "Old", 0), reviewed(200)),
				with(el(2, model.KindSystem, "Fresh", 0), reviewed(10)),
				el(3, model.KindSystem, "Never, not prod", 0),
				with(el(4, model.KindNode, "Never, prod", 0), prod),
				with(el(5, model.KindPerson, "People are not reviewed", 0), reviewed(900)),
			}},
			want: map[string]int{"stale-review": 2},
		},
		{
			name: "active depends on deprecated (uses and deployed_on); deprecated on deprecated is fine",
			snap: model.Snapshot{
				Elements: []model.Element{
					el(1, model.KindSystem, "S", 0),
					el(2, model.KindContainer, "API", 1),
					with(el(3, model.KindContainer, "Old DB", 1), func(e *model.Element) { e.Lifecycle = model.LifecycleDeprecated }),
					with(el(4, model.KindNode, "Old host", 0), func(e *model.Element) { e.Lifecycle = model.LifecycleDeprecated }),
				},
				Relations: []model.Relation{
					rel(10, model.RelUses, 2, 3, "SQL"),
					rel(11, model.RelDeployedOn, 2, 4, ""),
					rel(12, model.RelDeployedOn, 3, 4, ""),
				},
			},
			want: map[string]int{"deprecated-dependency": 2},
		},
	}
	rules := lint.Rules(lint.Options{Now: func() time.Time { return now }, ReviewMaxAge: 180 * 24 * time.Hour})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]int{}
			for _, f := range lint.Run(tc.snap, rules...).Findings {
				got[f.Rule]++
			}
			for rule, n := range tc.want {
				if got[rule] != n {
					t.Fatalf("%s findings = %d, want %d (all: %v)", rule, got[rule], n, lint.Run(tc.snap, rules...).Findings)
				}
			}
		})
	}
}

func TestEnrichmentSeverities(t *testing.T) {
	want := map[string]lint.Severity{
		"type-kind": lint.SeverityError, "ownership": lint.SeverityError, "owner-required": lint.SeverityError,
		"stale-review": lint.SeverityWarning, "deprecated-dependency": lint.SeverityWarning, "deployment": lint.SeverityWarning,
	}
	snap := model.Snapshot{
		Elements: []model.Element{
			func() model.Element {
				e := el(1, model.KindSystem, "S", 0)
				e.Environment = model.EnvProd
				e.Type = model.TypeQueue
				return e
			}(),
			el(2, model.KindContainer, "C", 1),
			func() model.Element {
				e := el(3, model.KindContainer, "D", 1)
				e.Lifecycle = model.LifecycleDeprecated
				return e
			}(),
		},
		Relations:  []model.Relation{rel(10, model.RelUses, 2, 3, "x")},
		Ownerships: []model.Ownership{owned(1, 99)},
	}
	for _, f := range lint.Run(snap).Findings {
		if sev, ok := want[f.Rule]; ok && f.Severity != sev {
			t.Errorf("%s has severity %s, want %s", f.Rule, f.Severity, sev)
		}
	}
}

func TestOwnerInactive(t *testing.T) {
	user, group := id(201), id(202)
	owners := []model.Owner{
		{ID: model.PrincipalOwnerID(user), Kind: model.OwnerUser, Name: "Ada", PrincipalID: &user},
		{ID: model.PrincipalOwnerID(group), Kind: model.OwnerGroup, Name: "Payments", PrincipalID: &group},
		{ID: id(100), Kind: model.OwnerTeam, Name: "Atlas"},
	}
	assign := func(element byte, owner model.Owner, role model.OwnerRole) model.Ownership {
		return model.Ownership{ID: model.OwnershipID(id(element), role), ElementID: id(element), OwnerID: owner.ID, Role: role}
	}
	snap := model.Snapshot{
		Elements: []model.Element{el(1, model.KindSystem, "Shop", 0), el(2, model.KindContainer, "API", 1), el(3, model.KindContainer, "DB", 1)},
		Owners:   owners,
		Ownerships: []model.Ownership{
			assign(1, owners[0], model.RoleOwner),  // disabled user on Shop: one finding (API inherits; not repeated)
			assign(3, owners[1], model.RoleOncall), // deleted group on DB
			assign(2, owners[2], model.RoleOwner),  // named team: never inactive
		},
	}
	count := func(p model.Principals) int {
		n := 0
		for _, f := range lint.Run(snap, lint.Rules(lint.Options{Principals: p})...).Findings {
			if f.Rule == "owner-inactive" {
				if f.Severity != lint.SeverityWarning {
					t.Fatalf("owner-inactive must be a warning: %+v", f)
				}
				n++
			}
		}
		return n
	}
	active := model.Principals{
		user:  {ID: user, Kind: model.OwnerUser, Name: "Ada"},
		group: {ID: group, Kind: model.OwnerGroup, Name: "Payments"},
	}
	if n := count(active); n != 0 {
		t.Fatalf("active principals: %d findings", n)
	}
	inactive := model.Principals{
		user:  {ID: user, Kind: model.OwnerUser, Name: "Ada", Inactive: true},
		group: {ID: group, Kind: model.OwnerGroup, Name: "Payments", Inactive: true},
	}
	if n := count(inactive); n != 2 {
		t.Fatalf("inactive principals: %d findings, want 2", n)
	}
	if n := count(nil); n != 0 {
		t.Fatalf("without a directory the rule is skipped, got %d", n)
	}
}
