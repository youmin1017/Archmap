// Package repository is the PostgreSQL (sqlc) implementation of view storage.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/view"
	"archmap/api/internal/view/repository/sqlcgen"
)

type Repository struct {
	pool *pgxpool.Pool // nil when bound to a transaction
	q    *sqlcgen.Queries
}

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool, q: sqlcgen.New(pool)} }

func (r *Repository) InTx(ctx context.Context, fn func(view.Repository) error) error {
	if r.pool == nil {
		return fn(r)
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error { return fn(&Repository{q: sqlcgen.New(tx)}) })
}

func (r *Repository) Insert(ctx context.Context, v view.View) (view.View, error) {
	row, err := r.q.InsertView(ctx, sqlcgen.InsertViewParams{
		Name: v.Name, Kind: sqlcgen.ViewKind(v.Kind), ScopeElementID: v.ScopeElementID,
		Description: v.Description, CreatedBy: v.CreatedBy, Layers: v.Layers,
	})
	return toView(row), err
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (view.View, error) {
	row, err := r.q.GetView(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return view.View{}, apperr.NotFound("view %s not found", id)
	}
	return toView(row), err
}

func (r *Repository) List(ctx context.Context) ([]view.View, error) {
	rows, err := r.q.ListViews(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]view.View, len(rows))
	for i, row := range rows {
		out[i] = toView(row)
	}
	return out, nil
}

func (r *Repository) Update(ctx context.Context, v view.View) (view.View, error) {
	row, err := r.q.UpdateView(ctx, sqlcgen.UpdateViewParams{
		ID: v.ID, Name: v.Name, Kind: sqlcgen.ViewKind(v.Kind), ScopeElementID: v.ScopeElementID, Description: v.Description, Layers: v.Layers,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return view.View{}, apperr.NotFound("view %s not found", v.ID)
	}
	return toView(row), err
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := r.q.DeleteView(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.NotFound("view %s not found", id)
	}
	return nil
}

func (r *Repository) Elements(ctx context.Context, viewID uuid.UUID) ([]view.Element, error) {
	rows, err := r.q.ListViewElements(ctx, viewID)
	if err != nil {
		return nil, err
	}
	out := make([]view.Element, len(rows))
	for i, row := range rows {
		out[i] = view.Element{
			ElementID: row.ElementID, X: row.X, Y: row.Y, W: row.W, H: row.H,
			Collapsed: row.Collapsed, Included: row.Included,
		}
	}
	return out, nil
}

func (r *Repository) SetMembers(ctx context.Context, viewID uuid.UUID, ids []uuid.UUID) error {
	if err := r.q.ClearViewMembers(ctx, viewID); err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.q.UpsertViewMember(ctx, sqlcgen.UpsertViewMemberParams{ViewID: viewID, ElementID: id}); err != nil {
			return err
		}
	}
	return r.q.PruneViewElements(ctx, viewID)
}

func (r *Repository) SetLayout(ctx context.Context, viewID uuid.UUID, layout []view.Element) error {
	if err := r.q.ResetViewLayout(ctx, viewID); err != nil {
		return err
	}
	for _, l := range layout {
		if err := r.q.UpsertViewLayout(ctx, sqlcgen.UpsertViewLayoutParams{
			ViewID: viewID, ElementID: l.ElementID, X: l.X, Y: l.Y, W: l.W, H: l.H, Collapsed: l.Collapsed,
		}); err != nil {
			return err
		}
	}
	return r.q.PruneViewElements(ctx, viewID)
}

func toView(v sqlcgen.View) view.View {
	return view.View{
		ID: v.ID, Name: v.Name, Kind: view.Kind(v.Kind), ScopeElementID: v.ScopeElementID,
		Description: v.Description, Layers: nonNilStrings(v.Layers), CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
