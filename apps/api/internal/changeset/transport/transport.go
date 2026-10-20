// Package transport maps the generated OpenAPI types to the changeset domain
// and implements the changeset operations of api.StrictServerInterface.
package transport

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/gen/api"
	"archmap/api/internal/lint"
	"archmap/api/internal/model"
	modeltransport "archmap/api/internal/model/transport"
	"archmap/api/internal/platform/apperr"
)

type service interface {
	directService
	Create(ctx context.Context, title, description string) (changeset.Changeset, error)
	List(ctx context.Context, f changeset.Filter) ([]changeset.Changeset, error)
	Get(ctx context.Context, id uuid.UUID) (changeset.Detail, error)
	EffectiveModel(ctx context.Context, id uuid.UUID) (model.Snapshot, error)
	AddChanges(ctx context.Context, id uuid.UUID, inputs []changeset.ChangeInput) ([]changeset.Change, error)
	Undo(ctx context.Context, id uuid.UUID) ([]changeset.Change, error)
	Redo(ctx context.Context, id uuid.UUID) ([]changeset.Change, error)

	Diff(ctx context.Context, id uuid.UUID) (changeset.DiffResult, error)
	Lint(ctx context.Context, id uuid.UUID) (lint.Report, error)
	Conflicts(ctx context.Context, id uuid.UUID) ([]changeset.Conflict, error)
	Resolve(ctx context.Context, id uuid.UUID, rs []changeset.Resolution) ([]changeset.Conflict, error)
	Submit(ctx context.Context, id uuid.UUID) (changeset.Changeset, error)
	Approve(ctx context.Context, id uuid.UUID) (changeset.Changeset, error)
	Reject(ctx context.Context, id uuid.UUID) (changeset.Changeset, error)
	Abandon(ctx context.Context, id uuid.UUID) (changeset.Changeset, error)
	Merge(ctx context.Context, id uuid.UUID) (changeset.MergeResult, error)
	Versions(ctx context.Context) ([]changeset.Version, error)
	ModelAt(ctx context.Context, version int) (model.Snapshot, error)
	VersionDiff(ctx context.Context, from, to int) (changeset.DiffResult, error)
}

type Handler struct {
	svc service
}

func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

func (h *Handler) CreateChangeset(ctx context.Context, req api.CreateChangesetRequestObject) (api.CreateChangesetResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	desc := ""
	if req.Body.Description != nil {
		desc = *req.Body.Description
	}
	cs, err := h.svc.Create(ctx, req.Body.Title, desc)
	if err != nil {
		return nil, err
	}
	return api.CreateChangeset201JSONResponse(toChangeset(cs)), nil
}

func (h *Handler) ListChangesets(ctx context.Context, req api.ListChangesetsRequestObject) (api.ListChangesetsResponseObject, error) {
	f := changeset.Filter{AuthorID: req.Params.AuthorId}
	if req.Params.Status != nil {
		s := changeset.Status(*req.Params.Status)
		f.Status = &s
	}
	list, err := h.svc.List(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]api.Changeset, len(list))
	for i, cs := range list {
		out[i] = toChangeset(cs)
	}
	return api.ListChangesets200JSONResponse{Changesets: out}, nil
}

func (h *Handler) GetChangeset(ctx context.Context, req api.GetChangesetRequestObject) (api.GetChangesetResponseObject, error) {
	d, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	changes, err := toChanges(d.Changes)
	if err != nil {
		return nil, err
	}
	return api.GetChangeset200JSONResponse{Changeset: toChangeset(d.Changeset), Changes: changes}, nil
}

func (h *Handler) GetChangesetModel(ctx context.Context, req api.GetChangesetModelRequestObject) (api.GetChangesetModelResponseObject, error) {
	snap, err := h.svc.EffectiveModel(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.GetChangesetModel200JSONResponse(modeltransport.ToModel(snap)), nil
}

func (h *Handler) AddChanges(ctx context.Context, req api.AddChangesRequestObject) (api.AddChangesResponseObject, error) {
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
	stored, err := h.svc.AddChanges(ctx, req.Id, inputs)
	if err != nil {
		return nil, err
	}
	changes, err := toChanges(stored)
	if err != nil {
		return nil, err
	}
	return api.AddChanges201JSONResponse{Changes: changes}, nil
}

func (h *Handler) UndoChange(ctx context.Context, req api.UndoChangeRequestObject) (api.UndoChangeResponseObject, error) {
	undone, err := h.svc.Undo(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	changes, err := toChanges(undone)
	if err != nil {
		return nil, err
	}
	return api.UndoChange200JSONResponse{Changes: changes}, nil
}

func (h *Handler) RedoChange(ctx context.Context, req api.RedoChangeRequestObject) (api.RedoChangeResponseObject, error) {
	restored, err := h.svc.Redo(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	changes, err := toChanges(restored)
	if err != nil {
		return nil, err
	}
	return api.RedoChange200JSONResponse{Changes: changes}, nil
}

func toInput(in api.ChangeInput) (changeset.ChangeInput, error) {
	ci := changeset.ChangeInput{
		Op:         changeset.Op(in.Op),
		TargetType: changeset.TargetType(in.TargetType),
	}
	if in.TargetId != nil {
		ci.TargetID = *in.TargetId
	}
	if in.Cascade != nil {
		ci.Cascade = *in.Cascade
	}
	if in.Patch != nil {
		raw, err := json.Marshal(*in.Patch)
		if err != nil {
			return changeset.ChangeInput{}, err
		}
		ci.Patch = raw
	}
	return ci, nil
}

func toChangeset(cs changeset.Changeset) api.Changeset {
	return api.Changeset{
		ApprovedBy: cs.ApprovedBy, ApprovedAt: modeltransport.UTCPtr(cs.ApprovedAt),
		Id: cs.ID, Title: cs.Title, Description: cs.Description, AuthorId: cs.AuthorID,
		BaseVersion: cs.BaseVersion, Status: api.ChangesetStatus(cs.Status),
		CreatedAt: modeltransport.UTC(cs.CreatedAt), MergedAt: modeltransport.UTCPtr(cs.MergedAt),
		Kind: changesetKind(cs.Kind), Reverts: cs.Reverts,
	}
}

// changesetKind maps the kind; rows that predate kinds are drafts.
func changesetKind(k changeset.Kind) api.ChangesetKind {
	if k == changeset.KindDirect {
		return api.ChangesetKindDirect
	}
	return api.ChangesetKindDraft
}

func toChanges(cs []changeset.Change) ([]api.Change, error) {
	out := make([]api.Change, len(cs))
	for i, c := range cs {
		var patch map[string]any
		if len(c.Patch) > 0 {
			if err := json.Unmarshal(c.Patch, &patch); err != nil {
				return nil, err
			}
		}
		if patch == nil {
			patch = map[string]any{}
		}
		var before *map[string]any
		if len(c.Before) > 0 {
			var b map[string]any
			if err := json.Unmarshal(c.Before, &b); err != nil {
				return nil, err
			}
			before = &b
		}
		out[i] = api.Change{
			Id: c.ID, ChangesetId: c.ChangesetID, Seq: c.Seq, BatchId: c.BatchID,
			Op: api.ChangeOp(c.Op), TargetType: api.ChangeTarget(c.TargetType), TargetId: c.TargetID,
			Patch: patch, Before: before, UndoneAt: modeltransport.UTCPtr(c.UndoneAt), CreatedAt: modeltransport.UTC(c.CreatedAt),
		}
	}
	return out, nil
}
