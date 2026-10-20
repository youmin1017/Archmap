package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/prometheus/client_golang/prometheus"

	"archmap/api/internal/apiserver"
	"archmap/api/internal/auth"
	"archmap/api/internal/auth/devlogin"
	"archmap/api/internal/auth/local"
	authrepo "archmap/api/internal/auth/repository"
	authtransport "archmap/api/internal/auth/transport"
	"archmap/api/internal/changeset"
	changesetrepo "archmap/api/internal/changeset/repository"
	changesettransport "archmap/api/internal/changeset/transport"
	"archmap/api/internal/gen/api"
	"archmap/api/internal/health"
	"archmap/api/internal/layerrule"
	layerrulerepo "archmap/api/internal/layerrule/repository"
	layerruletransport "archmap/api/internal/layerrule/transport"
	"archmap/api/internal/lint"
	"archmap/api/internal/model"
	modelrepo "archmap/api/internal/model/repository"
	modeltransport "archmap/api/internal/model/transport"
	"archmap/api/internal/platform/config"
	"archmap/api/internal/platform/db"
	"archmap/api/internal/platform/httpx"
	"archmap/api/internal/platform/logger"
	"archmap/api/internal/settings"
	settingsrepo "archmap/api/internal/settings/repository"
	settingstransport "archmap/api/internal/settings/transport"
	"archmap/api/internal/user"
	userrepo "archmap/api/internal/user/repository"
	usertransport "archmap/api/internal/user/transport"
	"archmap/api/internal/view"
	viewrepo "archmap/api/internal/view/repository"
	viewtransport "archmap/api/internal/view/transport"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	authSvc, err := newAuthService(cfg, authrepo.New(pool), log)
	if err != nil {
		return err
	}
	go authSvc.RunSweeper(ctx, time.Hour)

	e, err := httpx.New(httpx.Options{
		Logger:         log,
		CORSOrigins:    cfg.CORSOrigins,
		Registry:       prometheus.NewRegistry(),
		Sessions:       authSvc,
		TrustedProxies: cfg.Auth.TrustedProxies,
	})
	if err != nil {
		return fmt.Errorf("build http server: %w", err)
	}

	layerRuleSvc := layerrule.NewService(layerrulerepo.New(pool))
	changesetSvc := changeset.NewService(changesetrepo.New(pool), changeset.WithSelfMerge(cfg.MergeAllowSelf), changeset.WithLayerRules(layerRuleSvc),
		changeset.WithLintOptions(lint.Options{ReviewMaxAge: cfg.ReviewMaxAge}),
		changeset.WithPrincipals(userrepo.NewGroups(pool)), changeset.WithPolicy(settingsrepo.New(pool)))
	viewSvc := view.NewService(viewrepo.New(pool), apiserver.NewViewModelSource(modelrepo.New(pool), changesetSvc))
	server := apiserver.New(
		authtransport.NewHandler(authSvc, log),
		health.NewHandler(health.NewService(cfg.Version, pool)),
		modeltransport.NewHandler(model.NewService(modelrepo.New(pool))),
		changesettransport.NewHandler(changesetSvc),
		usertransport.NewHandler(user.NewService(userrepo.New(pool), user.WithProviders(authSvc))),
		usertransport.NewGroupHandler(user.NewGroupService(userrepo.NewGroups(pool))),
		viewtransport.NewHandler(viewSvc),
		layerruletransport.NewHandler(layerRuleSvc),
		settingstransport.NewHandler(settings.NewService(settingsrepo.New(pool))),
	)
	api.RegisterHandlers(e, api.NewStrictHandler(server, nil))

	log.Info("api listening", "addr", cfg.Addr, "version", cfg.Version, "env", cfg.Env)
	sc := echo.StartConfig{Address: cfg.Addr}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// newAuthService registers the sign-in providers allowed by the configuration.
func newAuthService(cfg config.Config, repo *authrepo.Repository, log *slog.Logger) (*auth.Service, error) {
	localProvider, err := local.New(repo, cfg.Auth.LocalPassword == config.LocalPasswordMaintainers)
	if err != nil {
		return nil, err
	}
	providers := []auth.Provider{localProvider}
	if cfg.Auth.DevLogin { // config.Load guarantees APP_ENV=development
		log.Warn("AUTH_DEV_LOGIN enabled: anyone can sign in as any local account without a password (development only)")
		providers = append(providers, devlogin.New())
	}
	return auth.NewService(repo, providers, auth.Options{
		IdleTimeout:  cfg.Auth.IdleTimeout,
		MaxAge:       cfg.Auth.MaxAge,
		CookieSecure: cfg.Auth.CookieSecure,
		Logger:       log,
	})
}
