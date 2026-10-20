// Package transport maps the generated OpenAPI types to the model domain and
// implements the read-only main-model operations of api.StrictServerInterface.
package transport

import (
	"context"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"archmap/api/internal/gen/api"
	"archmap/api/internal/model"
)

type reader interface {
	ListElements(ctx context.Context, f model.ElementFilter) ([]model.Element, error)
	GetElement(ctx context.Context, id uuid.UUID) (model.Element, error)
	ListRelations(ctx context.Context, f model.RelationFilter) ([]model.Relation, error)
	GetRelation(ctx context.Context, id uuid.UUID) (model.Relation, error)
	Snapshot(ctx context.Context) (model.Snapshot, error)
	Effective(ctx context.Context) (map[uuid.UUID]model.Effective, error)
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
	eff, err := h.svc.Effective(ctx)
	if err != nil {
		return nil, err
	}
	return api.ListElements200JSONResponse{Elements: ToElementsWith(els, eff)}, nil
}

func (h *Handler) GetElement(ctx context.Context, req api.GetElementRequestObject) (api.GetElementResponseObject, error) {
	e, err := h.svc.GetElement(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	eff, err := h.svc.Effective(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetElement200JSONResponse(ToElementsWith([]model.Element{e}, eff)[0]), nil
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

func (h *Handler) ListOwners(ctx context.Context, _ api.ListOwnersRequestObject) (api.ListOwnersResponseObject, error) {
	s, err := h.svc.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return api.ListOwners200JSONResponse{Owners: ToOwners(s.Owners), Ownerships: ToOwnerships(s.Ownerships)}, nil
}

// ToElement maps an element without inherited values (diffs, where they do not apply).
func ToElement(e model.Element) api.Element {
	e = e.Normalized()
	tags := e.Tags
	if tags == nil {
		tags = []string{}
	}
	out := api.Element{
		Id: e.ID, ParentId: e.ParentID, Kind: api.ElementKind(e.Kind), Name: e.Name,
		Description: e.Description, Technology: e.Technology, Tags: tags, Protected: e.Protected, Layer: e.Layer, Projects: model.NormalizeProjects(e.Projects),
		Replicas: max(e.Replicas, 1), Properties: model.NormalizeProperties(e.Properties),
		Type: api.ElementType(e.Type), Environment: api.Environment(e.Environment), Lifecycle: api.Lifecycle(e.Lifecycle),
		Criticality: api.Criticality(e.Criticality), RunbookUrl: e.RunbookURL, DocsUrl: e.DocsURL,
		Version: e.Version, CreatedAt: UTC(e.CreatedAt), UpdatedAt: UTC(e.UpdatedAt),
	}
	if t, ok := e.ReviewedAt(); ok {
		out.LastReviewedAt = &openapi_types.Date{Time: t}
	}
	return out
}

// ToElementsWith maps elements with their inherited values (model reads).
func ToElementsWith(els []model.Element, eff map[uuid.UUID]model.Effective) []api.Element {
	out := make([]api.Element, len(els))
	for i, e := range els {
		out[i] = ToElement(e)
		if ef, ok := eff[e.ID]; ok {
			x := toEffective(ef)
			out[i].Effective = &x
		}
	}
	return out
}

func toEffective(ef model.Effective) api.ElementEffective {
	owners := make([]api.EffectiveOwner, 0, len(ef.Owners))
	for _, role := range model.OwnerRoles {
		if o, ok := ef.Owners[role]; ok {
			owners = append(owners, api.EffectiveOwner{Role: api.OwnerRole(role), OwnerId: o.OwnerID, SourceElementId: o.Source})
		}
	}
	return api.ElementEffective{
		Environment: api.Environment(ef.Environment), EnvironmentSourceId: ef.EnvironmentSource,
		Owners: owners, DeployedViaId: ef.DeployedVia,
	}
}

func ToOwners(os []model.Owner) []api.Owner {
	out := make([]api.Owner, len(os))
	for i, o := range os {
		out[i] = ToOwner(o)
	}
	return out
}

func ToOwner(o model.Owner) api.Owner {
	return api.Owner{
		Id: o.ID, Kind: api.OwnerKind(o.Kind), Name: o.Name, Contact: o.Contact, OncallUrl: o.OncallURL,
		PrincipalId: o.PrincipalID, Version: o.Version,
	}
}

func ToOwnerships(os []model.Ownership) []api.Ownership {
	out := make([]api.Ownership, len(os))
	for i, o := range os {
		out[i] = ToOwnership(o)
	}
	return out
}

func ToOwnership(o model.Ownership) api.Ownership {
	return api.Ownership{Id: o.ID, ElementId: o.ElementID, OwnerId: o.OwnerID, Role: api.OwnerRole(o.Role), Version: o.Version}
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

// ToModel maps a whole snapshot, with inherited values computed from it.
func ToModel(s model.Snapshot) api.Model {
	return api.Model{
		Elements: ToElementsWith(s.Elements, model.ComputeEffective(s)), Relations: ToRelations(s.Relations),
		Owners: ToOwners(s.Owners), Ownerships: ToOwnerships(s.Ownerships),
	}
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
