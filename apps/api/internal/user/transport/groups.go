package transport

import (
	"context"

	"github.com/google/uuid"

	"archmap/api/internal/gen/api"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/user"
)

// GroupHandler implements the group and directory operations.
type GroupHandler struct{ svc *user.GroupService }

func NewGroupHandler(svc *user.GroupService) *GroupHandler { return &GroupHandler{svc: svc} }

func toGroup(g user.Group) api.Group {
	members := g.Members
	if members == nil {
		members = []uuid.UUID{}
	}
	return api.Group{Id: g.ID, Name: g.Name, Description: g.Description, CreatedAt: g.CreatedAt.UTC(), Members: members}
}

func toGroups(gs []user.Group) []api.Group {
	out := make([]api.Group, len(gs))
	for i, g := range gs {
		out[i] = toGroup(g)
	}
	return out
}

func groupInput(name string, description *string) user.GroupInput {
	in := user.GroupInput{Name: name}
	if description != nil {
		in.Description = *description
	}
	return in
}

func (h *GroupHandler) GetDirectory(ctx context.Context, _ api.GetDirectoryRequestObject) (api.GetDirectoryResponseObject, error) {
	d, err := h.svc.Directory(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]api.DirectoryUser, len(d.Users))
	for i, u := range d.Users {
		users[i] = api.DirectoryUser{Id: u.ID, Name: u.Name, Email: u.Email, Disabled: u.Disabled}
	}
	return api.GetDirectory200JSONResponse{Users: users, Groups: toGroups(d.Groups)}, nil
}

func (h *GroupHandler) ListGroups(ctx context.Context, _ api.ListGroupsRequestObject) (api.ListGroupsResponseObject, error) {
	gs, err := h.svc.Groups(ctx)
	if err != nil {
		return nil, err
	}
	return api.ListGroups200JSONResponse{Groups: toGroups(gs)}, nil
}

func (h *GroupHandler) CreateGroup(ctx context.Context, req api.CreateGroupRequestObject) (api.CreateGroupResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	g, err := h.svc.CreateGroup(ctx, groupInput(req.Body.Name, req.Body.Description))
	if err != nil {
		return nil, err
	}
	return api.CreateGroup201JSONResponse(toGroup(g)), nil
}

func (h *GroupHandler) UpdateGroup(ctx context.Context, req api.UpdateGroupRequestObject) (api.UpdateGroupResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	g, err := h.svc.UpdateGroup(ctx, req.Id, groupInput(req.Body.Name, req.Body.Description))
	if err != nil {
		return nil, err
	}
	return api.UpdateGroup200JSONResponse(toGroup(g)), nil
}

func (h *GroupHandler) DeleteGroup(ctx context.Context, req api.DeleteGroupRequestObject) (api.DeleteGroupResponseObject, error) {
	if err := h.svc.DeleteGroup(ctx, req.Id); err != nil {
		return nil, err
	}
	return api.DeleteGroup204Response{}, nil
}

func (h *GroupHandler) SetGroupMembers(ctx context.Context, req api.SetGroupMembersRequestObject) (api.SetGroupMembersResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	g, err := h.svc.SetGroupMembers(ctx, req.Id, req.Body.UserIds)
	if err != nil {
		return nil, err
	}
	return api.SetGroupMembers200JSONResponse(toGroup(g)), nil
}
