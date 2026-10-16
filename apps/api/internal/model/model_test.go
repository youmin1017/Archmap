package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
)

func TestLayerPatch(t *testing.T) {
	base := model.Element{ID: uuid.New(), Kind: model.KindContainer, Name: "API", Layer: "Services"}

	tests := []struct {
		name      string
		patch     string
		wantLayer string
		wantErr   bool
	}{
		{"set", `{"layer":"Kubernetes"}`, "Kubernetes", false},
		{"trimmed", `{"layer":"  Physical  "}`, "Physical", false},
		{"clear", `{"layer":""}`, "", false},
		{"untouched when absent", `{"name":"Gateway"}`, "Services", false},
		{"non-string", `{"layer":3}`, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := model.ApplyElementPatch(base, json.RawMessage(tc.patch))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Layer != tc.wantLayer {
				t.Fatalf("layer = %q, want %q", got.Layer, tc.wantLayer)
			}
			if base.Layer != "Services" {
				t.Fatal("the original element was mutated")
			}
		})
	}
}

func TestValidateLayer(t *testing.T) {
	tests := []struct {
		layer string
		ok    bool
	}{
		{"", true},
		{"Services", true},
		{"Kubernetes / EKS", true},
		{" padded", false},
		{"two\nlines", false},
		{strings.Repeat("x", model.MaxLayerLength), true},
		{strings.Repeat("x", model.MaxLayerLength+1), false},
	}
	for _, tc := range tests {
		err := model.ValidateLayer(tc.layer)
		if (err == nil) != tc.ok {
			t.Errorf("ValidateLayer(%q) = %v, ok=%v", tc.layer, err, tc.ok)
		}
		if err != nil && apperr.KindOf(err) != apperr.KindInvalid {
			t.Errorf("ValidateLayer(%q) kind = %v", tc.layer, apperr.KindOf(err))
		}
	}
	if err := (model.Element{Kind: model.KindNode, Name: "n", Layer: "bad\n"}).Validate(); err == nil {
		t.Error("Element.Validate must reject a bad layer")
	}
}

func TestLayerIsAPatchFieldAndSurvivesSnapshots(t *testing.T) {
	e := model.Element{ID: uuid.New(), Kind: model.KindNode, Name: "Host", Layer: "Physical", Tags: []string{}}
	if got := string(e.FieldMap()["layer"]); got != `"Physical"` {
		t.Fatalf("FieldMap layer = %s", got)
	}

	raw, err := model.MarshalSnapshot(model.Snapshot{Elements: []model.Element{e}})
	if err != nil {
		t.Fatal(err)
	}
	back, err := model.UnmarshalSnapshot(raw)
	if err != nil || back.Elements[0].Layer != "Physical" {
		t.Fatalf("round trip: %+v err=%v", back, err)
	}

	// Snapshots stored before layers existed have no "layer" key and read as unassigned.
	legacy := `{"elements":[{"id":"00000000-0000-0000-0000-000000000001","parent_id":null,"kind":"node","name":"Old","description":"","technology":"","tags":[],"protected":false,"version":1,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}],"relations":[]}`
	old, err := model.UnmarshalSnapshot([]byte(legacy))
	if err != nil || old.Elements[0].Layer != "" {
		t.Fatalf("legacy snapshot: %+v err=%v", old, err)
	}
}

func TestProjectsPatch(t *testing.T) {
	base := model.Element{ID: uuid.New(), Kind: model.KindContainer, Name: "API", Projects: []string{"Shop"}}

	tests := []struct {
		name  string
		patch string
		want  []string
	}{
		{"replace", `{"projects":["Payments","Shop"]}`, []string{"Payments", "Shop"}},
		{"normalized", `{"projects":[" Shop ","shop","","Payments"]}`, []string{"Shop", "Payments"}},
		{"clear", `{"projects":[]}`, []string{}},
		{"null clears", `{"projects":null}`, []string{}},
		{"untouched when absent", `{"name":"Gateway"}`, []string{"Shop"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := model.ApplyElementPatch(base, json.RawMessage(tc.patch))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got.Projects, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("projects = %v, want %v", got.Projects, tc.want)
			}
			if len(base.Projects) != 1 || base.Projects[0] != "Shop" {
				t.Fatal("the original element was mutated")
			}
		})
	}
}

func TestValidateProjects(t *testing.T) {
	tests := []struct {
		projects []string
		ok       bool
	}{
		{nil, true},
		{[]string{"Shop", "Payments"}, true},
		{[]string{"Shop", "shop"}, false},
		{[]string{" Shop"}, false},
		{[]string{""}, false},
		{[]string{strings.Repeat("x", model.MaxLayerLength+1)}, false},
		{make([]string, model.MaxProjects+1), false},
	}
	for _, tc := range tests {
		if err := model.ValidateProjects(tc.projects); (err == nil) != tc.ok {
			t.Errorf("ValidateProjects(%v) err = %v, want ok=%v", tc.projects, err, tc.ok)
		}
	}
}

func TestCheckLayer(t *testing.T) {
	rules := []model.LayerRule{
		{Class: string(model.KindDatabase), Layers: []string{"Services", "Data"}},
		{Class: "node", Layers: []string{"Physical"}},
	}
	db := model.Element{Kind: model.KindDatabase, Name: "Orders"}
	api := model.Element{Kind: model.KindContainer, Name: "API", Technology: "Go"}
	host := model.Element{Kind: model.KindNode, Name: "Host"}

	tests := []struct {
		name  string
		el    model.Element
		layer string
		ok    bool
	}{
		{"database on allowed layer", db, "data", true},
		{"database on another layer", db, "Physical", false},
		{"database without layer", db, "", true},
		{"plain container anywhere", api, "Physical", true},
		{"container with a database technology is still a container", model.Element{Kind: model.KindContainer, Name: "x", Technology: "PostgreSQL"}, "Physical", true},
		{"node on its layer", host, "Physical", true},
		{"node elsewhere", host, "Services", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.el.Layer = tc.layer
			if err := model.CheckLayer(rules, tc.el); (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
	if err := model.CheckLayer(nil, db); err != nil {
		t.Fatal("no rules must allow everything")
	}
}

func TestNormalizeLayerRules(t *testing.T) {
	got, err := model.NormalizeLayerRules([]model.LayerRule{
		{Class: "database", Layers: []string{" Services", "services", "Data", ""}},
		{Class: "node", Layers: nil},
	})
	if err != nil || len(got) != 1 || strings.Join(got[0].Layers, "|") != "Services|Data" {
		t.Fatalf("got %+v, err %v", got, err)
	}
	for _, bad := range [][]model.LayerRule{
		{{Class: "gizmo", Layers: []string{"A"}}},
		{{Class: "node", Layers: []string{"A"}}, {Class: "node", Layers: []string{"B"}}},
		{{Class: "node", Layers: []string{"a\nb"}}},
	} {
		if _, err := model.NormalizeLayerRules(bad); err == nil {
			t.Errorf("%+v should be rejected", bad)
		}
	}
}

func TestReplicasAndProperties(t *testing.T) {
	base := model.Element{ID: uuid.New(), Kind: model.KindDatabase, Name: "DB", Replicas: 2, Properties: map[string]string{"ip": "10.0.0.5", "port": "5432"}}

	got, err := model.ApplyElementPatch(base, json.RawMessage(`{"replicas":3}`))
	if err != nil || got.Replicas != 3 || got.Properties["ip"] != "10.0.0.5" {
		t.Fatalf("replicas patch: %+v, %v", got, err)
	}
	// A properties patch replaces the whole map, trims and drops empty values.
	got, err = model.ApplyElementPatch(base, json.RawMessage(`{"properties":{" ip ":" 10.0.0.9 ","port":""}}`))
	if err != nil || len(got.Properties) != 1 || got.Properties["ip"] != "10.0.0.9" {
		t.Fatalf("properties patch: %+v, %v", got.Properties, err)
	}
	if base.Properties["port"] != "5432" || base.Properties["ip"] != "10.0.0.5" {
		t.Fatal("the original element was mutated")
	}
	got, _ = model.ApplyElementPatch(base, json.RawMessage(`{"properties":null}`))
	if len(got.Properties) != 0 {
		t.Fatalf("null should clear: %v", got.Properties)
	}

	for _, bad := range []model.Element{
		{Kind: model.KindDatabase, Name: "x", Replicas: 0},
		{Kind: model.KindDatabase, Name: "x", Replicas: model.MaxReplicas + 1},
		{Kind: model.KindDatabase, Name: "x", Replicas: 1, Properties: map[string]string{"a\nb": "v"}},
		{Kind: model.KindDatabase, Name: "x", Replicas: 1, Properties: map[string]string{"IP": "1", "ip": "2"}},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("%+v should be invalid", bad)
		}
	}
}
