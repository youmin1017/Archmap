// Package transport maps the generated OpenAPI types to the model domain and
// implements the read-only main-model operations of api.StrictServerInterface.
package transport

import (
	"context"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/gen/api"
	"archmap/api/internal/model"
)

type reader interface {
	ListElements(ctx context.Context, f model.ElementFilter) ([]model.Element, error)
	GetElement(ctx context.Context, id uuid.UUID) (model.Element, error)
	ListRelations(ctx context.Context, f model.RelationFilter) ([]model.Relation, error)
	GetRelation(ctx context.Context, id uuid.UUID) (model.Relation, error)
}

type Handler struct {
	svc reader
}

func NewHandler(svc reader) *Handler { return &Handler{svc: svc} }

func (h *Handler) ListElements(ctx context.Context, req api.ListElementsRequestObject) (api.ListElementsResponseObject, error) {
	f := model.ElementFilter{ParentID: req.Params.ParentId}
	if req.Params.Q != nil {
		f.Query = *req.Params.Q
	}
	els, err := h.svc.ListElements(ctx, f)
	if err != nil {
		return nil, err
	}
	return api.ListElements200JSONResponse{Elements: ToElements(els)}, nil
}

func (h *Handler) GetElement(ctx context.Context, req api.GetElementRequestObject) (api.GetElementResponseObject, error) {
	e, err := h.svc.GetElement(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.GetElement200JSONResponse(ToElement(e)), nil
}

func (h *Handler) ListRelations(ctx context.Context, req api.ListRelationsRequestObject) (api.ListRelationsResponseObject, error) {
	rels, err := h.svc.ListRelations(ctx, model.RelationFilter{SourceID: req.Params.SourceId, TargetID: req.Params.TargetId})
	if err != nil {
		return nil, err
	}
	return api.ListRelations200JSONResponse{Relations: ToRelations(rels)}, nil
}

func (h *Handler) GetRelation(ctx context.Context, req api.GetRelationRequestObject) (api.GetRelationResponseObject, error) {
	r, err := h.svc.GetRelation(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.GetRelation200JSONResponse(ToRelation(r)), nil
}

func ToElement(e model.Element) api.Element {
	tags := e.Tags
	if tags == nil {
		tags = []string{}
	}
	return api.Element{
		Id: e.ID, ParentId: e.ParentID, Kind: api.ElementKind(e.Kind), Name: e.Name,
		Description: e.Description, Technology: e.Technology, Tags: tags, Protected: e.Protected, Layer: e.Layer, Projects: model.NormalizeProjects(e.Projects),
		Replicas: max(e.Replicas, 1), Properties: model.NormalizeProperties(e.Properties),
		Version: e.Version, CreatedAt: UTC(e.CreatedAt), UpdatedAt: UTC(e.UpdatedAt),
	}
}

func ToElements(els []model.Element) []api.Element {
	out := make([]api.Element, len(els))
	for i, e := range els {
		out[i] = ToElement(e)
	}
	return out
}

func ToRelation(r model.Relation) api.Relation {
	return api.Relation{
		Id: r.ID, SourceId: r.SourceID, TargetId: r.TargetID, Kind: api.RelationKind(r.Kind),
		Description: r.Description, Technology: r.Technology,
		Version: r.Version, CreatedAt: UTC(r.CreatedAt), UpdatedAt: UTC(r.UpdatedAt),
	}
}

func ToRelations(rels []model.Relation) []api.Relation {
	out := make([]api.Relation, len(rels))
	for i, r := range rels {
		out[i] = ToRelation(r)
	}
	return out
}

func ToModel(s model.Snapshot) api.Model {
	return api.Model{Elements: ToElements(s.Elements), Relations: ToRelations(s.Relations)}
}

// UTC normalizes timestamps for the API: the contract (and its generated Zod
// validators) expects RFC 3339 date-times with a Z suffix, not local offsets.
func UTC(t time.Time) time.Time { return t.UTC() }

// UTCPtr is UTC for optional timestamps.
func UTCPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
