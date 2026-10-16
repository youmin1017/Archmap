// Package model holds the architecture model domain: elements and relations,
// the current "main" state and pure helpers shared by the changeset package.
// Model (structure) is strictly separate from layout (positions, see views).
package model

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/platform/apperr"
)

type ElementKind string

const (
	KindPerson    ElementKind = "person"
	KindSystem    ElementKind = "system"
	KindContainer ElementKind = "container"
	KindComponent ElementKind = "component"
	KindNode      ElementKind = "node"
	KindDatabase  ElementKind = "database"
)

func (k ElementKind) Valid() bool {
	switch k {
	case KindPerson, KindSystem, KindContainer, KindComponent, KindNode, KindDatabase:
		return true
	}
	return false
}

type RelationKind string

const (
	RelUses       RelationKind = "uses"
	RelDeployedOn RelationKind = "deployed_on"
)

func (k RelationKind) Valid() bool { return k == RelUses || k == RelDeployedOn }

// Element is a node of the model tree. The json tags double as the field names
// of change patches (see ElementPatchFields).
type Element struct {
	ID          uuid.UUID   `json:"id"`
	ParentID    *uuid.UUID  `json:"parent_id"`
	Kind        ElementKind `json:"kind"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Technology  string      `json:"technology"`
	Tags        []string    `json:"tags"`
	Protected   bool        `json:"protected"`
	// Layer places the element in a layered (3D) view, e.g. "Services" or "Physical"; "" = unassigned.
	Layer string `json:"layer"`
	// Projects lists the projects the element belongs to (names; may be several).
	Projects []string `json:"projects"`
	// Replicas is how many instances the element stands for (1 = one).
	Replicas int `json:"replicas"`
	// Properties are free-form facts (ip, port, namespace, ...).
	Properties map[string]string `json:"properties"`
	Version    int               `json:"version"`
	DeletedAt  *time.Time        `json:"-"`
	CreatedAt  time.Time         `json:"-"`
	UpdatedAt  time.Time         `json:"-"`
}

type Relation struct {
	ID          uuid.UUID    `json:"id"`
	SourceID    uuid.UUID    `json:"source_id"`
	TargetID    uuid.UUID    `json:"target_id"`
	Kind        RelationKind `json:"kind"`
	Description string       `json:"description"`
	Technology  string       `json:"technology"`
	Version     int          `json:"version"`
	DeletedAt   *time.Time   `json:"-"`
	CreatedAt   time.Time    `json:"-"`
	UpdatedAt   time.Time    `json:"-"`
}

// Snapshot is a full, non-deleted model state (main, or a draft's effective model).
type Snapshot struct {
	Elements  []Element
	Relations []Relation
}

// Fields a change patch may set. id, version and timestamps are server-managed.
var (
	ElementPatchFields  = []string{"parent_id", "kind", "name", "description", "technology", "tags", "protected", "layer", "projects", "replicas", "properties"}
	RelationPatchFields = []string{"source_id", "target_id", "kind", "description", "technology"}
)

// Validate checks the structural invariants of an element in isolation.
func (e Element) Validate() error {
	if !e.Kind.Valid() {
		return apperr.Invalid("invalid element kind %q", e.Kind)
	}
	if strings.TrimSpace(e.Name) == "" {
		return apperr.Invalid("element name must not be empty")
	}
	if err := ValidateLayer(e.Layer); err != nil {
		return err
	}
	if err := ValidateProjects(e.Projects); err != nil {
		return err
	}
	if e.Replicas < 1 || e.Replicas > MaxReplicas {
		return apperr.Invalid("replicas must be between 1 and %d", MaxReplicas)
	}
	if err := ValidateProperties(e.Properties); err != nil {
		return err
	}
	return nil
}

func (r Relation) Validate() error {
	if !r.Kind.Valid() {
		return apperr.Invalid("invalid relation kind %q", r.Kind)
	}
	if r.SourceID == r.TargetID {
		return apperr.Invalid("relation source and target must differ")
	}
	return nil
}

// ApplyElementPatch overlays the fields present in patch onto e. Absent fields
// are untouched; an explicit null clears a nullable field (parent_id).
func ApplyElementPatch(e Element, patch json.RawMessage) (Element, error) {
	if err := checkKeys(patch, ElementPatchFields); err != nil {
		return Element{}, err
	}
	// json.Unmarshal writes through existing pointers / slice backing arrays;
	// detach them so the caller's original value is never mutated.
	if e.ParentID != nil {
		p := *e.ParentID
		e.ParentID = &p
	}
	e.Tags = slices.Clone(e.Tags)
	e.Projects = slices.Clone(e.Projects)
	// A patch replaces the whole properties map (unmarshal would merge into the old one).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(patch, &raw); err == nil {
		if _, ok := raw["properties"]; ok {
			e.Properties = nil
		}
	}
	if e.Properties != nil {
		e.Properties = maps.Clone(e.Properties)
	}
	if err := json.Unmarshal(patch, &e); err != nil {
		return Element{}, apperr.Invalid("invalid element patch: %v", err)
	}
	if e.Tags == nil {
		e.Tags = []string{}
	}
	e.Layer = strings.TrimSpace(e.Layer)
	e.Projects = NormalizeProjects(e.Projects)
	e.Properties = NormalizeProperties(e.Properties)
	if _, set := raw["replicas"]; !set && e.Replicas == 0 {
		e.Replicas = 1 // elements stored before replicas existed
	}
	return e, nil
}

func ApplyRelationPatch(r Relation, patch json.RawMessage) (Relation, error) {
	if err := checkKeys(patch, RelationPatchFields); err != nil {
		return Relation{}, err
	}
	if err := json.Unmarshal(patch, &r); err != nil {
		return Relation{}, apperr.Invalid("invalid relation patch: %v", err)
	}
	return r, nil
}

// PatchKeys returns the sorted top-level keys of a JSON object patch.
func PatchKeys(patch json.RawMessage) ([]string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(patch, &m); err != nil {
		return nil, apperr.Invalid("patch must be a JSON object")
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys, nil
}

func checkKeys(patch json.RawMessage, allowed []string) error {
	keys, err := PatchKeys(patch)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if !slices.Contains(allowed, k) {
			return apperr.Invalid("field %q cannot be set by a change", k)
		}
	}
	return nil
}

// FieldsOf marshals v and returns the subset of its JSON fields named by keys.
// It is used to capture the "before" values of a patch.
func FieldsOf(v any, keys []string) (json.RawMessage, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	out := make(map[string]json.RawMessage, len(keys))
	for _, k := range keys {
		if v, ok := all[k]; ok {
			out[k] = v
		}
	}
	return json.Marshal(out)
}

// FieldMap returns the patchable fields of e as raw JSON values keyed by field
// name; two elements are equal in model terms iff their FieldMaps are equal.
func (e Element) FieldMap() map[string]json.RawMessage {
	if e.Tags == nil {
		e.Tags = []string{}
	}
	if e.Projects == nil {
		e.Projects = []string{}
	}
	e.Properties = NormalizeProperties(e.Properties)
	if e.Replicas == 0 {
		e.Replicas = 1 // legacy values (before replicas existed) compare equal to the default
	}
	return fieldMap(e, ElementPatchFields)
}

func (r Relation) FieldMap() map[string]json.RawMessage { return fieldMap(r, RelationPatchFields) }

func fieldMap(v any, keys []string) map[string]json.RawMessage {
	raw, err := FieldsOf(v, keys)
	if err != nil {
		panic(err) // Element / Relation always marshal
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(err)
	}
	return m
}

// MaxLayerLength bounds layer names (also used for view layer lists).
const MaxLayerLength = 60

// ValidateLayer checks an element layer name: empty (unassigned) or a short single-line label.
func ValidateLayer(layer string) error {
	if strings.TrimSpace(layer) != layer {
		return apperr.Invalid("layer must not start or end with whitespace")
	}
	if len([]rune(layer)) > MaxLayerLength || strings.ContainsAny(layer, "\r\n") {
		return apperr.Invalid("layer must be a single line of at most %d characters", MaxLayerLength)
	}
	return nil
}

// MaxProjects bounds how many projects one element can belong to.
const MaxProjects = 20

// NormalizeProjects trims names, drops blanks and duplicates (ignoring case) and never returns nil.
func NormalizeProjects(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, p := range in {
		p = strings.TrimSpace(p)
		key := strings.ToLower(p)
		if p == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// ValidateProjects checks an element's project list: short single-line names, no duplicates.
func ValidateProjects(projects []string) error {
	if len(projects) > MaxProjects {
		return apperr.Invalid("an element can belong to at most %d projects", MaxProjects)
	}
	seen := map[string]bool{}
	for _, p := range projects {
		if strings.TrimSpace(p) == "" || strings.TrimSpace(p) != p {
			return apperr.Invalid("project names must not be blank or start or end with whitespace")
		}
		if len([]rune(p)) > MaxLayerLength || strings.ContainsAny(p, "\r\n") {
			return apperr.Invalid("project must be a single line of at most %d characters", MaxLayerLength)
		}
		if seen[strings.ToLower(p)] {
			return apperr.Invalid("duplicate project %q", p)
		}
		seen[strings.ToLower(p)] = true
	}
	return nil
}

// MaxReplicas bounds Element.Replicas.
const MaxReplicas = 100

const (
	maxProperties    = 20
	maxPropertyKey   = 40
	maxPropertyValue = 200
)

// NormalizeProperties trims keys and values and drops entries with an empty key or value; never nil.
func NormalizeProperties(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

// ValidateProperties checks the properties map: a few short single-line entries, keys unique ignoring case.
func ValidateProperties(p map[string]string) error {
	if len(p) > maxProperties {
		return apperr.Invalid("an element can have at most %d properties", maxProperties)
	}
	seen := map[string]bool{}
	for k, v := range p {
		if k == "" || strings.TrimSpace(k) != k || len([]rune(k)) > maxPropertyKey || strings.ContainsAny(k, "\r\n") {
			return apperr.Invalid("property names must be 1-%d characters on a single line", maxPropertyKey)
		}
		if len([]rune(v)) > maxPropertyValue || strings.ContainsAny(v, "\r\n") {
			return apperr.Invalid("property %q must be a single line of at most %d characters", k, maxPropertyValue)
		}
		if seen[strings.ToLower(k)] {
			return apperr.Invalid("duplicate property %q", k)
		}
		seen[strings.ToLower(k)] = true
	}
	return nil
}
