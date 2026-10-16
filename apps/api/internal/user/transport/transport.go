// Package transport implements the /me and user-management operations of api.StrictServerInterface.
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

func (*Handler) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	a, err := authctx.Require(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetMe200JSONResponse{Id: a.ID, Email: a.Email, Name: a.Name, Role: api.Role(a.Role)}, nil
}

func toUser(u user.User) api.User {
	return api.User{Id: u.ID, Email: u.Email, Name: u.Name, Role: api.Role(u.Role), CreatedAt: u.CreatedAt.UTC()}
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
