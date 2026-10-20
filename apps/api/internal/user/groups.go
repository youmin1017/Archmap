package user

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

// Group is an app-managed set of users (ADR-070). Groups are account
// administration: maintainers change them directly; they are not versioned.
type Group struct {
	ID          uuid.UUID
	Name        string
	Description string
	CreatedAt   time.Time
	Members     []uuid.UUID
}

// DirectoryUser is what every signed-in user may know about another user (to pick owners).
type DirectoryUser struct {
	ID       uuid.UUID
	Name     string
	Email    string
	Disabled bool
}

// Directory lists the users and groups that can be picked as owners.
type Directory struct {
	Users  []DirectoryUser
	Groups []Group
}

// GroupRepository is the group storage the service needs (implemented in ./repository).
type GroupRepository interface {
	ListGroups(ctx context.Context) ([]Group, error)
	DirectoryUsers(ctx context.Context) ([]DirectoryUser, error)
	// CreateGroup fails with a Conflict when the name is taken.
	CreateGroup(ctx context.Context, name, description string) (uuid.UUID, error)
	// UpdateGroup fails with NotFound for unknown or deleted groups and Conflict for a taken name.
	UpdateGroup(ctx context.Context, id uuid.UUID, name, description string) error
	DeleteGroup(ctx context.Context, id uuid.UUID) error
	// SetGroupMembers replaces the members; unknown users are Invalid.
	SetGroupMembers(ctx context.Context, id uuid.UUID, users []uuid.UUID) error
}

const (
	maxGroupName        = 120
	maxGroupDescription = 500
)

// GroupInput is the name and description of a group.
type GroupInput struct {
	Name        string
	Description string
}

func (in GroupInput) normalized() (GroupInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	switch {
	case in.Name == "" || len([]rune(in.Name)) > maxGroupName || strings.ContainsAny(in.Name, "\r\n"):
		return in, apperr.Invalid("group name must be 1-%d characters on a single line", maxGroupName)
	case len([]rune(in.Description)) > maxGroupDescription:
		return in, apperr.Invalid("group description must be at most %d characters", maxGroupDescription)
	}
	return in, nil
}

// GroupService manages groups and serves the owner directory.
type GroupService struct{ repo GroupRepository }

func NewGroupService(repo GroupRepository) *GroupService { return &GroupService{repo: repo} }

// Groups lists the live groups with their members; any signed-in user.
func (s *GroupService) Groups(ctx context.Context) ([]Group, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return nil, err
	}
	return s.repo.ListGroups(ctx)
}

// Directory lists users (name, email, disabled; no roles) and groups; any signed-in user.
func (s *GroupService) Directory(ctx context.Context) (Directory, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return Directory{}, err
	}
	users, err := s.repo.DirectoryUsers(ctx)
	if err != nil {
		return Directory{}, err
	}
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return Directory{}, err
	}
	return Directory{Users: users, Groups: groups}, nil
}

func (s *GroupService) CreateGroup(ctx context.Context, in GroupInput) (Group, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return Group{}, err
	}
	in, err := in.normalized()
	if err != nil {
		return Group{}, err
	}
	id, err := s.repo.CreateGroup(ctx, in.Name, in.Description)
	if err != nil {
		return Group{}, err
	}
	return s.group(ctx, id)
}

func (s *GroupService) UpdateGroup(ctx context.Context, id uuid.UUID, in GroupInput) (Group, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return Group{}, err
	}
	in, err := in.normalized()
	if err != nil {
		return Group{}, err
	}
	if err := s.repo.UpdateGroup(ctx, id, in.Name, in.Description); err != nil {
		return Group{}, err
	}
	return s.group(ctx, id)
}

// DeleteGroup soft-deletes a group; owners pointing at it keep their name and lint warns.
func (s *GroupService) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	if _, err := requireMaintainer(ctx); err != nil {
		return err
	}
	return s.repo.DeleteGroup(ctx, id)
}

// SetGroupMembers replaces a group's members (duplicates are ignored).
func (s *GroupService) SetGroupMembers(ctx context.Context, id uuid.UUID, users []uuid.UUID) (Group, error) {
	if _, err := requireMaintainer(ctx); err != nil {
		return Group{}, err
	}
	unique := slices.Clone(users)
	slices.SortFunc(unique, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	unique = slices.Compact(unique)
	if err := s.repo.SetGroupMembers(ctx, id, unique); err != nil {
		return Group{}, err
	}
	return s.group(ctx, id)
}

func (s *GroupService) group(ctx context.Context, id uuid.UUID) (Group, error) {
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return Group{}, err
	}
	for _, g := range groups {
		if g.ID == id {
			return g, nil
		}
	}
	return Group{}, apperr.NotFound("group %s not found", id)
}
