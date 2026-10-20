package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/user"
	"archmap/api/internal/user/repository/sqlcgen"
)

// GroupRepository stores groups (ADR-070) and serves the owner directory.
type GroupRepository struct {
	q  *sqlcgen.Queries
	db DB
}

func NewGroups(db DB) *GroupRepository { return &GroupRepository{q: sqlcgen.New(db), db: db} }

var _ user.GroupRepository = (*GroupRepository)(nil)

func (r *GroupRepository) ListGroups(ctx context.Context) ([]user.Group, error) {
	rows, err := r.q.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	members, err := r.q.ListGroupMembers(ctx)
	if err != nil {
		return nil, err
	}
	byGroup := map[uuid.UUID][]uuid.UUID{}
	for _, m := range members {
		byGroup[m.GroupID] = append(byGroup[m.GroupID], m.UserID)
	}
	out := make([]user.Group, len(rows))
	for i, g := range rows {
		ms := byGroup[g.ID]
		if ms == nil {
			ms = []uuid.UUID{}
		}
		out[i] = user.Group{ID: g.ID, Name: g.Name, Description: g.Description, CreatedAt: g.CreatedAt, Members: ms}
	}
	return out, nil
}

func (r *GroupRepository) DirectoryUsers(ctx context.Context) ([]user.DirectoryUser, error) {
	rows, err := r.q.ListDirectoryUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]user.DirectoryUser, len(rows))
	for i, u := range rows {
		out[i] = user.DirectoryUser{ID: u.ID, Name: u.Name, Email: u.Email.String, Disabled: u.Disabled}
	}
	return out, nil
}

func (r *GroupRepository) CreateGroup(ctx context.Context, name, description string) (uuid.UUID, error) {
	id, err := r.q.CreateGroup(ctx, sqlcgen.CreateGroupParams{Name: name, Description: description})
	return id, groupNameTaken(err)
}

func (r *GroupRepository) UpdateGroup(ctx context.Context, id uuid.UUID, name, description string) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if _, err := lockGroup(ctx, q, id); err != nil {
			return err
		}
		return groupNameTaken(q.UpdateGroup(ctx, sqlcgen.UpdateGroupParams{ID: id, Name: name, Description: description}))
	})
}

func (r *GroupRepository) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if _, err := lockGroup(ctx, q, id); err != nil {
			return err
		}
		if err := q.ClearGroupMembers(ctx, id); err != nil {
			return err
		}
		return q.SoftDeleteGroup(ctx, id)
	})
}

func (r *GroupRepository) SetGroupMembers(ctx context.Context, id uuid.UUID, users []uuid.UUID) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if _, err := lockGroup(ctx, q, id); err != nil {
			return err
		}
		if err := q.ClearGroupMembers(ctx, id); err != nil {
			return err
		}
		for _, u := range users {
			err := q.AddGroupMember(ctx, sqlcgen.AddGroupMemberParams{GroupID: id, UserID: u})
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "23503" { // foreign key: unknown user
				return apperr.Invalid("user %s does not exist", u)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// Principals lists every user and group (deleted groups included) for owner links;
// it implements changeset.PrincipalSource without an actor check (internal use).
func (r *GroupRepository) Principals(ctx context.Context) (model.Principals, error) {
	rows, err := r.q.ListPrincipals(ctx)
	if err != nil {
		return nil, err
	}
	out := make(model.Principals, len(rows))
	for _, p := range rows {
		out[p.ID] = model.Principal{ID: p.ID, Kind: model.OwnerKind(p.Kind), Name: p.Name, Inactive: p.Inactive}
	}
	return out, nil
}

func lockGroup(ctx context.Context, q *sqlcgen.Queries, id uuid.UUID) (sqlcgen.GetGroupForUpdateRow, error) {
	g, err := q.GetGroupForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, apperr.NotFound("group %s not found", id)
	}
	return g, err
}

func groupNameTaken(err error) error {
	return uniqueViolation(err, "a group with this name already exists")
}
