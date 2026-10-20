// Package config loads runtime configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"
)

// Env is the deployment environment. Anything but development is treated as production-like.
type Env string

const (
	EnvDevelopment Env = "development"
	EnvTest        Env = "test"
	EnvProduction  Env = "production"
)

// LocalPassword says who may sign in with a local password (ADR-059).
type LocalPassword string

const (
	LocalPasswordAll         LocalPassword = "all"
	LocalPasswordMaintainers LocalPassword = "maintainers"
)

type Config struct {
	Env         Env
	Addr        string
	DatabaseURL string
	LogLevel    slog.Level
	CORSOrigins []string
	Version     string
	// MergeAllowSelf lets a maintainer approve and merge their own changeset.
	MergeAllowSelf bool
	// ReviewMaxAge is how old an element review may be before lint warns (LINT_REVIEW_MAX_AGE).
	ReviewMaxAge time.Duration
	Auth         Auth
}

type Auth struct {
	// DevLogin registers the passwordless dev provider. Development only (startup fails otherwise).
	DevLogin bool
	// CookieSecure marks cookies Secure and uses the __Host- prefix. Only development may turn it off.
	CookieSecure  bool
	IdleTimeout   time.Duration
	MaxAge        time.Duration
	LocalPassword LocalPassword
	// TrustedProxies may set X-Forwarded-For (client IP for rate limiting and session records).
	TrustedProxies []*net.IPNet
}

// Load reads the environment. APP_ENV defaults to production so that a forgotten
// development flag fails closed.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	get := func(key, fallback string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return fallback
	}
	cfg := Config{
		Env:            Env(get("APP_ENV", string(EnvProduction))),
		Addr:           get("API_ADDR", ":8080"),
		DatabaseURL:    getenv("DATABASE_URL"),
		CORSOrigins:    splitCSV(get("CORS_ALLOW_ORIGINS", "http://localhost:3000")),
		Version:        get("APP_VERSION", "dev"),
		MergeAllowSelf: get("MERGE_ALLOW_SELF", "false") == "true",
	}
	switch cfg.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		return Config{}, fmt.Errorf("invalid APP_ENV %q (development, test or production)", cfg.Env)
	}
	var err error
	if cfg.ReviewMaxAge, err = time.ParseDuration(get("LINT_REVIEW_MAX_AGE", "4320h")); err != nil || cfg.ReviewMaxAge <= 0 {
		return Config{}, fmt.Errorf("invalid LINT_REVIEW_MAX_AGE %q (e.g. 4320h for 180 days)", getenv("LINT_REVIEW_MAX_AGE"))
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("invalid LOG_LEVEL: %w", err)
	}

	a := &cfg.Auth
	a.DevLogin = get("AUTH_DEV_LOGIN", "false") == "true"
	a.CookieSecure = get("COOKIE_SECURE", "true") != "false"
	a.LocalPassword = LocalPassword(get("AUTH_LOCAL_PASSWORD", string(LocalPasswordAll)))
	if a.IdleTimeout, err = time.ParseDuration(get("SESSION_IDLE_TIMEOUT", "8h")); err != nil || a.IdleTimeout <= 0 {
		return Config{}, fmt.Errorf("invalid SESSION_IDLE_TIMEOUT %q", getenv("SESSION_IDLE_TIMEOUT"))
	}
	if a.MaxAge, err = time.ParseDuration(get("SESSION_MAX_AGE", "168h")); err != nil || a.MaxAge <= 0 {
		return Config{}, fmt.Errorf("invalid SESSION_MAX_AGE %q", getenv("SESSION_MAX_AGE"))
	}
	for _, c := range splitCSV(getenv("TRUSTED_PROXIES")) {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return Config{}, fmt.Errorf("invalid TRUSTED_PROXIES entry %q: %w", c, err)
		}
		a.TrustedProxies = append(a.TrustedProxies, n)
	}
	if getenv("AUTH_DEV_HEADER") != "" {
		return Config{}, errors.New("AUTH_DEV_HEADER was removed; use AUTH_DEV_LOGIN=true with APP_ENV=development")
	}
	switch a.LocalPassword {
	case LocalPasswordAll, LocalPasswordMaintainers:
	default:
		return Config{}, fmt.Errorf("invalid AUTH_LOCAL_PASSWORD %q (all or maintainers)", a.LocalPassword)
	}
	if cfg.Env != EnvDevelopment {
		if a.DevLogin {
			return Config{}, fmt.Errorf("AUTH_DEV_LOGIN=true is only allowed with APP_ENV=development (APP_ENV=%s)", cfg.Env)
		}
		if !a.CookieSecure {
			return Config{}, fmt.Errorf("COOKIE_SECURE=false is only allowed with APP_ENV=development (APP_ENV=%s)", cfg.Env)
		}
	}
	return cfg, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
