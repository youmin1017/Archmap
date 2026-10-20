// Package user is user management: accounts, roles, local passwords and linked
// sign-in identities (ADR-053, ADR-059, ADR-060). Signing in is internal/auth.
package user

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/auth"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/platform/password"
)

// User is an account as the user-management screen sees it.
type User struct {
	ID            uuid.UUID
	Email         string
	Name          string
	Role          authctx.Role
	CreatedAt     time.Time
	Disabled      bool
	LastLoginAt   *time.Time
	IdentityCount int
}

// Identity is one way a user signs in.
type Identity struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Provider    string
	Issuer      string
	Subject     string
	Email       string
	CreatedAt   time.Time
	LastLoginAt *time.Time
}

// Maintainer is a locked maintainer row with what the break-glass invariant needs.
type Maintainer struct {
	ID       uuid.UUID
	Enabled  bool
	HasLocal bool
}

// LockedUser is a user row read FOR UPDATE.
type LockedUser struct {
	ID       uuid.UUID
	Email    string
	Role     authctx.Role
	Disabled bool
}

// Repository is the storage the service needs (implemented in ./repository).
type Repository interface {
	// InTx runs fn against a transaction-bound Repository.
	InTx(ctx context.Context, fn func(Repository) error) error

	List(ctx context.Context) ([]User, error)
	Get(ctx context.Context, id uuid.UUID) (User, error)
	// LockMaintainers locks every maintainer row; call it first inside InTx (ADR-059).
	LockMaintainers(ctx context.Context) ([]Maintainer, error)
	LockUser(ctx context.Context, id uuid.UUID) (LockedUser, error)
	SetRole(ctx context.Context, id uuid.UUID, role authctx.Role) error
	SetName(ctx context.Context, id uuid.UUID, name string) error
	SetDisabled(ctx context.Context, id uuid.UUID, at *time.Time) error
	DeleteSessions(ctx context.Context, userID uuid.UUID) error
	CreateUser(ctx context.Context, email, name string, role authctx.Role) (uuid.UUID, error)
	// CreateLocalIdentity fails with a Conflict when the subject is taken.
	CreateLocalIdentity(ctx context.Context, userID uuid.UUID, subject, email, hash string, mustChange bool) error
	FindLocalIdentity(ctx context.Context, subject string) (userID uuid.UUID, ok bool, err error)
	LocalIdentity(ctx context.Context, userID uuid.UUID) (uuid.UUID, bool, error)
	SetLocalPassword(ctx context.Context, identityID uuid.UUID, hash string, mustChange bool) error
	Identities(ctx context.Context, userID uuid.UUID) ([]Identity, error)
	// LinkIdentity fails with a Conflict when (issuer, subject) is taken.
	LinkIdentity(ctx context.Context, userID uuid.UUID, provider, issuer, subject string) (Identity, error)
	// LockIdentity returns the issuer of the user's identity, or NotFound.
	LockIdentity(ctx context.Context, userID, identityID uuid.UUID) (string, error)
	DeleteIdentity(ctx context.Context, identityID uuid.UUID) error
}

// ExternalProviders resolves configured external providers (implemented by auth.Service).
type ExternalProviders interface {
	ExternalIssuer(providerID string) (string, bool)
}

type Service struct {
	repo      Repository
	providers ExternalProviders
	now       func() time.Time
}

type Option func(*Service)

func WithProviders(p ExternalProviders) Option { return func(s *Service) { s.providers = p } }

func WithClock(now func() time.Time) Option { return func(s *Service) { s.now = now } }

func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{repo: repo, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

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

// errBreakGlass is returned when a change would leave no enabled maintainer with a local password.
var errBreakGlass = apperr.Conflict("at least one enabled maintainer with a local password must remain; set up another one first")

// breakGlassAfter reports whether an enabled maintainer with a local password
// remains after change is applied to the maintainer with the given id.
func breakGlassAfter(ms []Maintainer, id uuid.UUID, change func(*Maintainer) (keep bool)) bool {
	for _, m := range ms {
		if m.ID == id && !change(&m) {
			continue
		}
		if m.Enabled && m.HasLocal {
			return true
		}
	}
	return false
}

// List returns every user; maintainers only.
func (s *Service) List(ctx context.Context) ([]User, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return nil, err
	}
	return s.repo.List(ctx)
}

// SetRole changes a user's role; maintainers only. Demoting is refused when it would
// break the break-glass invariant.
func (s *Service) SetRole(ctx context.Context, id uuid.UUID, role authctx.Role) (User, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return User{}, err
	}
	if !role.Valid() {
		return User{}, apperr.Invalid("unknown role %q", role)
	}
	err := s.repo.InTx(ctx, func(r Repository) error {
		ms, err := r.LockMaintainers(ctx)
		if err != nil {
			return err
		}
		cur, err := r.LockUser(ctx, id)
		if err != nil {
			return err
		}
		if cur.Role == authctx.RoleMaintainer && role != authctx.RoleMaintainer &&
			!breakGlassAfter(ms, id, func(*Maintainer) bool { return false }) {
			return errBreakGlass
		}
		return r.SetRole(ctx, id, role)
	})
	if err != nil {
		return User{}, err
	}
	return s.repo.Get(ctx, id)
}

// NewUser is the input of Create.
type NewUser struct {
	Name     string
	Email    string
	Role     authctx.Role
	Password string // empty: generate a temporary one
}

// Create adds a user with a local password identity; maintainers only. It returns
// the generated temporary password when none was given.
func (s *Service) Create(ctx context.Context, in NewUser) (User, string, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return User{}, "", err
	}
	name, email := strings.TrimSpace(in.Name), auth.NormalizeLogin(in.Email)
	switch {
	case name == "":
		return User{}, "", apperr.Invalid("name is required")
	case !strings.Contains(email, "@") || strings.ContainsAny(email, " \t"):
		return User{}, "", apperr.Invalid("a valid email is required")
	case !in.Role.Valid():
		return User{}, "", apperr.Invalid("unknown role %q", in.Role)
	}
	pw, temporary, err := chosenPassword(in.Password)
	if err != nil {
		return User{}, "", err
	}
	hash, err := password.Hash(pw)
	if err != nil {
		return User{}, "", err
	}
	var id uuid.UUID
	err = s.repo.InTx(ctx, func(r Repository) error {
		if id, err = r.CreateUser(ctx, email, name, in.Role); err != nil {
			return err
		}
		return r.CreateLocalIdentity(ctx, id, email, email, hash, temporary != "")
	})
	if err != nil {
		return User{}, "", err
	}
	u, err := s.repo.Get(ctx, id)
	return u, temporary, err
}

// chosenPassword validates a maintainer-chosen password or generates a temporary one.
func chosenPassword(pw string) (chosen, temporary string, err error) {
	if pw == "" {
		t := password.Generate()
		return t, t, nil
	}
	if err := password.Validate(pw); err != nil {
		return "", "", apperr.Invalid("%s", err.Error())
	}
	return pw, "", nil
}

// Disable blocks a user from signing in and ends their sessions at once; maintainers only.
func (s *Service) Disable(ctx context.Context, id uuid.UUID) (User, error) {
	actor, err := requireMaintainer(ctx)
	if err != nil {
		return User{}, err
	}
	if actor.ID == id {
		return User{}, apperr.Conflict("you cannot disable your own account")
	}
	err = s.repo.InTx(ctx, func(r Repository) error {
		ms, err := r.LockMaintainers(ctx)
		if err != nil {
			return err
		}
		cur, err := r.LockUser(ctx, id)
		if err != nil {
			return err
		}
		if cur.Disabled {
			return nil
		}
		if cur.Role == authctx.RoleMaintainer &&
			!breakGlassAfter(ms, id, func(m *Maintainer) bool { m.Enabled = false; return true }) {
			return errBreakGlass
		}
		now := s.now()
		if err := r.SetDisabled(ctx, id, &now); err != nil {
			return err
		}
		return r.DeleteSessions(ctx, id)
	})
	if err != nil {
		return User{}, err
	}
	return s.repo.Get(ctx, id)
}

// Enable lets a disabled user sign in again; maintainers only.
func (s *Service) Enable(ctx context.Context, id uuid.UUID) (User, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return User{}, err
	}
	err := s.repo.InTx(ctx, func(r Repository) error {
		if _, err := r.LockUser(ctx, id); err != nil {
			return err
		}
		return r.SetDisabled(ctx, id, nil)
	})
	if err != nil {
		return User{}, err
	}
	return s.repo.Get(ctx, id)
}

// ResetPassword sets a new local password (creating the local identity from the
// user's email when missing), marks it must-change and ends the user's sessions;
// maintainers only. It returns the generated password when none was given.
func (s *Service) ResetPassword(ctx context.Context, id uuid.UUID, pw string) (string, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return "", err
	}
	pw, temporary, err := chosenPassword(pw)
	if err != nil {
		return "", err
	}
	hash, err := password.Hash(pw)
	if err != nil {
		return "", err
	}
	err = s.repo.InTx(ctx, func(r Repository) error {
		u, err := r.LockUser(ctx, id)
		if err != nil {
			return err
		}
		identityID, ok, err := r.LocalIdentity(ctx, id)
		if err != nil {
			return err
		}
		if ok {
			err = r.SetLocalPassword(ctx, identityID, hash, true)
		} else {
			subject := auth.NormalizeLogin(u.Email)
			if subject == "" {
				return apperr.Invalid("the user has no email to sign in with")
			}
			err = r.CreateLocalIdentity(ctx, id, subject, u.Email, hash, true)
		}
		if err != nil {
			return err
		}
		return r.DeleteSessions(ctx, id)
	})
	return temporary, err
}

// Identities lists a user's sign-in identities; maintainers only.
func (s *Service) Identities(ctx context.Context, id uuid.UUID) ([]Identity, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return nil, err
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.Identities(ctx, id)
}

// Link attaches an external identity to a user. This is the only way an existing
// account gains an external identity: nothing is linked by email (ADR-059).
func (s *Service) Link(ctx context.Context, id uuid.UUID, providerID, subject string) (Identity, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return Identity{}, err
	}
	issuer, ok := "", false
	if s.providers != nil {
		issuer, ok = s.providers.ExternalIssuer(providerID)
	}
	if !ok {
		return Identity{}, apperr.Invalid("%q is not a configured external sign-in provider", providerID)
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return Identity{}, apperr.Invalid("subject is required")
	}
	var out Identity
	err := s.repo.InTx(ctx, func(r Repository) error {
		if _, err := r.LockUser(ctx, id); err != nil {
			return err
		}
		var err error
		out, err = r.LinkIdentity(ctx, id, providerID, issuer, subject)
		return err
	})
	return out, err
}

// Unlink removes an identity; sessions created with it end (FK cascade). Removing the
// last local password of the last break-glass maintainer is refused.
func (s *Service) Unlink(ctx context.Context, id, identityID uuid.UUID) error {
	if _, err := requireMaintainer(ctx); err != nil {
		return err
	}
	return s.repo.InTx(ctx, func(r Repository) error {
		ms, err := r.LockMaintainers(ctx)
		if err != nil {
			return err
		}
		u, err := r.LockUser(ctx, id)
		if err != nil {
			return err
		}
		issuer, err := r.LockIdentity(ctx, id, identityID)
		if err != nil {
			return err
		}
		if issuer == auth.LocalIssuer && u.Role == authctx.RoleMaintainer &&
			!breakGlassAfter(ms, id, func(m *Maintainer) bool { m.HasLocal = false; return true }) {
			return errBreakGlass
		}
		return r.DeleteIdentity(ctx, identityID)
	})
}

// LocalAccount is the input of EnsureLocal.
type LocalAccount struct {
	Email    string
	Name     string
	Role     authctx.Role
	Password string
}

// EnsureLocal creates or updates an account that signs in with a local password
// (name, role and password are set; the account is enabled). It performs no
// actor check: only trusted callers use it (cmd/seed, cmd/admin, tests).
func (s *Service) EnsureLocal(ctx context.Context, a LocalAccount) (uuid.UUID, error) {
	email := auth.NormalizeLogin(a.Email)
	if email == "" || !a.Role.Valid() || strings.TrimSpace(a.Name) == "" {
		return uuid.Nil, apperr.Invalid("email, name and a valid role are required")
	}
	hash, err := password.Hash(a.Password)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = s.repo.InTx(ctx, func(r Repository) error {
		var found bool
		id, found, err = r.FindLocalIdentity(ctx, email)
		if err != nil {
			return err
		}
		if !found {
			if id, err = r.CreateUser(ctx, email, a.Name, a.Role); err != nil {
				return err
			}
			return r.CreateLocalIdentity(ctx, id, email, email, hash, false)
		}
		identityID, _, err := r.LocalIdentity(ctx, id)
		if err != nil {
			return err
		}
		if err := r.SetLocalPassword(ctx, identityID, hash, false); err != nil {
			return err
		}
		if err := r.SetName(ctx, id, a.Name); err != nil {
			return err
		}
		if err := r.SetRole(ctx, id, a.Role); err != nil {
			return err
		}
		return r.SetDisabled(ctx, id, nil)
	})
	return id, err
}
