package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"archmap/api/internal/gen/api"
	"archmap/api/internal/health"
	"archmap/api/internal/platform/httpx"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

// healthOnly satisfies the full generated interface while serving just GetHealth;
// the other operations are nil and never reached by these tests.
type healthOnly struct {
	api.StrictServerInterface
	h *health.Handler
}

func (s healthOnly) GetHealth(ctx context.Context, r api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	return s.h.GetHealth(ctx, r)
}

func newServer(t *testing.T, p health.Pinger) http.Handler {
	t.Helper()
	e, err := httpx.New(httpx.Options{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry: prometheus.NewRegistry(),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := health.NewHandler(health.NewService("test", p))
	api.RegisterHandlers(e, api.NewStrictHandler(healthOnly{h: h}, nil))
	return e
}

func TestHealthEndpoint(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		path        string
		pingErr     error
		wantStatus  int
		wantContent string
	}{
		{"healthy", http.MethodGet, "/api/v1/health", nil, http.StatusOK, "application/json"},
		{"db down", http.MethodGet, "/api/v1/health", errors.New("boom"), http.StatusServiceUnavailable, "application/problem+json"},
		{"unknown api route", http.MethodGet, "/api/v1/nope", nil, http.StatusNotFound, "application/problem+json"},
		{"method not in spec", http.MethodPost, "/api/v1/health", nil, http.StatusMethodNotAllowed, "application/problem+json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, fakePinger{err: tc.pingErr})
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tc.wantContent) {
				t.Fatalf("content-type = %q, want %q", got, tc.wantContent)
			}
			if tc.wantStatus == http.StatusOK {
				var body api.Health
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body.Status != api.Ok || body.Version != "test" {
					t.Fatalf("unexpected body: %+v", body)
				}
			}
		})
	}
}

func TestMetricsEndpoint(t *testing.T) {
	srv := newServer(t, fakePinger{})
	srv.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "http_requests_total") {
		t.Fatalf("metrics output missing http_requests_total:\n%s", rec.Body)
	}
}
