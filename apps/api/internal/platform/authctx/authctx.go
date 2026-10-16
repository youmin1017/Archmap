// Package authctx carries the authenticated actor through context.Context so
// services can enforce role rules without knowing how the actor was
// established (session cookie, dev header, OIDC later).
package authctx

import (
	"context"

	"github.com/google/uuid"

	"archmap/api/internal/platform/apperr"
)

type Role string

const (
	RoleViewer     Role = "viewer"
	RoleEditor     Role = "editor"
	RoleMaintainer Role = "maintainer"
)

func (r Role) Valid() bool {
	return r == RoleViewer || r == RoleEditor || r == RoleMaintainer
}

type Actor struct {
	ID    uuid.UUID
	Email string
	Name  string
	Role  Role
}

// CanEdit reports whether the actor may create and edit drafts.
func (a Actor) CanEdit() bool { return a.Role == RoleEditor || a.Role == RoleMaintainer }

func (a Actor) IsMaintainer() bool { return a.Role == RoleMaintainer }

type ctxKey struct{}

func With(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

func From(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(ctxKey{}).(Actor)
	return a, ok
}

// Require returns the actor or an Unauthenticated error.
func Require(ctx context.Context) (Actor, error) {
	a, ok := From(ctx)
	if !ok {
		return Actor{}, apperr.Unauthenticated("authentication required")
	}
	return a, nil
}
