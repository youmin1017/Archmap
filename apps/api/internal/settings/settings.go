// Package settings holds the workspace-wide settings, today the review policy
// (ADR-072). Settings are administration, not model data: they are not versioned.
package settings

import (
	"context"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

// ReviewPolicy decides how main changes: only through reviewed drafts, or also by
// direct edits that are merged automatically.
type ReviewPolicy string

const (
	PolicyRequired ReviewPolicy = "required"
	PolicyDirect   ReviewPolicy = "direct"
)

func (p ReviewPolicy) Valid() bool { return p == PolicyRequired || p == PolicyDirect }

type Settings struct {
	ReviewPolicy ReviewPolicy
	UpdatedBy    *uuid.UUID
	UpdatedAt    time.Time
}

// Repository is the storage the service needs (implemented in ./repository).
type Repository interface {
	Get(ctx context.Context) (Settings, error)
	SetReviewPolicy(ctx context.Context, p ReviewPolicy, by uuid.UUID) (Settings, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// Get returns the settings; any signed-in user may read them (the web needs the policy).
func (s *Service) Get(ctx context.Context) (Settings, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return Settings{}, err
	}
	return s.repo.Get(ctx)
}

// SetReviewPolicy changes the review policy; maintainers only.
func (s *Service) SetReviewPolicy(ctx context.Context, p ReviewPolicy) (Settings, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return Settings{}, err
	}
	if !actor.IsMaintainer() {
		return Settings{}, apperr.Forbidden("only maintainers can change the review policy")
	}
	if !p.Valid() {
		return Settings{}, apperr.Invalid("review_policy must be required or direct")
	}
	return s.repo.SetReviewPolicy(ctx, p, actor.ID)
}
