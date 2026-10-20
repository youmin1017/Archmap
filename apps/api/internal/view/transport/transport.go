// Package transport maps the generated OpenAPI types to the view domain and
// implements the view operations of api.StrictServerInterface.
package transport

import (
	"context"

	"github.com/google/uuid"

	"archmap/api/internal/gen/api"
	modeltransport "archmap/api/internal/model/transport"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/view"
)

type service interface {
	List(ctx context.Context) ([]view.View, error)
	Create(ctx context.Context, in view.Input) (view.View, error)
	Update(ctx context.Context, id uuid.UUID, in view.Input) (view.View, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Model(ctx context.Context, id uuid.UUID, changesetID *uuid.UUID) (view.Detail, error)
	SetLayout(ctx context.Context, id uuid.UUID, layout []view.Element) ([]view.Element, error)
}

type Handler struct {
	svc service
}

func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

func (h *Handler) ListViews(ctx context.Context, _ api.ListViewsRequestObject) (api.ListViewsResponseObject, error) {
	views, err := h.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.View, len(views))
	for i, v := range views {
		out[i] = toView(v)
	}
	return api.ListViews200JSONResponse{Views: out}, nil
}

func (h *Handler) CreateView(ctx context.Context, req api.CreateViewRequestObject) (api.CreateViewResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	v, err := h.svc.Create(ctx, toInput(*req.Body))
	if err != nil {
		return nil, err
	}
	return api.CreateView201JSONResponse(toView(v)), nil
}

func (h *Handler) UpdateView(ctx context.Context, req api.UpdateViewRequestObject) (api.UpdateViewResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	v, err := h.svc.Update(ctx, req.Id, toInput(*req.Body))
	if err != nil {
		return nil, err
	}
	return api.UpdateView200JSONResponse(toView(v)), nil
}

func (h *Handler) DeleteView(ctx context.Context, req api.DeleteViewRequestObject) (api.DeleteViewResponseObject, error) {
	if err := h.svc.Delete(ctx, req.Id); err != nil {
		return nil, err
	}
	return api.DeleteView204Response{}, nil
}

func (h *Handler) GetViewModel(ctx context.Context, req api.GetViewModelRequestObject) (api.GetViewModelResponseObject, error) {
	d, err := h.svc.Model(ctx, req.Id, req.Params.ChangesetId)
	if err != nil {
		return nil, err
	}
	return api.GetViewModel200JSONResponse{
		View:         toView(d.View),
		Elements:     modeltransport.ToElementsWith(d.Result.Elements, d.Result.Effective),
		Relations:    modeltransport.ToRelations(d.Result.Relations),
		Roots:        nonNil(d.Result.Roots),
		Expanded:     nonNil(d.Result.Expanded),
		Layout:       toLayout(d.Layout),
		ScopeMissing: d.Result.ScopeMissing,
	}, nil
}

func (h *Handler) SetViewLayout(ctx context.Context, req api.SetViewLayoutRequestObject) (api.SetViewLayoutResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	layout := make([]view.Element, len(req.Body.Elements))
	for i, e := range req.Body.Elements {
		layout[i] = view.Element{ElementID: e.ElementId, X: e.X, Y: e.Y, W: e.W, H: e.H, Collapsed: e.Collapsed}
	}
	saved, err := h.svc.SetLayout(ctx, req.Id, layout)
	if err != nil {
		return nil, err
	}
	return api.SetViewLayout200JSONResponse{Elements: toLayout(saved)}, nil
}

func toInput(in api.ViewInput) view.Input {
	out := view.Input{Name: in.Name, Kind: view.Kind(in.Kind), ScopeElementID: in.ScopeElementId}
	if in.Description != nil {
		out.Description = *in.Description
	}
	if in.Layers != nil {
		out.Layers = append([]string{}, *in.Layers...)
	}
	if in.Elements != nil {
		out.Members = append([]uuid.UUID{}, *in.Elements...)
	}
	return out
}

func toView(v view.View) api.View {
	return api.View{
		Id: v.ID, Name: v.Name, Kind: api.ViewKind(v.Kind), ScopeElementId: v.ScopeElementID,
		Description: v.Description, Layers: nonNilStrings(v.Layers), CreatedBy: v.CreatedBy,
		CreatedAt: modeltransport.UTC(v.CreatedAt), UpdatedAt: modeltransport.UTC(v.UpdatedAt),
	}
}

func toLayout(els []view.Element) []api.ViewElement {
	out := make([]api.ViewElement, len(els))
	for i, e := range els {
		included := e.Included
		out[i] = api.ViewElement{ElementId: e.ElementID, X: e.X, Y: e.Y, W: e.W, H: e.H, Collapsed: e.Collapsed, Included: &included}
	}
	return out
}

func nonNil(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
