// Package repository is the PostgreSQL (sqlc) implementation of user management storage.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

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
	db DB // nil inside a transaction
}

func New(db DB) *Repository { return &Repository{q: sqlcgen.New(db), db: db} }

var _ user.Repository = (*Repository)(nil)

func (r *Repository) InTx(ctx context.Context, fn func(user.Repository) error) error {
	if r.db == nil { // already inside a transaction: join it
		return fn(r)
	}
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		return fn(&Repository{q: sqlcgen.New(tx)})
	})
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// uniqueViolation turns a unique-key error into a Conflict with msg.
func uniqueViolation(err error, msg string) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return apperr.Conflict("%s", msg)
	}
	return err
}

func toUser(id uuid.UUID, email pgtype.Text, name string, role sqlcgen.UserRole, created time.Time,
	disabled, lastLogin *time.Time, identities int32,
) user.User {
	return user.User{
		ID: id, Email: email.String, Name: name, Role: authctx.Role(role), CreatedAt: created,
		Disabled: disabled != nil, LastLoginAt: lastLogin, IdentityCount: int(identities),
	}
}

func (r *Repository) List(ctx context.Context) ([]user.User, error) {
	rows, err := r.q.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]user.User, len(rows))
	for i, u := range rows {
		out[i] = toUser(u.ID, u.Email, u.Name, u.Role, u.CreatedAt, u.DisabledAt, u.LastLoginAt, u.IdentityCount)
	}
	return out, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (user.User, error) {
	u, err := r.q.GetUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, apperr.NotFound("user %s not found", id)
	}
	if err != nil {
		return user.User{}, err
	}
	return toUser(u.ID, u.Email, u.Name, u.Role, u.CreatedAt, u.DisabledAt, u.LastLoginAt, u.IdentityCount), nil
}

func (r *Repository) LockMaintainers(ctx context.Context) ([]user.Maintainer, error) {
	rows, err := r.q.LockMaintainers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]user.Maintainer, len(rows))
	for i, m := range rows {
		out[i] = user.Maintainer{ID: m.ID, Enabled: m.Enabled, HasLocal: m.HasLocal}
	}
	return out, nil
}

func (r *Repository) LockUser(ctx context.Context, id uuid.UUID) (user.LockedUser, error) {
	u, err := r.q.GetUserForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.LockedUser{}, apperr.NotFound("user %s not found", id)
	}
	if err != nil {
		return user.LockedUser{}, err
	}
	return user.LockedUser{ID: u.ID, Email: u.Email.String, Role: authctx.Role(u.Role), Disabled: u.DisabledAt != nil}, nil
}

func (r *Repository) SetRole(ctx context.Context, id uuid.UUID, role authctx.Role) error {
	return r.q.SetUserRole(ctx, sqlcgen.SetUserRoleParams{ID: id, Role: sqlcgen.UserRole(role)})
}

func (r *Repository) SetName(ctx context.Context, id uuid.UUID, name string) error {
	return r.q.SetUserName(ctx, sqlcgen.SetUserNameParams{ID: id, Name: name})
}

func (r *Repository) SetDisabled(ctx context.Context, id uuid.UUID, at *time.Time) error {
	return r.q.SetUserDisabled(ctx, sqlcgen.SetUserDisabledParams{ID: id, DisabledAt: at})
}

func (r *Repository) DeleteSessions(ctx context.Context, userID uuid.UUID) error {
	return r.q.DeleteUserSessions(ctx, userID)
}

func (r *Repository) CreateUser(ctx context.Context, email, name string, role authctx.Role) (uuid.UUID, error) {
	return r.q.CreateUser(ctx, sqlcgen.CreateUserParams{Name: name, Role: sqlcgen.UserRole(role), Email: text(email)})
}

func (r *Repository) CreateLocalIdentity(ctx context.Context, userID uuid.UUID, subject, email, hash string, mustChange bool) error {
	_, err := r.q.CreateLocalIdentity(ctx, sqlcgen.CreateLocalIdentityParams{
		UserID: userID, Subject: subject, Email: text(email), PasswordHash: text(hash), PasswordMustChange: mustChange,
	})
	return uniqueViolation(err, "another account already signs in with this email")
}

func (r *Repository) FindLocalIdentity(ctx context.Context, subject string) (uuid.UUID, bool, error) {
	row, err := r.q.GetLocalIdentityBySubject(ctx, subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return row.UserID, true, nil
}

func (r *Repository) LocalIdentity(ctx context.Context, userID uuid.UUID) (uuid.UUID, bool, error) {
	id, err := r.q.GetLocalIdentity(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	return id, err == nil, err
}

func (r *Repository) SetLocalPassword(ctx context.Context, identityID uuid.UUID, hash string, mustChange bool) error {
	return r.q.SetLocalPassword(ctx, sqlcgen.SetLocalPasswordParams{ID: identityID, PasswordHash: text(hash), PasswordMustChange: mustChange})
}

func toIdentity(id, userID uuid.UUID, provider, issuer, subject string, email pgtype.Text, created time.Time, last *time.Time) user.Identity {
	return user.Identity{
		ID: id, UserID: userID, Provider: provider, Issuer: issuer, Subject: subject,
		Email: email.String, CreatedAt: created, LastLoginAt: last,
	}
}

func (r *Repository) Identities(ctx context.Context, userID uuid.UUID) ([]user.Identity, error) {
	rows, err := r.q.ListIdentities(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]user.Identity, len(rows))
	for i, x := range rows {
		out[i] = toIdentity(x.ID, x.UserID, x.Provider, x.Issuer, x.Subject, x.Email, x.CreatedAt, x.LastLoginAt)
	}
	return out, nil
}

func (r *Repository) LinkIdentity(ctx context.Context, userID uuid.UUID, provider, issuer, subject string) (user.Identity, error) {
	x, err := r.q.CreateExternalIdentity(ctx, sqlcgen.CreateExternalIdentityParams{
		UserID: userID, Provider: provider, Issuer: issuer, Subject: subject,
	})
	if err != nil {
		return user.Identity{}, uniqueViolation(err, "this identity is already linked to an account")
	}
	return toIdentity(x.ID, x.UserID, x.Provider, x.Issuer, x.Subject, x.Email, x.CreatedAt, x.LastLoginAt), nil
}

func (r *Repository) LockIdentity(ctx context.Context, userID, identityID uuid.UUID) (string, error) {
	row, err := r.q.GetIdentityForUpdate(ctx, sqlcgen.GetIdentityForUpdateParams{ID: identityID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.NotFound("identity %s not found for user %s", identityID, userID)
	}
	if err != nil {
		return "", err
	}
	return row.Issuer, nil
}

func (r *Repository) DeleteIdentity(ctx context.Context, identityID uuid.UUID) error {
	return r.q.DeleteIdentity(ctx, identityID)
}
