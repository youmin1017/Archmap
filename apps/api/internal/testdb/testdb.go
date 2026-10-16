// Package testdb provides throwaway PostgreSQL databases for integration
// tests. A single postgres:18 container (testcontainers-go) is shared per test
// binary; every call to Pool creates a fresh, fully migrated database inside
// it, so tests are isolated and never touch the manually started dev database.
// Tests are skipped under `go test -short`.
package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const image = "postgres:18"

var (
	once    sync.Once
	adminDB string // connection string of the shared container's "postgres" database
	initErr error
	counter atomic.Int64
)

// Pool returns a pool on a new migrated database. It is closed on test cleanup.
func Pool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test (needs Docker); skipped with -short")
	}
	ctx := context.Background()

	once.Do(func() {
		// The container is removed by testcontainers' reaper when the test binary exits.
		c, err := postgres.Run(ctx, image,
			postgres.WithDatabase("postgres"),
			postgres.WithUsername("archmap"),
			postgres.WithPassword("archmap"),
			postgres.BasicWaitStrategies(),
		)
		if err != nil {
			initErr = fmt.Errorf("start %s container: %w", image, err)
			return
		}
		adminDB, initErr = c.ConnectionString(ctx, "sslmode=disable")
	})
	if initErr != nil {
		t.Fatal(initErr)
	}

	name := fmt.Sprintf("t%d", counter.Add(1))
	admin, err := pgx.Connect(ctx, adminDB)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}

	cfg, err := pgxpool.ParseConfig(adminDB)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err := migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

// migrate applies the "Up" section of every goose migration in db/migrations.
// goose itself is not a library dependency (see ADR-009); the migrations are
// plain SQL, so running the Up halves in order is equivalent for tests.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations")
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return fmt.Errorf("no migrations found in %s", dir)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		up, _, _ := strings.Cut(string(raw), "-- +goose Down")
		up = strings.Replace(up, "-- +goose Up", "", 1)
		if _, err := pool.Exec(ctx, up); err != nil {
			return fmt.Errorf("apply %s: %w", filepath.Base(f), err)
		}
	}
	return nil
}
