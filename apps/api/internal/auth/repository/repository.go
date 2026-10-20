// Package repository is the PostgreSQL (sqlc) storage of identities, sessions and login flows.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"archmap/api/internal/auth"
	"archmap/api/internal/auth/repository/sqlcgen"
	"archmap/api/internal/platform/authctx"
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

var _ auth.Repository = (*Repository)(nil)

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func (r *Repository) FindIdentity(ctx context.Context, issuer, subject string) (auth.IdentityRecord, bool, error) {
	row, err := r.q.GetIdentity(ctx, sqlcgen.GetIdentityParams{Issuer: issuer, Subject: subject})
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.IdentityRecord{}, false, nil
	}
	if err != nil {
		return auth.IdentityRecord{}, false, err
	}
	return auth.IdentityRecord{
		ID: row.ID, UserID: row.UserID, Role: authctx.Role(row.Role),
		Disabled: row.DisabledAt != nil, PasswordHash: row.PasswordHash.String,
	}, true, nil
}

func (r *Repository) Provision(ctx context.Context, id auth.Identity, role authctx.Role) (auth.IdentityRecord, error) {
	name := id.Name
	if name == "" {
		name = id.Email
	}
	if name == "" {
		name = id.Subject
	}
	var rec auth.IdentityRecord
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		userID, err := q.CreateProvisionedUser(ctx, sqlcgen.CreateProvisionedUserParams{
			Name: name, Role: sqlcgen.UserRole(role), Email: text(id.Email),
		})
		if err != nil {
			return err
		}
		identityID, err := q.CreateIdentity(ctx, sqlcgen.CreateIdentityParams{
			UserID: userID, Provider: id.Provider, Issuer: id.Issuer, Subject: id.Subject, Email: text(id.Email),
		})
		if err != nil {
			return err
		}
		rec = auth.IdentityRecord{ID: identityID, UserID: userID, Role: role}
		return nil
	})
	return rec, err
}

func (r *Repository) RecordLogin(ctx context.Context, rec auth.IdentityRecord, email string, at time.Time) error {
	return pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if err := q.TouchIdentityLogin(ctx, sqlcgen.TouchIdentityLoginParams{ID: rec.ID, LastLoginAt: &at, Email: text(email)}); err != nil {
			return err
		}
		return q.TouchUserLogin(ctx, sqlcgen.TouchUserLoginParams{ID: rec.UserID, LastLoginAt: &at})
	})
}

func (r *Repository) SetRole(ctx context.Context, userID uuid.UUID, role authctx.Role) error {
	return r.q.SetUserRole(ctx, sqlcgen.SetUserRoleParams{ID: userID, Role: sqlcgen.UserRole(role)})
}

func (r *Repository) CreateSession(ctx context.Context, s auth.SessionRecord) error {
	identityID := s.IdentityID
	return r.q.CreateSession(ctx, sqlcgen.CreateSessionParams{
		TokenHash: s.TokenHash, UserID: s.UserID, IdentityID: &identityID, Provider: s.Provider,
		CsrfToken: s.CSRFToken, CreatedAt: s.CreatedAt, IdleExpiresAt: s.IdleExpiresAt, ExpiresAt: s.ExpiresAt,
		UserAgent: s.UserAgent, Ip: s.IP,
	})
}

func (r *Repository) GetSession(ctx context.Context, tokenHash []byte, now time.Time) (auth.ActiveSession, bool, error) {
	row, err := r.q.GetSession(ctx, sqlcgen.GetSessionParams{TokenHash: tokenHash, Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ActiveSession{}, false, nil
	}
	if err != nil {
		return auth.ActiveSession{}, false, err
	}
	return auth.ActiveSession{
		Actor: authctx.Actor{
			ID: row.UserID, Email: row.Email.String, Name: row.Name, Role: authctx.Role(row.Role),
			Provider: row.Provider, PasswordMustChange: row.PasswordMustChange,
		},
		CSRFToken: row.CsrfToken, LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt,
	}, true, nil
}

func (r *Repository) TouchSession(ctx context.Context, tokenHash []byte, now, idleExpiresAt time.Time) error {
	return r.q.TouchSession(ctx, sqlcgen.TouchSessionParams{TokenHash: tokenHash, Now: now, IdleExpiresAt: idleExpiresAt})
}

func (r *Repository) DeleteSession(ctx context.Context, tokenHash []byte) error {
	return r.q.DeleteSession(ctx, tokenHash)
}

func (r *Repository) DeleteOtherSessions(ctx context.Context, userID uuid.UUID, keep []byte) error {
	return r.q.DeleteOtherSessions(ctx, sqlcgen.DeleteOtherSessionsParams{UserID: userID, TokenHash: keep})
}

func (r *Repository) DeleteExpired(ctx context.Context, now time.Time) error {
	if _, err := r.q.DeleteExpiredSessions(ctx, now); err != nil {
		return err
	}
	_, err := r.q.DeleteExpiredFlows(ctx, now)
	return err
}

func (r *Repository) SaveFlow(ctx context.Context, f auth.Flow) error {
	return r.q.CreateFlow(ctx, sqlcgen.CreateFlowParams{
		ID: f.ID, Provider: f.Provider, State: f.State, Nonce: f.Nonce,
		PkceVerifier: f.PKCEVerifier, ReturnTo: f.ReturnTo, ExpiresAt: f.ExpiresAt,
	})
}

func (r *Repository) TakeFlow(ctx context.Context, id string) (auth.Flow, bool, error) {
	row, err := r.q.TakeFlow(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Flow{}, false, nil
	}
	if err != nil {
		return auth.Flow{}, false, err
	}
	return auth.Flow{
		ID: id, Provider: row.Provider, State: row.State, Nonce: row.Nonce,
		PKCEVerifier: row.PkceVerifier, ReturnTo: row.ReturnTo, ExpiresAt: row.ExpiresAt,
	}, true, nil
}

func (r *Repository) LocalPassword(ctx context.Context, userID uuid.UUID) (uuid.UUID, string, bool, error) {
	row, err := r.q.GetLocalPassword(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", false, nil
	}
	if err != nil {
		return uuid.Nil, "", false, err
	}
	return row.ID, row.PasswordHash.String, true, nil
}

func (r *Repository) SetPassword(ctx context.Context, identityID uuid.UUID, hash string) error {
	return r.q.SetPassword(ctx, sqlcgen.SetPasswordParams{ID: identityID, PasswordHash: text(hash)})
}
