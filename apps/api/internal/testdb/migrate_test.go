package testdb_test

import (
	"context"
	"testing"

	"archmap/api/internal/testdb"
)

// TestAuthMigration upgrades a dev-header-era database (users with password_hash)
// to identities, rolls back and upgrades again.
func TestAuthMigration(t *testing.T) {
	ctx := context.Background()
	pool := testdb.EmptyPool(t)
	ms, err := testdb.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	var auth testdb.Migration
	for _, m := range ms {
		if m.Name == "00009_auth.sql" {
			auth = m
			break
		}
		if _, err := pool.Exec(ctx, m.Up); err != nil {
			t.Fatalf("%s: %v", m.Name, err)
		}
	}
	if auth.Name == "" {
		t.Fatal("00009_auth.sql not found")
	}
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	count := func(sql string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	exec(`INSERT INTO users (email, name, role, password_hash) VALUES
		('Mia@Example.test', 'Mia', 'maintainer', '$argon2id$mia'),
		('ed@example.test', 'Ed', 'editor', '$argon2id$ed')`)

	exec(auth.Up)
	if n := count(`SELECT count(*) FROM user_identities i JOIN users u ON u.id = i.user_id
		WHERE i.issuer = 'urn:archmap:local' AND i.subject = lower(u.email) AND i.password_hash LIKE '$argon2id$%'`); n != 2 {
		t.Fatalf("local identities = %d, want 2", n)
	}
	if n := count(`SELECT count(*) FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'password_hash'`); n != 0 {
		t.Fatal("users.password_hash still exists")
	}
	// Email is no longer unique: two accounts may share one (never linked).
	exec(`INSERT INTO users (email, name, role) VALUES ('ed@example.test', 'Ed (SSO)', 'viewer')`)
	exec(`DELETE FROM users WHERE name = 'Ed (SSO)'`)

	exec(auth.Down)
	if n := count(`SELECT count(*) FROM users WHERE password_hash LIKE '$argon2id$%'`); n != 2 {
		t.Fatalf("restored password hashes = %d, want 2", n)
	}
	exec(auth.Up)
	if n := count(`SELECT count(*) FROM user_identities`); n != 2 {
		t.Fatalf("identities after re-upgrade = %d", n)
	}
}

// TestEnrichmentMigration retires the `database` kind on existing data (00010):
// datastores become containers of type database with the same id; down restores them.
func TestEnrichmentMigration(t *testing.T) {
	ctx := context.Background()
	pool := testdb.EmptyPool(t)
	ms, err := testdb.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	var enrich testdb.Migration
	for _, m := range ms {
		if m.Name == "00010_enrichment.sql" {
			enrich = m
			break
		}
		if _, err := pool.Exec(ctx, m.Up); err != nil {
			t.Fatalf("%s: %v", m.Name, err)
		}
	}
	if enrich.Name == "" {
		t.Fatal("00010_enrichment.sql not found")
	}
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO elements (id, kind, name) VALUES ('00000000-0000-7000-8000-000000000001', 'system', 'Shop')`)
	exec(`INSERT INTO elements (id, parent_id, kind, name) VALUES
		('00000000-0000-7000-8000-000000000002', '00000000-0000-7000-8000-000000000001', 'database', 'Orders'),
		('00000000-0000-7000-8000-000000000003', '00000000-0000-7000-8000-000000000001', 'container', 'API')`)

	exec(enrich.Up)
	var kind, typ, lifecycle string
	if err := pool.QueryRow(ctx, `SELECT kind::text, type, lifecycle FROM elements WHERE id = '00000000-0000-7000-8000-000000000002'`).
		Scan(&kind, &typ, &lifecycle); err != nil {
		t.Fatal(err)
	}
	if kind != "container" || typ != "database" || lifecycle != "active" {
		t.Fatalf("migrated datastore = %s/%s/%s", kind, typ, lifecycle)
	}
	var apiType string
	if err := pool.QueryRow(ctx, `SELECT type FROM elements WHERE name = 'API'`).Scan(&apiType); err != nil || apiType != "" {
		t.Fatalf("other elements must keep an empty type: %q, %v", apiType, err)
	}

	exec(enrich.Down)
	if err := pool.QueryRow(ctx, `SELECT kind::text FROM elements WHERE id = '00000000-0000-7000-8000-000000000002'`).Scan(&kind); err != nil || kind != "database" {
		t.Fatalf("down: kind = %s, %v", kind, err)
	}
	exec(enrich.Up)
}
