// Package transport maps the generated OpenAPI types to settings and implements
// the settings operations of api.StrictServerInterface.
package transport

import (
	"context"

	"archmap/api/internal/gen/api"
	modeltransport "archmap/api/internal/model/transport"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/settings"
)

type service interface {
	Get(ctx context.Context) (settings.Settings, error)
	SetReviewPolicy(ctx context.Context, p settings.ReviewPolicy) (settings.Settings, error)
}

type Handler struct{ svc service }

func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

func (h *Handler) GetSettings(ctx context.Context, _ api.GetSettingsRequestObject) (api.GetSettingsResponseObject, error) {
	s, err := h.svc.Get(ctx)
	if err != nil {
		return nil, err
	}
	return api.GetSettings200JSONResponse(toSettings(s)), nil
}

func (h *Handler) UpdateSettings(ctx context.Context, req api.UpdateSettingsRequestObject) (api.UpdateSettingsResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	s, err := h.svc.SetReviewPolicy(ctx, settings.ReviewPolicy(req.Body.ReviewPolicy))
	if err != nil {
		return nil, err
	}
	return api.UpdateSettings200JSONResponse(toSettings(s)), nil
}

func toSettings(s settings.Settings) api.Settings {
	return api.Settings{ReviewPolicy: api.ReviewPolicy(s.ReviewPolicy), UpdatedBy: s.UpdatedBy, UpdatedAt: modeltransport.UTC(s.UpdatedAt)}
}
