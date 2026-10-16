// Package repository is the PostgreSQL (sqlc) implementation of main-model storage.
package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"archmap/api/internal/model"
	"archmap/api/internal/model/repository/sqlcgen"
	"archmap/api/internal/platform/apperr"
)

type Repository struct {
	q *sqlcgen.Queries
}

// New builds a repository over a pool or a transaction.
func New(db sqlcgen.DBTX) *Repository { return &Repository{q: sqlcgen.New(db)} }

func (r *Repository) ListElements(ctx context.Context, f model.ElementFilter) ([]model.Element, error) {
	rows, err := r.q.ListElements(ctx, sqlcgen.ListElementsParams{
		ParentID: f.ParentID,
		Q:        pgtype.Text{String: f.Query, Valid: f.Query != ""},
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.Element, len(rows))
	for i, row := range rows {
		out[i] = toElement(row)
	}
	return out, nil
}

func (r *Repository) GetElement(ctx context.Context, id uuid.UUID) (model.Element, error) {
	row, err := r.q.GetElement(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Element{}, apperr.NotFound("element %s not found", id)
	}
	if err != nil {
		return model.Element{}, err
	}
	return toElement(row), nil
}

func (r *Repository) ListRelations(ctx context.Context, f model.RelationFilter) ([]model.Relation, error) {
	rows, err := r.q.ListRelations(ctx, sqlcgen.ListRelationsParams{SourceID: f.SourceID, TargetID: f.TargetID})
	if err != nil {
		return nil, err
	}
	out := make([]model.Relation, len(rows))
	for i, row := range rows {
		out[i] = toRelation(row)
	}
	return out, nil
}

func (r *Repository) GetRelation(ctx context.Context, id uuid.UUID) (model.Relation, error) {
	row, err := r.q.GetRelation(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Relation{}, apperr.NotFound("relation %s not found", id)
	}
	if err != nil {
		return model.Relation{}, err
	}
	return toRelation(row), nil
}

// Snapshot returns the whole current (non-deleted) main model.
func (r *Repository) Snapshot(ctx context.Context) (model.Snapshot, error) {
	els, err := r.ListElements(ctx, model.ElementFilter{})
	if err != nil {
		return model.Snapshot{}, err
	}
	rels, err := r.ListRelations(ctx, model.RelationFilter{})
	if err != nil {
		return model.Snapshot{}, err
	}
	return model.Snapshot{Elements: els, Relations: rels}, nil
}

// The methods below materialize changes into main. They are only called by a
// merge (or the dev seed bootstrap); the HTTP API never writes main directly.

func (r *Repository) InsertElement(ctx context.Context, e model.Element) error {
	return r.q.InsertElement(ctx, sqlcgen.InsertElementParams{
		ID: e.ID, ParentID: e.ParentID, Kind: sqlcgen.ElementKind(e.Kind), Name: e.Name,
		Description: e.Description, Technology: e.Technology, Tags: nonNil(e.Tags), Protected: e.Protected, Layer: e.Layer, Projects: nonNil(e.Projects),
		Replicas: int32(max(e.Replicas, 1)), Properties: propsJSON(e.Properties),
	})
}

func (r *Repository) UpdateElement(ctx context.Context, e model.Element) error {
	return r.q.UpdateElement(ctx, sqlcgen.UpdateElementParams{
		ID: e.ID, ParentID: e.ParentID, Kind: sqlcgen.ElementKind(e.Kind), Name: e.Name,
		Description: e.Description, Technology: e.Technology, Tags: nonNil(e.Tags), Protected: e.Protected, Layer: e.Layer, Projects: nonNil(e.Projects),
		Replicas: int32(max(e.Replicas, 1)), Properties: propsJSON(e.Properties),
	})
}

func (r *Repository) SoftDeleteElement(ctx context.Context, id uuid.UUID) error {
	return r.q.SoftDeleteElement(ctx, id)
}

func (r *Repository) InsertRelation(ctx context.Context, rel model.Relation) error {
	return r.q.InsertRelation(ctx, sqlcgen.InsertRelationParams{
		ID: rel.ID, SourceID: rel.SourceID, TargetID: rel.TargetID, Kind: sqlcgen.RelationKind(rel.Kind),
		Description: rel.Description, Technology: rel.Technology,
	})
}

func (r *Repository) UpdateRelation(ctx context.Context, rel model.Relation) error {
	return r.q.UpdateRelation(ctx, sqlcgen.UpdateRelationParams{
		ID: rel.ID, SourceID: rel.SourceID, TargetID: rel.TargetID, Kind: sqlcgen.RelationKind(rel.Kind),
		Description: rel.Description, Technology: rel.Technology,
	})
}

func (r *Repository) SoftDeleteRelation(ctx context.Context, id uuid.UUID) error {
	return r.q.SoftDeleteRelation(ctx, id)
}

func toElement(e sqlcgen.Element) model.Element {
	return model.Element{
		ID: e.ID, ParentID: e.ParentID, Kind: model.ElementKind(e.Kind), Name: e.Name,
		Description: e.Description, Technology: e.Technology, Tags: nonNil(e.Tags), Protected: e.Protected, Layer: e.Layer, Projects: nonNil(e.Projects),
		Replicas: int(e.Replicas), Properties: propsFromJSON(e.Properties),
		Version: int(e.Version), DeletedAt: e.DeletedAt, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

func toRelation(r sqlcgen.Relation) model.Relation {
	return model.Relation{
		ID: r.ID, SourceID: r.SourceID, TargetID: r.TargetID, Kind: model.RelationKind(r.Kind),
		Description: r.Description, Technology: r.Technology,
		Version: int(r.Version), DeletedAt: r.DeletedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func propsJSON(p map[string]string) []byte {
	raw, err := json.Marshal(model.NormalizeProperties(p))
	if err != nil {
		panic(err) // a string map always marshals
	}
	return raw
}

func propsFromJSON(raw []byte) map[string]string {
	var p map[string]string
	if err := json.Unmarshal(raw, &p); err != nil {
		return map[string]string{}
	}
	return model.NormalizeProperties(p)
}
