package testdb

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/user/repository/sqlcgen"
)

// Actors inserts one user per role and returns them as request actors.
func Actors(t testing.TB, pool *pgxpool.Pool) map[authctx.Role]authctx.Actor {
	t.Helper()
	q := sqlcgen.New(pool)
	out := map[authctx.Role]authctx.Actor{}
	for _, role := range []authctx.Role{authctx.RoleViewer, authctx.RoleEditor, authctx.RoleMaintainer} {
		email := string(role) + "@test.local"
		u, err := q.UpsertUser(context.Background(), sqlcgen.UpsertUserParams{
			Email: email, Name: string(role), Role: sqlcgen.UserRole(role), PasswordHash: "x",
		})
		if err != nil {
			t.Fatal(err)
		}
		out[role] = authctx.Actor{ID: u.ID, Email: u.Email, Name: u.Name, Role: role}
	}
	return out
}

// MainDump returns a deterministic JSON dump of every main-model table
// (including soft-deleted rows) so tests can assert main is byte-identical.
func MainDump(t testing.TB, pool *pgxpool.Pool) string {
	t.Helper()
	var dump string
	err := pool.QueryRow(context.Background(), `
		SELECT jsonb_build_object(
			'elements',       (SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY e.id), '[]') FROM elements e),
			'relations',      (SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY r.id), '[]') FROM relations r),
			'model_versions', (SELECT coalesce(jsonb_agg(to_jsonb(v) ORDER BY v.number), '[]') FROM model_versions v)
		)::text`).Scan(&dump)
	if err != nil {
		t.Fatal(err)
	}
	return dump
}
