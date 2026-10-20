// Package httpx builds the Echo v5 HTTP shell: middleware (request info,
// cross-origin protection, session + CSRF), RFC 9457 error handling, request
// validation against the OpenAPI spec and the metrics endpoint. Echo is confined to this package and to feature transport packages.
package httpx

import (
	"log/slog"
	"net"
	"strings"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/labstack/echo-contrib/v5/echoprometheus"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	oapimw "github.com/oapi-codegen/echo-v5-middleware"
	"github.com/prometheus/client_golang/prometheus"

	"archmap/api/internal/gen/api"
)

type Options struct {
	Logger      *slog.Logger
	CORSOrigins []string
	// Registry receives HTTP metrics and backs GET /metrics.
	Registry *prometheus.Registry
	// Sessions resolves session cookies (required for authenticated routes).
	Sessions SessionResolver
	// TrustedProxies may set X-Forwarded-For; without any, the peer address is the client IP.
	TrustedProxies []*net.IPNet
}

// New returns an Echo instance with all cross-cutting middleware installed.
// Callers register feature routes on the result.
func New(opts Options) (*echo.Echo, error) {
	spec, err := api.GetSwagger()
	if err != nil {
		return nil, err
	}

	e := echo.New()
	e.Logger = opts.Logger
	e.HTTPErrorHandler = errorHandler()
	e.IPExtractor = ipExtractor(opts.TrustedProxies)
	cop, err := crossOrigin(opts.CORSOrigins)
	if err != nil {
		return nil, err
	}

	onlyAPI := func(c *echo.Context) bool { return !strings.HasPrefix(c.Request().URL.Path, "/api/") }

	e.Use(middleware.RequestID())
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		HandleError:  true, // let the error handler pick the status before we log it
		LogMethod:    true,
		LogURIPath:   true,
		LogStatus:    true,
		LogLatency:   true,
		LogRequestID: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			attrs := []slog.Attr{
				slog.String("request_id", v.RequestID),
				slog.String("method", v.Method),
				slog.String("path", v.URIPath),
				slog.Int("status", v.Status),
				slog.Duration("latency", v.Latency),
			}
			level := slog.LevelInfo
			if v.Error != nil {
				level = slog.LevelError
				attrs = append(attrs, slog.String("error", v.Error.Error()))
			}
			opts.Logger.LogAttrs(c.Request().Context(), level, "http request", attrs...)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(requestInfo())
	e.Use(cop)
	if len(opts.CORSOrigins) > 0 {
		e.Use(middleware.CORS(opts.CORSOrigins...))
	}
	if opts.Sessions != nil {
		e.Use(sessionAuth(opts.Sessions))
	}
	e.Use(echoprometheus.NewMiddlewareWithConfig(echoprometheus.MiddlewareConfig{
		Subsystem:                 "http",
		Registerer:                opts.Registry,
		DoNotUseRequestPathFor404: true,
	}))
	e.Use(oapimw.OapiRequestValidatorWithOptions(spec, &oapimw.Options{
		Skipper:              onlyAPI,
		DoNotValidateServers: true,
		// The spec documents the session cookie; enforcement is sessionAuth + services (ADR-061).
		Options: openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	}))

	e.GET("/metrics", echoprometheus.NewHandlerWithConfig(echoprometheus.HandlerConfig{
		Gatherer: opts.Registry,
	}))

	return e, nil
}

// ipExtractor trusts X-Forwarded-For only from the configured proxies.
func ipExtractor(trusted []*net.IPNet) echo.IPExtractor {
	if len(trusted) == 0 {
		return echo.ExtractIPDirect()
	}
	opts := []echo.TrustOption{echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false)}
	for _, n := range trusted {
		opts = append(opts, echo.TrustIPRange(n))
	}
	return echo.ExtractIPFromXFFHeader(opts...)
}
