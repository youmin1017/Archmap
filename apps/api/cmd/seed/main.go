// Command seed upserts the dev users (viewer / editor / maintainer). Dev only.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"archmap/api/internal/platform/config"
	"archmap/api/internal/platform/db"
	"archmap/api/internal/user"
	"archmap/api/internal/user/repository/sqlcgen"
)

const devPassword = "archmap-dev" // dev seed only; never used outside local environments

func main() {
	if err := run(); err != nil {
		log.Println("seed:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	hash, err := user.HashPassword(devPassword)
	if err != nil {
		return err
	}
	q := sqlcgen.New(pool)
	for _, u := range []struct {
		email, name string
		role        sqlcgen.UserRole
	}{
		{"viewer@archmap.local", "Vera Viewer", sqlcgen.UserRoleViewer},
		{"editor@archmap.local", "Eddie Editor", sqlcgen.UserRoleEditor},
		{"maintainer@archmap.local", "Mia Maintainer", sqlcgen.UserRoleMaintainer},
	} {
		if _, err := q.UpsertUser(ctx, sqlcgen.UpsertUserParams{
			Email: u.email, Name: u.name, Role: u.role, PasswordHash: hash,
		}); err != nil {
			return fmt.Errorf("upsert %s: %w", u.email, err)
		}
		fmt.Printf("seeded %s (%s)\n", u.email, u.role)
	}
	return nil
}
