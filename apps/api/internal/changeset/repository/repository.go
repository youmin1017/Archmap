// Package repository is the PostgreSQL (sqlc) implementation of changeset.Repository.
package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"archmap/api/internal/changeset"
	"archmap/api/internal/changeset/repository/sqlcgen"
	"archmap/api/internal/model"
	modelrepo "archmap/api/internal/model/repository"
	"archmap/api/internal/platform/apperr"
)

type Repository struct {
	pool   *pgxpool.Pool // nil when bound to a transaction
	q      *sqlcgen.Queries
	models *modelrepo.Repository
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: sqlcgen.New(pool), models: modelrepo.New(pool)}
}

func (r *Repository) InTx(ctx context.Context, fn func(changeset.Repository) error) error {
	if r.pool == nil { // already inside a transaction: join it
		return fn(r)
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		return fn(&Repository{q: sqlcgen.New(tx), models: modelrepo.New(tx)})
	})
}

func (r *Repository) InsertChangeset(ctx context.Context, cs changeset.Changeset) (changeset.Changeset, error) {
	row, err := r.q.InsertChangeset(ctx, sqlcgen.InsertChangesetParams{
		Title: cs.Title, Description: cs.Description, AuthorID: cs.AuthorID,
		BaseVersion: int32(cs.BaseVersion), Status: sqlcgen.ChangesetStatus(cs.Status),
	})
	if err != nil {
		return changeset.Changeset{}, err
	}
	return toChangeset(row), nil
}

func (r *Repository) GetChangeset(ctx context.Context, id uuid.UUID) (changeset.Changeset, error) {
	row, err := r.q.GetChangeset(ctx, id)
	return changesetOrNotFound(row, err, id)
}

func (r *Repository) LockChangeset(ctx context.Context, id uuid.UUID) (changeset.Changeset, error) {
	row, err := r.q.GetChangesetForUpdate(ctx, id)
	return changesetOrNotFound(row, err, id)
}

func changesetOrNotFound(row sqlcgen.Changeset, err error, id uuid.UUID) (changeset.Changeset, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return changeset.Changeset{}, apperr.NotFound("changeset %s not found", id)
	}
	if err != nil {
		return changeset.Changeset{}, err
	}
	return toChangeset(row), nil
}

func (r *Repository) ListChangesets(ctx context.Context, f changeset.Filter) ([]changeset.Changeset, error) {
	p := sqlcgen.ListChangesetsParams{AuthorID: f.AuthorID}
	if f.Status != nil {
		p.Status = sqlcgen.NullChangesetStatus{ChangesetStatus: sqlcgen.ChangesetStatus(*f.Status), Valid: true}
	}
	rows, err := r.q.ListChangesets(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]changeset.Changeset, len(rows))
	for i, row := range rows {
		out[i] = toChangeset(row)
	}
	return out, nil
}

func (r *Repository) CurrentVersion(ctx context.Context) (int, error) {
	v, err := r.q.CurrentModelVersion(ctx)
	return int(v), err
}

func (r *Repository) MainSnapshot(ctx context.Context) (model.Snapshot, error) {
	return r.models.Snapshot(ctx)
}

func (r *Repository) ListChanges(ctx context.Context, id uuid.UUID) ([]changeset.Change, error) {
	rows, err := r.q.ListChanges(ctx, id)
	return toChanges(rows), err
}

func (r *Repository) ActiveChanges(ctx context.Context, id uuid.UUID) ([]changeset.Change, error) {
	rows, err := r.q.ListActiveChanges(ctx, id)
	return toChanges(rows), err
}

func (r *Repository) MaxSeq(ctx context.Context, id uuid.UUID) (int, error) {
	v, err := r.q.MaxChangeSeq(ctx, id)
	return int(v), err
}

func (r *Repository) InsertChange(ctx context.Context, c changeset.Change) (changeset.Change, error) {
	row, err := r.q.InsertChange(ctx, sqlcgen.InsertChangeParams{
		ChangesetID: c.ChangesetID, Seq: int32(c.Seq), BatchID: c.BatchID,
		Op: sqlcgen.ChangeOp(c.Op), TargetType: sqlcgen.ChangeTarget(c.TargetType),
		TargetID: c.TargetID, Patch: c.Patch, Before: c.Before,
	})
	if err != nil {
		return changeset.Change{}, err
	}
	return toChange(row), nil
}

func (r *Repository) DeleteUndone(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteUndoneChanges(ctx, id)
}

func (r *Repository) UndoLastBatch(ctx context.Context, id uuid.UUID) ([]changeset.Change, error) {
	rows, err := r.q.UndoLastBatch(ctx, id)
	return toChanges(rows), err
}

func (r *Repository) RedoNextBatch(ctx context.Context, id uuid.UUID) ([]changeset.Change, error) {
	rows, err := r.q.RedoNextBatch(ctx, id)
	return toChanges(rows), err
}

// MaterializeChanges writes changes (in seq order) into the main tables.
func (r *Repository) MaterializeChanges(ctx context.Context, changes []changeset.Change) error {
	for _, c := range changes {
		var err error
		switch c.TargetType {
		case changeset.TargetElement:
			err = r.materializeElement(ctx, c)
		case changeset.TargetRelation:
			err = r.materializeRelation(ctx, c)
		default:
			err = apperr.Invalid("unknown target type %q", c.TargetType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) materializeElement(ctx context.Context, c changeset.Change) error {
	switch c.Op {
	case changeset.OpAdd:
		e, err := model.ApplyElementPatch(model.Element{ID: c.TargetID}, c.Patch)
		if err != nil {
			return err
		}
		return r.models.InsertElement(ctx, e)
	case changeset.OpUpdate:
		cur, err := r.models.GetElement(ctx, c.TargetID)
		if err != nil {
			return err
		}
		e, err := model.ApplyElementPatch(cur, c.Patch)
		if err != nil {
			return err
		}
		return r.models.UpdateElement(ctx, e)
	case changeset.OpDelete:
		return r.models.SoftDeleteElement(ctx, c.TargetID)
	}
	return apperr.Invalid("unknown op %q", c.Op)
}

func (r *Repository) materializeRelation(ctx context.Context, c changeset.Change) error {
	switch c.Op {
	case changeset.OpAdd:
		rel, err := model.ApplyRelationPatch(model.Relation{ID: c.TargetID}, c.Patch)
		if err != nil {
			return err
		}
		return r.models.InsertRelation(ctx, rel)
	case changeset.OpUpdate:
		cur, err := r.models.GetRelation(ctx, c.TargetID)
		if err != nil {
			return err
		}
		rel, err := model.ApplyRelationPatch(cur, c.Patch)
		if err != nil {
			return err
		}
		return r.models.UpdateRelation(ctx, rel)
	case changeset.OpDelete:
		return r.models.SoftDeleteRelation(ctx, c.TargetID)
	}
	return apperr.Invalid("unknown op %q", c.Op)
}

func (r *Repository) RecordVersion(ctx context.Context, number int, changesetID, mergedBy uuid.UUID, summary string, snapshot model.Snapshot) error {
	if err := r.q.InsertModelVersion(ctx, sqlcgen.InsertModelVersionParams{
		Number: int32(number), ChangesetID: changesetID, MergedBy: mergedBy, Summary: summary,
	}); err != nil {
		return err
	}
	data, err := model.MarshalSnapshot(snapshot)
	if err != nil {
		return err
	}
	return r.q.InsertSnapshot(ctx, sqlcgen.InsertSnapshotParams{Version: int32(number), Data: data})
}

// SnapshotAt returns main as of a merged version; version 0 is the empty model.
func (r *Repository) SnapshotAt(ctx context.Context, version int) (model.Snapshot, error) {
	if version == 0 {
		return model.Snapshot{Elements: []model.Element{}, Relations: []model.Relation{}}, nil
	}
	data, err := r.q.GetSnapshot(ctx, int32(version))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Snapshot{}, apperr.NotFound("version %d not found", version)
	}
	if err != nil {
		return model.Snapshot{}, err
	}
	return model.UnmarshalSnapshot(data)
}

func (r *Repository) ListVersions(ctx context.Context) ([]changeset.Version, error) {
	rows, err := r.q.ListModelVersions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]changeset.Version, len(rows))
	for i, v := range rows {
		out[i] = changeset.Version{
			Number: int(v.Number), ChangesetID: v.ChangesetID, MergedBy: v.MergedBy, MergedAt: v.MergedAt, Summary: v.Summary,
		}
	}
	return out, nil
}

// LockMain takes a transaction-scoped advisory lock that serializes merges.
func (r *Repository) LockMain(ctx context.Context) error { return r.q.LockMain(ctx) }

func (r *Repository) ApproveChangeset(ctx context.Context, id, by uuid.UUID) (changeset.Changeset, error) {
	row, err := r.q.ApproveChangeset(ctx, sqlcgen.ApproveChangesetParams{ID: id, ApprovedBy: &by})
	return changesetOrNotFound(row, err, id)
}

func (r *Repository) SetStatus(ctx context.Context, id uuid.UUID, status changeset.Status) (changeset.Changeset, error) {
	row, err := r.q.SetChangesetStatus(ctx, sqlcgen.SetChangesetStatusParams{ID: id, Status: sqlcgen.ChangesetStatus(status)})
	return changesetOrNotFound(row, err, id)
}

func (r *Repository) Resolutions(ctx context.Context, id uuid.UUID) ([]changeset.Resolution, error) {
	rows, err := r.q.ListResolutions(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]changeset.Resolution, len(rows))
	for i, row := range rows {
		out[i] = changeset.Resolution{
			ResolutionKey: changeset.ResolutionKey{
				TargetType: changeset.TargetType(row.TargetType), TargetID: row.TargetID, Field: row.Field,
			},
			Choice: changeset.Choice(row.Choice),
		}
	}
	return out, nil
}

func (r *Repository) SaveResolutions(ctx context.Context, id uuid.UUID, rs []changeset.Resolution) error {
	for _, res := range rs {
		if err := r.q.UpsertResolution(ctx, sqlcgen.UpsertResolutionParams{
			ChangesetID: id, TargetType: sqlcgen.ChangeTarget(res.TargetType), TargetID: res.TargetID,
			Field: res.Field, Choice: sqlcgen.ResolutionChoice(res.Choice),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) MarkMerged(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkChangesetMerged(ctx, id)
}

func toChangeset(c sqlcgen.Changeset) changeset.Changeset {
	return changeset.Changeset{
		ID: c.ID, Title: c.Title, Description: c.Description, AuthorID: c.AuthorID,
		BaseVersion: int(c.BaseVersion), Status: changeset.Status(c.Status),
		CreatedAt: c.CreatedAt, MergedAt: c.MergedAt, ApprovedBy: c.ApprovedBy, ApprovedAt: c.ApprovedAt,
	}
}

func toChange(c sqlcgen.Change) changeset.Change {
	return changeset.Change{
		ID: c.ID, ChangesetID: c.ChangesetID, Seq: int(c.Seq), BatchID: c.BatchID,
		Op: changeset.Op(c.Op), TargetType: changeset.TargetType(c.TargetType), TargetID: c.TargetID,
		Patch: c.Patch, Before: c.Before, UndoneAt: c.UndoneAt, CreatedAt: c.CreatedAt,
	}
}

func toChanges(rows []sqlcgen.Change) []changeset.Change {
	out := make([]changeset.Change, len(rows))
	for i, row := range rows {
		out[i] = toChange(row)
	}
	return out
}
