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

	"github.com/labstack/echo/v5"
	"github.com/prometheus/client_golang/prometheus"

	"archmap/api/internal/apiserver"
	"archmap/api/internal/changeset"
	changesetrepo "archmap/api/internal/changeset/repository"
	changesettransport "archmap/api/internal/changeset/transport"
	"archmap/api/internal/gen/api"
	"archmap/api/internal/health"
	"archmap/api/internal/layerrule"
	layerrulerepo "archmap/api/internal/layerrule/repository"
	layerruletransport "archmap/api/internal/layerrule/transport"
	"archmap/api/internal/model"
	modelrepo "archmap/api/internal/model/repository"
	modeltransport "archmap/api/internal/model/transport"
	"archmap/api/internal/platform/config"
	"archmap/api/internal/platform/db"
	"archmap/api/internal/platform/httpx"
	"archmap/api/internal/platform/logger"
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

	opts := httpx.Options{
		Logger:      log,
		CORSOrigins: cfg.CORSOrigins,
		Registry:    prometheus.NewRegistry(),
	}
	if cfg.AuthDevHeader {
		log.Warn("AUTH_DEV_HEADER enabled: callers are identified by the X-Dev-User header (development only)")
		opts.DevActorLookup = userrepo.New(pool).ActorByEmail
	}
	e, err := httpx.New(opts)
	if err != nil {
		return fmt.Errorf("build http server: %w", err)
	}

	layerRuleSvc := layerrule.NewService(layerrulerepo.New(pool))
	changesetSvc := changeset.NewService(changesetrepo.New(pool), changeset.WithSelfMerge(cfg.MergeAllowSelf), changeset.WithLayerRules(layerRuleSvc))
	viewSvc := view.NewService(viewrepo.New(pool), apiserver.NewViewModelSource(modelrepo.New(pool), changesetSvc))
	server := apiserver.New(
		health.NewHandler(health.NewService(cfg.Version, pool)),
		modeltransport.NewHandler(model.NewService(modelrepo.New(pool))),
		changesettransport.NewHandler(changesetSvc),
		usertransport.NewHandler(user.NewService(userrepo.New(pool))),
		viewtransport.NewHandler(viewSvc),
		layerruletransport.NewHandler(layerRuleSvc),
	)
	api.RegisterHandlers(e, api.NewStrictHandler(server, nil))

	log.Info("api listening", "addr", cfg.Addr, "version", cfg.Version)
	sc := echo.StartConfig{Address: cfg.Addr}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
