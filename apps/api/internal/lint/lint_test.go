package lint_test

import (
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/lint"
	"archmap/api/internal/model"
)

func id(n byte) uuid.UUID { return uuid.UUID{15: n} }

func el(n byte, kind model.ElementKind, name string, parent byte) model.Element {
	e := model.Element{ID: id(n), Kind: kind, Name: name}
	if parent != 0 {
		p := id(parent)
		e.ParentID = &p
	}
	return e
}

func rel(n byte, kind model.RelationKind, src, dst byte, tech string) model.Relation {
	return model.Relation{ID: id(n), Kind: kind, SourceID: id(src), TargetID: id(dst), Technology: tech}
}

func TestRules(t *testing.T) {
	tests := []struct {
		name string
		snap model.Snapshot
		// want maps "rule/severity" to the expected number of findings.
		want map[string]int
	}{
		{
			name: "clean model",
			snap: model.Snapshot{
				Elements: []model.Element{
					el(1, model.KindSystem, "Shop", 0),
					el(2, model.KindContainer, "API", 1),
					el(3, model.KindNode, "Host", 0),
					el(4, model.KindNode, "Docker", 3),
				},
				Relations: []model.Relation{rel(10, model.RelDeployedOn, 2, 4, "")},
			},
			want: map[string]int{},
		},
		{
			name: "container without parent and component under system",
			snap: model.Snapshot{Elements: []model.Element{
				el(1, model.KindSystem, "S", 0),
				el(2, model.KindContainer, "Lonely", 0),
				el(3, model.KindComponent, "Bad", 1),
			}},
			want: map[string]int{
				"hierarchy/error":    2,
				"deployment/warning": 2,
				"orphan/warning":     1, // Lonely has no relations; Bad has a parent
			},
		},
		{
			name: "person with parent and node under system",
			snap: model.Snapshot{Elements: []model.Element{
				el(1, model.KindSystem, "S", 0),
				el(2, model.KindPerson, "P", 1),
				el(3, model.KindNode, "N", 1),
			}},
			want: map[string]int{"hierarchy/error": 2},
		},
		{
			name: "missing parent element",
			snap: model.Snapshot{Elements: []model.Element{el(2, model.KindContainer, "C", 9)}},
			want: map[string]int{"hierarchy/error": 1, "deployment/warning": 1},
		},
		{
			name: "duplicate sibling names are case-insensitive, other parents are fine",
			snap: model.Snapshot{Elements: []model.Element{
				el(1, model.KindSystem, "S1", 0),
				el(2, model.KindSystem, "S2", 0),
				el(3, model.KindContainer, "API", 1),
				el(4, model.KindContainer, "api ", 1),
				el(5, model.KindContainer, "API", 2),
			}},
			want: map[string]int{"duplicate-sibling-name/error": 1, "deployment/warning": 3},
		},
		{
			name: "relation to missing element and duplicate relation",
			snap: model.Snapshot{
				Elements: []model.Element{
					el(1, model.KindSystem, "A", 0),
					el(2, model.KindSystem, "B", 0),
				},
				Relations: []model.Relation{
					rel(10, model.RelUses, 1, 2, "HTTP"),
					rel(11, model.RelUses, 1, 2, "HTTP"),
					rel(12, model.RelUses, 1, 9, "HTTP"),
				},
			},
			want: map[string]int{"relation/error": 2},
		},
		{
			name: "deployed_on must go container to node",
			snap: model.Snapshot{
				Elements: []model.Element{
					el(1, model.KindSystem, "S", 0),
					el(2, model.KindContainer, "C", 1),
					el(3, model.KindNode, "N", 0),
				},
				Relations: []model.Relation{
					rel(10, model.RelDeployedOn, 2, 1, ""), // target is a system
					rel(11, model.RelDeployedOn, 1, 3, ""), // source is a system
					rel(12, model.RelDeployedOn, 2, 3, ""), // fine
				},
			},
			want: map[string]int{"relation/error": 2},
		},
		{
			name: "orphan warning skips person, system and elements in use",
			snap: model.Snapshot{Elements: []model.Element{
				el(1, model.KindPerson, "P", 0),
				el(2, model.KindSystem, "S", 0),
				el(3, model.KindNode, "Alone", 0),
				el(4, model.KindNode, "Parent", 0),
				el(5, model.KindNode, "Child", 4),
			}},
			want: map[string]int{"orphan/warning": 1},
		},
		{
			name: "uses relation without technology",
			snap: model.Snapshot{
				Elements: []model.Element{el(1, model.KindSystem, "A", 0), el(2, model.KindSystem, "B", 0)},
				Relations: []model.Relation{
					rel(10, model.RelUses, 1, 2, " "),
					rel(11, model.RelUses, 2, 1, "gRPC"),
				},
			},
			want: map[string]int{"uses-technology/warning": 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]int{}
			report := lint.Run(tc.snap)
			for _, f := range report.Findings {
				got[f.Rule+"/"+string(f.Severity)]++
			}
			if len(got) != len(tc.want) {
				t.Fatalf("findings = %v, want %v", got, tc.want)
			}
			for k, n := range tc.want {
				if got[k] != n {
					t.Fatalf("findings = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestReportOrderAndCounts(t *testing.T) {
	snap := model.Snapshot{
		Elements: []model.Element{
			el(1, model.KindSystem, "S", 0),
			el(2, model.KindContainer, "C", 0),
		},
		Relations: []model.Relation{rel(10, model.RelUses, 1, 2, "")},
	}
	r := lint.Run(snap)
	if r.Errors() != 1 || r.Warnings() != 2 {
		t.Fatalf("errors=%d warnings=%d: %+v", r.Errors(), r.Warnings(), r.Findings)
	}
	if r.Findings[0].Severity != lint.SeverityError {
		t.Fatalf("errors must sort first: %+v", r.Findings)
	}
}
