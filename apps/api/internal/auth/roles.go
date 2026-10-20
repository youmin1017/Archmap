package auth

import (
	"context"

	"archmap/api/internal/platform/authctx"
)

// RoleResolver decides a user's role when they sign in (ADR-058). It is the
// extension point for mapping identity-provider claims / groups to roles (M15).
// A resolver that returns a role different from the stored one has it saved;
// such a mapper must respect the break-glass maintainer invariant (ADR-059).
type RoleResolver interface {
	// Default is the role of an account provisioned from an unknown identity.
	Default() authctx.Role
	// Resolve returns the role for a user whose stored role is stored, signing in as id.
	Resolve(ctx context.Context, stored authctx.Role, id Identity) (authctx.Role, error)
}

// StoredRoles keeps roles on the user and ignores every claim, so nothing a
// provider asserts can raise a role.
type StoredRoles struct{}

func (StoredRoles) Default() authctx.Role { return authctx.RoleViewer }

func (StoredRoles) Resolve(_ context.Context, stored authctx.Role, _ Identity) (authctx.Role, error) {
	return stored, nil
}
