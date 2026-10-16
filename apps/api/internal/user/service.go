package user

import (
	"context"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

// User is an account as the user-management screen sees it.
type User struct {
	ID        uuid.UUID
	Email     string
	Name      string
	Role      authctx.Role
	CreatedAt time.Time
}

// Repository is the storage the service needs (implemented in ./repository).
type Repository interface {
	List(ctx context.Context) ([]User, error)
	// ChangeRole sets a user's role in one transaction with the maintainers locked.
	// check sees the current role and the number of maintainers and may veto the change.
	ChangeRole(ctx context.Context, id uuid.UUID, role authctx.Role, check func(current authctx.Role, maintainers int) error) (User, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func requireMaintainer(ctx context.Context) (authctx.Actor, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return authctx.Actor{}, err
	}
	if !actor.IsMaintainer() {
		return authctx.Actor{}, apperr.Forbidden("only maintainers can manage users")
	}
	return actor, nil
}

// List returns every user; maintainers only.
func (s *Service) List(ctx context.Context) ([]User, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return nil, err
	}
	return s.repo.List(ctx)
}

// SetRole changes a user's role; maintainers only. The last maintainer cannot be
// demoted, so the model can always be administered.
func (s *Service) SetRole(ctx context.Context, id uuid.UUID, role authctx.Role) (User, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return User{}, err
	}
	if !role.Valid() {
		return User{}, apperr.Invalid("unknown role %q", role)
	}
	return s.repo.ChangeRole(ctx, id, role, func(current authctx.Role, maintainers int) error {
		if current == authctx.RoleMaintainer && role != authctx.RoleMaintainer && maintainers <= 1 {
			return apperr.Conflict("at least one maintainer is required; promote someone else first")
		}
		return nil
	})
}
