package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Snapshots are stored as JSON per merged version. The wire shapes below are
// independent of Element / Relation (whose json tags describe change patches),
// and match the SQL backfill in migration 00003.
type snapshotElement struct {
	ID          uuid.UUID         `json:"id"`
	ParentID    *uuid.UUID        `json:"parent_id"`
	Kind        ElementKind       `json:"kind"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Technology  string            `json:"technology"`
	Tags        []string          `json:"tags"`
	Protected   bool              `json:"protected"`
	Layer       string            `json:"layer"`      // missing in snapshots written before layers existed: reads as ""
	Projects    []string          `json:"projects"`   // missing in older snapshots: reads as empty
	Replicas    int               `json:"replicas"`   // missing in older snapshots: reads as 1
	Properties  map[string]string `json:"properties"` // missing in older snapshots: reads as empty
	// M10 fields; missing in older snapshots: read as the defaults (kind "database" is normalized).
	Type           ElementType `json:"type"`
	Environment    Environment `json:"environment"`
	Lifecycle      Lifecycle   `json:"lifecycle"`
	Criticality    Criticality `json:"criticality"`
	RunbookURL     string      `json:"runbook_url"`
	DocsURL        string      `json:"docs_url"`
	LastReviewedAt *string     `json:"last_reviewed_at"`
	Version        int         `json:"version"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type snapshotRelation struct {
	ID          uuid.UUID    `json:"id"`
	SourceID    uuid.UUID    `json:"source_id"`
	TargetID    uuid.UUID    `json:"target_id"`
	Kind        RelationKind `json:"kind"`
	Description string       `json:"description"`
	Technology  string       `json:"technology"`
	Version     int          `json:"version"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type snapshotOwner struct {
	ID        uuid.UUID `json:"id"`
	Kind      OwnerKind `json:"kind"`
	Name      string    `json:"name"`
	Contact   string    `json:"contact"`
	OncallURL string    `json:"oncall_url"`
	// PrincipalID: user / group owners (ADR-069); missing before: nil.
	PrincipalID *uuid.UUID `json:"principal_id"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type snapshotOwnership struct {
	ID        uuid.UUID `json:"id"`
	ElementID uuid.UUID `json:"element_id"`
	OwnerID   uuid.UUID `json:"owner_id"`
	Role      OwnerRole `json:"role"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type snapshotDoc struct {
	Elements   []snapshotElement   `json:"elements"`
	Relations  []snapshotRelation  `json:"relations"`
	Owners     []snapshotOwner     `json:"owners"`     // missing before M10: none
	Ownerships []snapshotOwnership `json:"ownerships"` // missing before M10: none
}

func MarshalSnapshot(s Snapshot) ([]byte, error) {
	doc := snapshotDoc{
		Elements:   make([]snapshotElement, len(s.Elements)),
		Relations:  make([]snapshotRelation, len(s.Relations)),
		Owners:     make([]snapshotOwner, len(s.Owners)),
		Ownerships: make([]snapshotOwnership, len(s.Ownerships)),
	}
	for i, e := range s.Elements {
		e = e.normalized()
		tags := e.Tags
		if tags == nil {
			tags = []string{}
		}
		doc.Elements[i] = snapshotElement{
			ID: e.ID, ParentID: e.ParentID, Kind: e.Kind, Name: e.Name, Description: e.Description,
			Technology: e.Technology, Tags: tags, Protected: e.Protected, Layer: e.Layer, Projects: NormalizeProjects(e.Projects), Replicas: max(e.Replicas, 1), Properties: NormalizeProperties(e.Properties),
			Type: e.Type, Environment: e.Environment, Lifecycle: e.Lifecycle, Criticality: e.Criticality,
			RunbookURL: e.RunbookURL, DocsURL: e.DocsURL, LastReviewedAt: e.LastReviewedAt, Version: e.Version,
			CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
		}
	}
	for i, o := range s.Owners {
		doc.Owners[i] = snapshotOwner{
			ID: o.ID, Kind: o.Kind, Name: o.Name, Contact: o.Contact, OncallURL: o.OncallURL, PrincipalID: o.PrincipalID,
			Version: o.Version, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
		}
	}
	for i, o := range s.Ownerships {
		doc.Ownerships[i] = snapshotOwnership{
			ID: o.ID, ElementID: o.ElementID, OwnerID: o.OwnerID, Role: o.Role,
			Version: o.Version, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
		}
	}
	for i, r := range s.Relations {
		doc.Relations[i] = snapshotRelation{
			ID: r.ID, SourceID: r.SourceID, TargetID: r.TargetID, Kind: r.Kind, Description: r.Description,
			Technology: r.Technology, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	return json.Marshal(doc)
}

func UnmarshalSnapshot(data []byte) (Snapshot, error) {
	var doc snapshotDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return Snapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	s := Snapshot{
		Elements:   make([]Element, len(doc.Elements)),
		Relations:  make([]Relation, len(doc.Relations)),
		Owners:     make([]Owner, len(doc.Owners)),
		Ownerships: make([]Ownership, len(doc.Ownerships)),
	}
	for i, e := range doc.Elements {
		tags := e.Tags
		if tags == nil {
			tags = []string{}
		}
		s.Elements[i] = Element{
			ID: e.ID, ParentID: e.ParentID, Kind: e.Kind, Name: e.Name, Description: e.Description,
			Technology: e.Technology, Tags: tags, Protected: e.Protected, Layer: e.Layer, Projects: NormalizeProjects(e.Projects), Replicas: max(e.Replicas, 1), Properties: NormalizeProperties(e.Properties),
			Type: e.Type, Environment: e.Environment, Lifecycle: e.Lifecycle, Criticality: e.Criticality,
			RunbookURL: e.RunbookURL, DocsURL: e.DocsURL, LastReviewedAt: e.LastReviewedAt, Version: e.Version,
			CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
		}.normalized()
	}
	for i, o := range doc.Owners {
		s.Owners[i] = Owner{
			ID: o.ID, Kind: o.Kind, Name: o.Name, Contact: o.Contact, OncallURL: o.OncallURL, PrincipalID: o.PrincipalID,
			Version: o.Version, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
		}
	}
	for i, o := range doc.Ownerships {
		s.Ownerships[i] = Ownership{
			ID: o.ID, ElementID: o.ElementID, OwnerID: o.OwnerID, Role: o.Role,
			Version: o.Version, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
		}
	}
	for i, r := range doc.Relations {
		s.Relations[i] = Relation{
			ID: r.ID, SourceID: r.SourceID, TargetID: r.TargetID, Kind: r.Kind, Description: r.Description,
			Technology: r.Technology, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	return s, nil
}
