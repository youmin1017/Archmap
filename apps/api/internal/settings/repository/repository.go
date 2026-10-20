// Package repository is the PostgreSQL (sqlc) implementation of settings storage.
package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"archmap/api/internal/settings"
	"archmap/api/internal/settings/repository/sqlcgen"
)

type Repository struct{ q *sqlcgen.Queries }

func New(pool *pgxpool.Pool) *Repository { return &Repository{q: sqlcgen.New(pool)} }

func (r *Repository) Get(ctx context.Context) (settings.Settings, error) {
	row, err := r.q.GetSettings(ctx)
	if err != nil {
		return settings.Settings{}, err
	}
	return settings.Settings{ReviewPolicy: settings.ReviewPolicy(row.ReviewPolicy), UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt}, nil
}

func (r *Repository) SetReviewPolicy(ctx context.Context, p settings.ReviewPolicy, by uuid.UUID) (settings.Settings, error) {
	row, err := r.q.UpdateReviewPolicy(ctx, sqlcgen.UpdateReviewPolicyParams{ReviewPolicy: string(p), UpdatedBy: &by})
	if err != nil {
		return settings.Settings{}, err
	}
	return settings.Settings{ReviewPolicy: settings.ReviewPolicy(row.ReviewPolicy), UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt}, nil
}

// DirectEditsAllowed satisfies the changeset service's policy source (no actor
// check: it is asked inside an already authorized edit).
func (r *Repository) DirectEditsAllowed(ctx context.Context) (bool, error) {
	s, err := r.Get(ctx)
	return s.ReviewPolicy == settings.PolicyDirect, err
}
