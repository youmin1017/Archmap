// Package transport implements the user-management operations of api.StrictServerInterface.
package transport

import (
	"context"

	"archmap/api/internal/gen/api"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/user"
)

type Handler struct{ svc *user.Service }

func NewHandler(svc *user.Service) *Handler { return &Handler{svc: svc} }

func toUser(u user.User) api.User {
	out := api.User{
		Id: u.ID, Email: u.Email, Name: u.Name, Role: api.Role(u.Role), CreatedAt: u.CreatedAt.UTC(),
		Disabled: u.Disabled, IdentityCount: u.IdentityCount,
	}
	if u.LastLoginAt != nil {
		t := u.LastLoginAt.UTC()
		out.LastLoginAt = &t
	}
	return out
}

func toIdentity(i user.Identity) api.Identity {
	out := api.Identity{
		Id: i.ID, Provider: i.Provider, Issuer: i.Issuer, Subject: i.Subject, Email: i.Email,
		CreatedAt: i.CreatedAt.UTC(),
	}
	if i.LastLoginAt != nil {
		t := i.LastLoginAt.UTC()
		out.LastLoginAt = &t
	}
	return out
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (h *Handler) ListUsers(ctx context.Context, _ api.ListUsersRequestObject) (api.ListUsersResponseObject, error) {
	users, err := h.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.User, len(users))
	for i, u := range users {
		out[i] = toUser(u)
	}
	return api.ListUsers200JSONResponse(api.UserList{Users: out}), nil
}

func (h *Handler) CreateUser(ctx context.Context, req api.CreateUserRequestObject) (api.CreateUserResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	u, temporary, err := h.svc.Create(ctx, user.NewUser{
		Name: req.Body.Name, Email: req.Body.Email, Role: authctx.Role(req.Body.Role), Password: deref(req.Body.Password),
	})
	if err != nil {
		return nil, err
	}
	return api.CreateUser201JSONResponse(api.CreatedUser{User: toUser(u), TemporaryPassword: optional(temporary)}), nil
}

func (h *Handler) SetUserRole(ctx context.Context, req api.SetUserRoleRequestObject) (api.SetUserRoleResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	u, err := h.svc.SetRole(ctx, req.Id, authctx.Role(req.Body.Role))
	if err != nil {
		return nil, err
	}
	return api.SetUserRole200JSONResponse(toUser(u)), nil
}

func (h *Handler) DisableUser(ctx context.Context, req api.DisableUserRequestObject) (api.DisableUserResponseObject, error) {
	u, err := h.svc.Disable(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.DisableUser200JSONResponse(toUser(u)), nil
}

func (h *Handler) EnableUser(ctx context.Context, req api.EnableUserRequestObject) (api.EnableUserResponseObject, error) {
	u, err := h.svc.Enable(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.EnableUser200JSONResponse(toUser(u)), nil
}

func (h *Handler) ResetUserPassword(ctx context.Context, req api.ResetUserPasswordRequestObject) (api.ResetUserPasswordResponseObject, error) {
	var pw string
	if req.Body != nil {
		pw = deref(req.Body.Password)
	}
	temporary, err := h.svc.ResetPassword(ctx, req.Id, pw)
	if err != nil {
		return nil, err
	}
	return api.ResetUserPassword200JSONResponse(api.PasswordResetResult{TemporaryPassword: optional(temporary)}), nil
}

func (h *Handler) ListUserIdentities(ctx context.Context, req api.ListUserIdentitiesRequestObject) (api.ListUserIdentitiesResponseObject, error) {
	ids, err := h.svc.Identities(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out := make([]api.Identity, len(ids))
	for i, x := range ids {
		out[i] = toIdentity(x)
	}
	return api.ListUserIdentities200JSONResponse(api.IdentityList{Identities: out}), nil
}

func (h *Handler) LinkUserIdentity(ctx context.Context, req api.LinkUserIdentityRequestObject) (api.LinkUserIdentityResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	x, err := h.svc.Link(ctx, req.Id, req.Body.Provider, req.Body.Subject)
	if err != nil {
		return nil, err
	}
	return api.LinkUserIdentity201JSONResponse(toIdentity(x)), nil
}

func (h *Handler) UnlinkUserIdentity(ctx context.Context, req api.UnlinkUserIdentityRequestObject) (api.UnlinkUserIdentityResponseObject, error) {
	if err := h.svc.Unlink(ctx, req.Id, req.IdentityId); err != nil {
		return nil, err
	}
	return api.UnlinkUserIdentity204Response{}, nil
}
