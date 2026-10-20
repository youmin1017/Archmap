package testdb

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/user"
	userrepo "archmap/api/internal/user/repository"
)

// Password is the local password of every fixture user.
const Password = "test-password-123"

// Actors inserts one user per role and returns them as request actors.
func Actors(t testing.TB, pool *pgxpool.Pool) map[authctx.Role]authctx.Actor {
	t.Helper()
	out := map[authctx.Role]authctx.Actor{}
	for _, role := range []authctx.Role{authctx.RoleViewer, authctx.RoleEditor, authctx.RoleMaintainer} {
		out[role] = User(t, pool, string(role)+"@test.local", role)
	}
	return out
}

// User creates (or updates) an enabled account with a local password (Password) and returns it as an actor.
func User(t testing.TB, pool *pgxpool.Pool, email string, role authctx.Role) authctx.Actor {
	t.Helper()
	name, _, _ := strings.Cut(email, "@")
	id, err := user.NewService(userrepo.New(pool)).EnsureLocal(context.Background(), user.LocalAccount{
		Email: email, Name: name, Role: role, Password: Password,
	})
	if err != nil {
		t.Fatal(err)
	}
	return authctx.Actor{ID: id, Email: email, Name: name, Role: role}
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
