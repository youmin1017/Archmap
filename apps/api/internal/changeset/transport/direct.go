package transport

import (
	"context"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/gen/api"
	"archmap/api/internal/platform/apperr"
)

type directService interface {
	ApplyDirect(ctx context.Context, summary string, inputs []changeset.ChangeInput) (changeset.DirectResult, error)
	RevertDirect(ctx context.Context, id uuid.UUID) (changeset.DirectResult, error)
}

// ApplyDirect edits main directly under the "direct" review policy (ADR-072).
func (h *Handler) ApplyDirect(ctx context.Context, req api.ApplyDirectRequestObject) (api.ApplyDirectResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	inputs := make([]changeset.ChangeInput, len(req.Body.Changes))
	for i, in := range req.Body.Changes {
		ci, err := toInput(in)
		if err != nil {
			return nil, apperr.Invalid("change %d: %v", i, err)
		}
		inputs[i] = ci
	}
	summary := ""
	if req.Body.Summary != nil {
		summary = *req.Body.Summary
	}
	res, err := h.svc.ApplyDirect(ctx, summary, inputs)
	if err != nil {
		return nil, err
	}
	return api.ApplyDirect201JSONResponse(toDirectResult(res)), nil
}

// RevertDirect undoes a direct edit with a new version that reverses it.
func (h *Handler) RevertDirect(ctx context.Context, req api.RevertDirectRequestObject) (api.RevertDirectResponseObject, error) {
	res, err := h.svc.RevertDirect(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.RevertDirect201JSONResponse(toDirectResult(res)), nil
}

func toDirectResult(r changeset.DirectResult) api.DirectResult {
	return api.DirectResult{
		Outcome: api.DirectOutcome(r.Outcome), Changeset: toChangeset(r.Changeset), Version: r.Version, Reason: r.Reason,
	}
}
