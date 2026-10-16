// Package transport maps the generated OpenAPI types to layer rules and
// implements the layer-rule operations of api.StrictServerInterface.
package transport

import (
	"context"

	"archmap/api/internal/gen/api"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
)

type service interface {
	List(ctx context.Context) ([]model.LayerRule, error)
	Replace(ctx context.Context, rules []model.LayerRule) ([]model.LayerRule, error)
}

type Handler struct{ svc service }

func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

func (h *Handler) ListLayerRules(ctx context.Context, _ api.ListLayerRulesRequestObject) (api.ListLayerRulesResponseObject, error) {
	rules, err := h.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	return api.ListLayerRules200JSONResponse(toRules(rules)), nil
}

func (h *Handler) ReplaceLayerRules(ctx context.Context, req api.ReplaceLayerRulesRequestObject) (api.ReplaceLayerRulesResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	in := make([]model.LayerRule, len(req.Body.Rules))
	for i, r := range req.Body.Rules {
		in[i] = model.LayerRule{Class: r.ElementClass, Layers: r.Layers}
	}
	rules, err := h.svc.Replace(ctx, in)
	if err != nil {
		return nil, err
	}
	return api.ReplaceLayerRules200JSONResponse(toRules(rules)), nil
}

func toRules(rules []model.LayerRule) api.LayerRules {
	out := make([]api.LayerRule, len(rules))
	for i, r := range rules {
		layers := r.Layers
		if layers == nil {
			layers = []string{}
		}
		out[i] = api.LayerRule{ElementClass: r.Class, Layers: layers}
	}
	return api.LayerRules{Rules: out}
}
