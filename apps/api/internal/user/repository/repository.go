// Package repository is the PostgreSQL (sqlc) implementation of user lookups.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/user"
	"archmap/api/internal/user/repository/sqlcgen"
)

// DB is what the repository needs: queries plus transactions.
type DB interface {
	sqlcgen.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Repository struct {
	q  *sqlcgen.Queries
	db DB
}

func New(db DB) *Repository { return &Repository{q: sqlcgen.New(db), db: db} }

func toUser(id uuid.UUID, email, name string, role sqlcgen.UserRole, created time.Time) user.User {
	return user.User{ID: id, Email: email, Name: name, Role: authctx.Role(role), CreatedAt: created}
}

func (r *Repository) List(ctx context.Context) ([]user.User, error) {
	rows, err := r.q.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]user.User, len(rows))
	for i, u := range rows {
		out[i] = toUser(u.ID, u.Email, u.Name, u.Role, u.CreatedAt)
	}
	return out, nil
}

func (r *Repository) ChangeRole(ctx context.Context, id uuid.UUID, role authctx.Role, check func(authctx.Role, int) error) (user.User, error) {
	var out user.User
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		maintainers, err := q.LockMaintainers(ctx)
		if err != nil {
			return err
		}
		cur, err := q.GetUserForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("user %s not found", id)
		}
		if err != nil {
			return err
		}
		if err := check(authctx.Role(cur.Role), len(maintainers)); err != nil {
			return err
		}
		u, err := q.SetUserRole(ctx, sqlcgen.SetUserRoleParams{ID: id, Role: sqlcgen.UserRole(role)})
		if err != nil {
			return err
		}
		out = toUser(u.ID, u.Email, u.Name, u.Role, u.CreatedAt)
		return nil
	})
	return out, err
}

// ActorByEmail resolves a user into the actor carried through the request context.
func (r *Repository) ActorByEmail(ctx context.Context, email string) (authctx.Actor, error) {
	u, err := r.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return authctx.Actor{}, apperr.Unauthenticated("unknown user %q", email)
	}
	if err != nil {
		return authctx.Actor{}, err
	}
	return authctx.Actor{ID: u.ID, Email: u.Email, Name: u.Name, Role: authctx.Role(u.Role)}, nil
}
