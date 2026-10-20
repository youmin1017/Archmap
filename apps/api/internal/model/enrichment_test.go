package model_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/model"
)

func uid(n byte) uuid.UUID { return uuid.UUID{15: n} }

func elem(n byte, kind model.ElementKind, parent byte) model.Element {
	e := model.Element{ID: uid(n), Kind: kind, Name: string(rune('A' + n)), Tags: []string{}, Replicas: 1, Version: 1}
	if parent != 0 {
		p := uid(parent)
		e.ParentID = &p
	}
	return e
}

func owns(element, owner byte, role model.OwnerRole) model.Ownership {
	return model.Ownership{ID: model.OwnershipID(uid(element), role), ElementID: uid(element), OwnerID: uid(owner), Role: role}
}

// tree: 1 system (prod) -> 2 container (database-server, deployed) -> 3 component (logical database)
//
//	-> 4 container (staging, not deployed)
func enrichmentSnapshot() model.Snapshot {
	sys := elem(1, model.KindSystem, 0)
	sys.Environment = model.EnvProd
	server := elem(2, model.KindContainer, 1)
	db := elem(3, model.KindComponent, 2)
	other := elem(4, model.KindContainer, 1)
	other.Environment = model.EnvStaging
	node := elem(9, model.KindNode, 0)
	return model.Snapshot{
		Elements:  []model.Element{sys, server, db, other, node},
		Relations: []model.Relation{{ID: uid(50), SourceID: uid(2), TargetID: uid(9), Kind: model.RelDeployedOn}},
		Owners: []model.Owner{
			{ID: uid(100), Kind: model.OwnerTeam, Name: "Team Atlas"},
			{ID: uid(101), Kind: model.OwnerTeam, Name: "Team Borealis"},
		},
		Ownerships: []model.Ownership{
			owns(1, 100, model.RoleOwner),
			owns(1, 100, model.RoleOncall),
			owns(3, 101, model.RoleOwner), // override on the logical database
			owns(4, 102, model.RoleOwner), // owner 102 does not exist: ignored
		},
	}
}

func TestComputeEffective(t *testing.T) {
	eff := model.ComputeEffective(enrichmentSnapshot())
	tests := []struct {
		name       string
		element    byte
		env        model.Environment
		envSource  byte // 0 = none
		owner      byte // 0 = none
		ownerFrom  byte
		oncall     byte
		deployedBy byte // 0 = not deployed
	}{
		{"own values", 1, model.EnvProd, 1, 100, 1, 100, 0},
		{"inherits owner, on-call and environment; deployed itself", 2, model.EnvProd, 1, 100, 1, 100, 2},
		{"override wins for one role, the others still inherit", 3, model.EnvProd, 1, 101, 3, 100, 2},
		{"own environment overrides; missing owner falls back to the parent", 4, model.EnvStaging, 4, 100, 1, 100, 0},
		{"unrelated root has nothing", 9, model.EnvUnset, 0, 0, 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := eff[uid(tc.element)]
			if e.Environment != tc.env {
				t.Errorf("environment = %q, want %q", e.Environment, tc.env)
			}
			if (tc.envSource == 0) != (e.EnvironmentSource == nil) || (e.EnvironmentSource != nil && *e.EnvironmentSource != uid(tc.envSource)) {
				t.Errorf("environment source = %v, want %d", e.EnvironmentSource, tc.envSource)
			}
			o, ok := e.Owner(model.RoleOwner)
			if (tc.owner == 0) == ok || (ok && (o.OwnerID != uid(tc.owner) || o.Source != uid(tc.ownerFrom))) {
				t.Errorf("owner = %+v (%v), want %d from %d", o, ok, tc.owner, tc.ownerFrom)
			}
			oc, ok := e.Owner(model.RoleOncall)
			if (tc.oncall == 0) == ok || (ok && oc.OwnerID != uid(tc.oncall)) {
				t.Errorf("oncall = %+v (%v), want %d", oc, ok, tc.oncall)
			}
			if (tc.deployedBy == 0) != (e.DeployedVia == nil) || (e.DeployedVia != nil && *e.DeployedVia != uid(tc.deployedBy)) {
				t.Errorf("deployed via = %v, want %d", e.DeployedVia, tc.deployedBy)
			}
		})
	}
}

func TestComputeEffectiveSurvivesParentCycles(t *testing.T) {
	a, b := elem(1, model.KindContainer, 2), elem(2, model.KindContainer, 1)
	a.Environment = model.EnvDev
	eff := model.ComputeEffective(model.Snapshot{Elements: []model.Element{a, b}})
	if len(eff) != 2 || eff[uid(1)].Environment != model.EnvDev {
		t.Fatalf("eff = %+v", eff)
	}
}

func TestRetiredDatabaseKindIsNormalized(t *testing.T) {
	// A patch from an old client / an open draft.
	e, err := model.ApplyElementPatch(model.Element{ID: uid(1)}, json.RawMessage(`{"kind":"database","name":"DB"}`))
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != model.KindContainer || e.Type != model.TypeDatabase || e.Lifecycle != model.LifecycleActive {
		t.Fatalf("element = %+v", e)
	}
	if model.KindDatabase.Valid() {
		t.Fatal("the retired kind must not be valid in a model")
	}
	// A snapshot written before M10: kind database, no new fields.
	old := `{"elements":[{"id":"00000000-0000-0000-0000-000000000001","parent_id":null,"kind":"database","name":"DB",
		"description":"","technology":"","tags":[],"protected":false,"version":1,
		"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}],"relations":[]}`
	s, err := model.UnmarshalSnapshot([]byte(old))
	if err != nil {
		t.Fatal(err)
	}
	got := s.Elements[0]
	if got.Kind != model.KindContainer || got.Type != model.TypeDatabase || got.Lifecycle != model.LifecycleActive ||
		len(s.Owners) != 0 || len(s.Ownerships) != 0 {
		t.Fatalf("old snapshot read as %+v / %+v", got, s)
	}
	// Normalized values compare equal to the migrated row, so version diffs stay empty.
	migrated := model.Element{ID: uid(1), Kind: model.KindContainer, Type: model.TypeDatabase, Name: "DB", Version: 1}
	if string(mustJSON(t, got.FieldMap())) != string(mustJSON(t, migrated.FieldMap())) {
		t.Fatalf("field maps differ:\n%s\n%s", mustJSON(t, got.FieldMap()), mustJSON(t, migrated.FieldMap()))
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSnapshotRoundTripKeepsEnrichment(t *testing.T) {
	in := enrichmentSnapshot()
	reviewed := "2026-09-01"
	in.Elements[1].Type = model.TypeDatabaseServer
	in.Elements[1].Criticality = model.CriticalityCritical
	in.Elements[1].RunbookURL = "https://runbooks.example/db"
	in.Elements[1].LastReviewedAt = &reviewed
	raw, err := model.MarshalSnapshot(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := model.UnmarshalSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	e := out.Elements[1]
	if e.Type != model.TypeDatabaseServer || e.Criticality != model.CriticalityCritical || e.LastReviewedAt == nil ||
		*e.LastReviewedAt != reviewed || e.RunbookURL == "" || len(out.Owners) != 2 || len(out.Ownerships) != 4 {
		t.Fatalf("round trip lost data: %+v, %d owners, %d ownerships", e, len(out.Owners), len(out.Ownerships))
	}
}

func TestOperationsValidation(t *testing.T) {
	base := model.Element{ID: uid(1), Kind: model.KindContainer, Name: "X", Replicas: 1}
	tests := []struct {
		name  string
		patch string
		ok    bool
	}{
		{"all fields", `{"type":"database-server","environment":"prod","lifecycle":"deprecated","criticality":"high",
			"runbook_url":"https://runbooks.example/x","docs_url":"http://docs.example","last_reviewed_at":"2026-10-01"}`, true},
		{"clear review date", `{"last_reviewed_at":null}`, true},
		{"unknown type", `{"type":"mainframe"}`, false},
		{"unknown environment", `{"environment":"qa"}`, false},
		{"empty lifecycle means active", `{"lifecycle":""}`, true},
		{"unknown lifecycle", `{"lifecycle":"retired"}`, false},
		{"unknown criticality", `{"criticality":"extreme"}`, false},
		{"non-http url", `{"runbook_url":"ftp://x.example"}`, false},
		{"relative url", `{"docs_url":"/docs"}`, false},
		{"bad date", `{"last_reviewed_at":"01/10/2026"}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, err := model.ApplyElementPatch(base, json.RawMessage(tc.patch))
			if err == nil {
				err = e.Validate()
			}
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestOwnerAndOwnershipValidation(t *testing.T) {
	good := model.Owner{ID: uid(1), Kind: model.OwnerTeam, Name: "Team Atlas", OncallURL: "https://oncall.example/atlas"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, o := range map[string]model.Owner{
		"kind":   {Kind: "group", Name: "x"},
		"name":   {Kind: model.OwnerTeam, Name: "  "},
		"oncall": {Kind: model.OwnerPerson, Name: "x", OncallURL: "not a url"},
	} {
		if o.Validate() == nil {
			t.Errorf("%s: invalid owner accepted", name)
		}
	}
	ok := owns(1, 2, model.RoleMaintainer)
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	wrongID := ok
	wrongID.ID = uid(7)
	badRole := ok
	badRole.Role = "boss"
	for name, o := range map[string]model.Ownership{"derived id": wrongID, "role": badRole} {
		if o.Validate() == nil {
			t.Errorf("%s: invalid ownership accepted", name)
		}
	}
	if model.OwnershipID(uid(1), model.RoleOwner) == model.OwnershipID(uid(1), model.RoleOncall) ||
		model.OwnershipID(uid(1), model.RoleOwner) != model.OwnershipID(uid(1), model.RoleOwner) {
		t.Fatal("ownership ids must be per (element, role) and deterministic")
	}
}

func TestLayerRuleMatchesKindOrType(t *testing.T) {
	rules := []model.LayerRule{{Class: "database", Layers: []string{"Data"}}}
	db := model.Element{Kind: model.KindComponent, Type: model.TypeDatabase, Layer: "Physical"}
	server := model.Element{Kind: model.KindContainer, Type: model.TypeDatabaseServer, Layer: "Physical"}
	if model.CheckLayer(rules, db) == nil {
		t.Fatal("type database must be matched by the class database")
	}
	if model.CheckLayer(rules, server) != nil {
		t.Fatal("database-server is a different class")
	}
	if _, err := model.NormalizeLayerRules([]model.LayerRule{{Class: "cache", Layers: []string{"Data"}}}); err != nil {
		t.Fatalf("a type is a valid class: %v", err)
	}
}

func TestPrincipalOwners(t *testing.T) {
	user := uid(42)
	id := model.PrincipalOwnerID(user)
	tests := []struct {
		name  string
		owner model.Owner
		ok    bool
	}{
		{"user owner", model.Owner{ID: id, Kind: model.OwnerUser, Name: "Ada", PrincipalID: &user}, true},
		{"group owner", model.Owner{ID: id, Kind: model.OwnerGroup, Name: "Payments", PrincipalID: &user}, true},
		{"user owner without principal", model.Owner{ID: id, Kind: model.OwnerUser, Name: "Ada"}, false},
		{"named team with a principal", model.Owner{ID: id, Kind: model.OwnerTeam, Name: "T", PrincipalID: &user}, false},
		{"id not derived", model.Owner{ID: uid(7), Kind: model.OwnerUser, Name: "Ada", PrincipalID: &user}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.owner.Validate(); (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
	if model.PrincipalOwnerID(user) != id || model.PrincipalOwnerID(uid(43)) == id {
		t.Fatal("principal owner ids must be deterministic and distinct")
	}
	// Survives snapshots; older snapshots read as nil.
	raw, err := model.MarshalSnapshot(model.Snapshot{Owners: []model.Owner{{ID: id, Kind: model.OwnerUser, Name: "Ada", PrincipalID: &user}}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := model.UnmarshalSnapshot(raw)
	if err != nil || s.Owners[0].PrincipalID == nil || *s.Owners[0].PrincipalID != user {
		t.Fatalf("round trip = %+v, %v", s.Owners, err)
	}
}
