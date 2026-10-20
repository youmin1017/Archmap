// Command seed upserts the dev users (viewer / editor / maintainer). Dev only.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"archmap/api/internal/changeset"
	changesetrepo "archmap/api/internal/changeset/repository"
	"archmap/api/internal/layerrule"
	layerrulerepo "archmap/api/internal/layerrule/repository"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/platform/config"
	"archmap/api/internal/platform/db"
	"archmap/api/internal/seed"
	"archmap/api/internal/user"
	userrepo "archmap/api/internal/user/repository"
	"archmap/api/internal/view"
	viewrepo "archmap/api/internal/view/repository"
)

const devPassword = "archmap-dev-password" // dev seed only; never used outside local environments

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

	users := user.NewService(userrepo.New(pool))
	for _, u := range []struct {
		email, name string
		role        authctx.Role
	}{
		{"viewer@archmap.local", "Vera Viewer", authctx.RoleViewer},
		{"editor@archmap.local", "Eddie Editor", authctx.RoleEditor},
		{"maintainer@archmap.local", "Mia Maintainer", authctx.RoleMaintainer},
	} {
		if _, err := users.EnsureLocal(ctx, user.LocalAccount{
			Email: u.email, Name: u.name, Role: u.role, Password: devPassword,
		}); err != nil {
			return fmt.Errorf("seed %s: %w", u.email, err)
		}
		fmt.Printf("seeded %s (%s)\n", u.email, u.role)
	}
	if err := seedModel(ctx, pool); err != nil {
		return err
	}
	if err := seedLayersView(ctx, pool); err != nil {
		return err
	}
	return seedLayerRules(ctx, pool)
}

// seedModel creates main's version 1 from the sample model; a no-op once any version exists.
func seedModel(ctx context.Context, pool *pgxpool.Pool) error {
	maintainer, err := seedMaintainer(ctx, pool)
	if err != nil {
		return err
	}
	svc := changeset.NewService(changesetrepo.New(pool))
	_, err = svc.Bootstrap(authctx.With(ctx, maintainer), "Sample model", seed.SampleModel())
	switch {
	case apperr.KindOf(err) == apperr.KindConflict:
		fmt.Println("sample model already present; skipped")
		return nil
	case err != nil:
		return fmt.Errorf("seed sample model: %w", err)
	}
	fmt.Println("seeded sample model (version 1)")
	return nil
}

// seedLayersView creates a sample layers view once; later runs leave views alone.
func seedLayersView(ctx context.Context, pool *pgxpool.Pool) error {
	maintainer, err := seedMaintainer(ctx, pool)
	if err != nil {
		return err
	}
	svc := view.NewService(viewrepo.New(pool), nil)
	ctx = authctx.With(ctx, maintainer)
	existing, err := svc.List(ctx)
	if err != nil {
		return err
	}
	for _, v := range existing {
		if v.Kind == view.KindLayers {
			fmt.Println("layers view already present; skipped")
			return nil
		}
	}
	if _, err := svc.Create(ctx, view.Input{Name: "Layers", Kind: view.KindLayers, Layers: seed.SampleLayers}); err != nil {
		return fmt.Errorf("seed layers view: %w", err)
	}
	fmt.Println("seeded layers view")
	return nil
}

// seedLayerRules sets a sample rule (databases belong on Services or Data) when no rules exist yet.
func seedLayerRules(ctx context.Context, pool *pgxpool.Pool) error {
	maintainer, err := seedMaintainer(ctx, pool)
	if err != nil {
		return err
	}
	svc := layerrule.NewService(layerrulerepo.New(pool))
	ctx = authctx.With(ctx, maintainer)
	existing, err := svc.List(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		fmt.Println("layer rules already present; skipped")
		return nil
	}
	if _, err := svc.Replace(ctx, []model.LayerRule{{Class: string(model.TypeDatabase), Layers: seed.SampleDatabaseLayers}}); err != nil {
		return fmt.Errorf("seed layer rules: %w", err)
	}
	fmt.Println("seeded layer rules")
	return nil
}

// seedMaintainer returns the seeded maintainer as an actor.
func seedMaintainer(ctx context.Context, pool *pgxpool.Pool) (authctx.Actor, error) {
	id, err := user.NewService(userrepo.New(pool)).EnsureLocal(ctx, user.LocalAccount{
		Email: "maintainer@archmap.local", Name: "Mia Maintainer", Role: authctx.RoleMaintainer, Password: devPassword,
	})
	if err != nil {
		return authctx.Actor{}, err
	}
	return authctx.Actor{ID: id, Email: "maintainer@archmap.local", Name: "Mia Maintainer", Role: authctx.RoleMaintainer}, nil
}
