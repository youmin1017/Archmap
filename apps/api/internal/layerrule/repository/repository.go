// Package repository is the PostgreSQL (sqlc) implementation of layer rule storage.
package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"archmap/api/internal/layerrule/repository/sqlcgen"
	"archmap/api/internal/model"
)

type Repository struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool, q: sqlcgen.New(pool)} }

func (r *Repository) List(ctx context.Context) ([]model.LayerRule, error) {
	rows, err := r.q.ListLayerRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.LayerRule, len(rows))
	for i, row := range rows {
		out[i] = model.LayerRule{Class: row.ElementClass, Layers: row.Layers}
	}
	return out, nil
}

func (r *Repository) Replace(ctx context.Context, rules []model.LayerRule) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if err := q.DeleteLayerRules(ctx); err != nil {
			return err
		}
		for _, rule := range rules {
			if err := q.InsertLayerRule(ctx, sqlcgen.InsertLayerRuleParams{ElementClass: rule.Class, Layers: rule.Layers}); err != nil {
				return err
			}
		}
		return nil
	})
}
