// Package layerrule stores which layers each element class may be placed on.
// The rules are enforced by the changeset service (see model.CheckLayer).
package layerrule

import (
	"context"

	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

// Repository is the storage the service needs (implemented in ./repository).
type Repository interface {
	List(ctx context.Context) ([]model.LayerRule, error)
	// Replace swaps all rules in one transaction.
	Replace(ctx context.Context, rules []model.LayerRule) error
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

// List returns the rules; any authenticated user may read them.
func (s *Service) List(ctx context.Context) ([]model.LayerRule, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return nil, err
	}
	return s.repo.List(ctx)
}

// LayerRules satisfies the changeset service's rule source (no actor check:
// enforcement happens inside an already authorized edit).
func (s *Service) LayerRules(ctx context.Context) ([]model.LayerRule, error) {
	return s.repo.List(ctx)
}

// Replace sets all rules; maintainers only. Existing elements are not touched.
func (s *Service) Replace(ctx context.Context, rules []model.LayerRule) ([]model.LayerRule, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return nil, err
	}
	if !actor.IsMaintainer() {
		return nil, apperr.Forbidden("only maintainers can change layer rules")
	}
	clean, err := model.NormalizeLayerRules(rules)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Replace(ctx, clean); err != nil {
		return nil, err
	}
	return clean, nil
}
