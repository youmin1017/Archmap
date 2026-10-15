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

	"archmap/api/internal/gen/api"
	"archmap/api/internal/health"
	"archmap/api/internal/platform/config"
	"archmap/api/internal/platform/db"
	"archmap/api/internal/platform/httpx"
	"archmap/api/internal/platform/logger"
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

	e, err := httpx.New(httpx.Options{
		Logger:      log,
		CORSOrigins: cfg.CORSOrigins,
		Registry:    prometheus.NewRegistry(),
	})
	if err != nil {
		return fmt.Errorf("build http server: %w", err)
	}

	healthHandler := health.NewHandler(health.NewService(cfg.Version, pool))
	api.RegisterHandlers(e, api.NewStrictHandler(healthHandler, nil))

	log.Info("api listening", "addr", cfg.Addr, "version", cfg.Version)
	sc := echo.StartConfig{Address: cfg.Addr}
	if err := sc.Start(ctx, e); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
