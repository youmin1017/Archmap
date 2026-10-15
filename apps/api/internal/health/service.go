// Package health reports service liveness and readiness.
package health

import (
	"context"
	"fmt"
)

// Pinger is satisfied by *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Service struct {
	version string
	db      Pinger
}

func NewService(version string, db Pinger) *Service {
	return &Service{version: version, db: db}
}

type Status struct {
	Version string
}

// Check returns an error when a dependency is unavailable.
func (s *Service) Check(ctx context.Context) (Status, error) {
	if err := s.db.Ping(ctx); err != nil {
		return Status{}, fmt.Errorf("database: %w", err)
	}
	return Status{Version: s.version}, nil
}
