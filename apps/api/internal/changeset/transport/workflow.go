package transport

import (
	"context"
	"encoding/json"

	"archmap/api/internal/changeset"
	"archmap/api/internal/gen/api"
	modeltransport "archmap/api/internal/model/transport"
	"archmap/api/internal/platform/apperr"
)

func (h *Handler) GetChangesetDiff(ctx context.Context, req api.GetChangesetDiffRequestObject) (api.GetChangesetDiffResponseObject, error) {
	res, err := h.svc.Diff(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.GetChangesetDiff200JSONResponse(toDiff(res, res.MainVersion > res.BaseVersion)), nil
}

func (h *Handler) GetVersionDiff(ctx context.Context, req api.GetVersionDiffRequestObject) (api.GetVersionDiffResponseObject, error) {
	res, err := h.svc.VersionDiff(ctx, req.From, req.To)
	if err != nil {
		return nil, err
	}
	return api.GetVersionDiff200JSONResponse(toDiff(res, false)), nil
}

func toDiff(res changeset.DiffResult, stale bool) api.Diff {
	d := res.Diff
	out := api.Diff{BaseVersion: res.BaseVersion, MainVersion: res.MainVersion, Stale: stale}
	out.Elements.Added = modeltransport.ToElements(d.AddedElements)
	out.Elements.Removed = modeltransport.ToElements(d.RemovedElements)
	out.Elements.Modified = make([]api.ElementModification, len(d.ModifiedElements))
	for i, m := range d.ModifiedElements {
		out.Elements.Modified[i] = api.ElementModification{
			Before: modeltransport.ToElement(m.Before), After: modeltransport.ToElement(m.After), Fields: toFieldChanges(m.Fields),
		}
	}
	out.Relations.Added = modeltransport.ToRelations(d.AddedRelations)
	out.Relations.Removed = modeltransport.ToRelations(d.RemovedRelations)
	out.Relations.Modified = make([]api.RelationModification, len(d.ModifiedRelations))
	for i, m := range d.ModifiedRelations {
		out.Relations.Modified[i] = api.RelationModification{
			Before: modeltransport.ToRelation(m.Before), After: modeltransport.ToRelation(m.After), Fields: toFieldChanges(m.Fields),
		}
	}
	out.Owners.Added = modeltransport.ToOwners(d.AddedOwners)
	out.Owners.Removed = modeltransport.ToOwners(d.RemovedOwners)
	out.Owners.Modified = make([]api.OwnerModification, len(d.ModifiedOwners))
	for i, m := range d.ModifiedOwners {
		out.Owners.Modified[i] = api.OwnerModification{
			Before: modeltransport.ToOwner(m.Before), After: modeltransport.ToOwner(m.After), Fields: toFieldChanges(m.Fields),
		}
	}
	out.Ownerships.Added = modeltransport.ToOwnerships(d.AddedOwnerships)
	out.Ownerships.Removed = modeltransport.ToOwnerships(d.RemovedOwnerships)
	out.Ownerships.Modified = make([]api.OwnershipModification, len(d.ModifiedOwnerships))
	for i, m := range d.ModifiedOwnerships {
		out.Ownerships.Modified[i] = api.OwnershipModification{
			Before: modeltransport.ToOwnership(m.Before), After: modeltransport.ToOwnership(m.After), Fields: toFieldChanges(m.Fields),
		}
	}
	return out
}

func (h *Handler) GetChangesetLint(ctx context.Context, req api.GetChangesetLintRequestObject) (api.GetChangesetLintResponseObject, error) {
	report, err := h.svc.Lint(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	findings := make([]api.LintFinding, len(report.Findings))
	for i, f := range report.Findings {
		findings[i] = api.LintFinding{
			Rule: f.Rule, Severity: api.LintSeverity(f.Severity), Message: f.Message,
			TargetType: api.ChangeTarget(f.TargetType), TargetId: f.TargetID,
		}
	}
	return api.GetChangesetLint200JSONResponse{Errors: report.Errors(), Warnings: report.Warnings(), Findings: findings}, nil
}

func (h *Handler) GetChangesetConflicts(ctx context.Context, req api.GetChangesetConflictsRequestObject) (api.GetChangesetConflictsResponseObject, error) {
	conflicts, err := h.svc.Conflicts(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.GetChangesetConflicts200JSONResponse{Conflicts: toConflicts(conflicts)}, nil
}

func (h *Handler) ResolveChangeset(ctx context.Context, req api.ResolveChangesetRequestObject) (api.ResolveChangesetResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("request body is required")
	}
	rs := make([]changeset.Resolution, len(req.Body.Resolutions))
	for i, r := range req.Body.Resolutions {
		rs[i] = changeset.Resolution{
			ResolutionKey: changeset.ResolutionKey{
				TargetType: changeset.TargetType(r.TargetType), TargetID: r.TargetId, Field: r.Field,
			},
			Choice: changeset.Choice(r.Choice),
		}
	}
	remaining, err := h.svc.Resolve(ctx, req.Id, rs)
	if err != nil {
		return nil, err
	}
	return api.ResolveChangeset200JSONResponse{Conflicts: toConflicts(remaining)}, nil
}

func (h *Handler) SubmitChangeset(ctx context.Context, req api.SubmitChangesetRequestObject) (api.SubmitChangesetResponseObject, error) {
	cs, err := h.svc.Submit(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.SubmitChangeset200JSONResponse(toChangeset(cs)), nil
}

func (h *Handler) ApproveChangeset(ctx context.Context, req api.ApproveChangesetRequestObject) (api.ApproveChangesetResponseObject, error) {
	cs, err := h.svc.Approve(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.ApproveChangeset200JSONResponse(toChangeset(cs)), nil
}

func (h *Handler) RejectChangeset(ctx context.Context, req api.RejectChangesetRequestObject) (api.RejectChangesetResponseObject, error) {
	cs, err := h.svc.Reject(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.RejectChangeset200JSONResponse(toChangeset(cs)), nil
}

func (h *Handler) AbandonChangeset(ctx context.Context, req api.AbandonChangesetRequestObject) (api.AbandonChangesetResponseObject, error) {
	cs, err := h.svc.Abandon(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return api.AbandonChangeset200JSONResponse(toChangeset(cs)), nil
}

func (h *Handler) MergeChangeset(ctx context.Context, req api.MergeChangesetRequestObject) (api.MergeChangesetResponseObject, error) {
	res, err := h.svc.Merge(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if len(res.Conflicts) > 0 {
		return api.MergeChangeset409JSONResponse{Conflicts: toConflicts(res.Conflicts)}, nil
	}
	return api.MergeChangeset200JSONResponse{Version: res.Version, Changeset: toChangeset(res.Changeset)}, nil
}

func (h *Handler) ListVersions(ctx context.Context, _ api.ListVersionsRequestObject) (api.ListVersionsResponseObject, error) {
	versions, err := h.svc.Versions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.Version, len(versions))
	for i, v := range versions {
		out[i] = api.Version{
			Number: v.Number, ChangesetId: v.ChangesetID, MergedBy: v.MergedBy, MergedAt: modeltransport.UTC(v.MergedAt), Summary: v.Summary,
			Kind: changesetKind(v.Kind), Reverts: v.Reverts,
		}
	}
	return api.ListVersions200JSONResponse{Versions: out}, nil
}

func (h *Handler) GetVersionModel(ctx context.Context, req api.GetVersionModelRequestObject) (api.GetVersionModelResponseObject, error) {
	snap, err := h.svc.ModelAt(ctx, req.N)
	if err != nil {
		return nil, err
	}
	return api.GetVersionModel200JSONResponse(modeltransport.ToModel(snap)), nil
}

func toConflicts(cs []changeset.Conflict) []api.Conflict {
	out := make([]api.Conflict, len(cs))
	for i, c := range cs {
		out[i] = api.Conflict{
			Kind: api.ConflictKind(c.Kind), TargetType: api.ChangeTarget(c.TargetType), TargetId: c.TargetID,
			Field: c.Field, Label: c.Label, Message: c.Message,
			Base: rawToAny(c.Base), Mine: rawToAny(c.Mine), Theirs: rawToAny(c.Theirs),
		}
	}
	return out
}

func toFieldChanges(fs []changeset.FieldChange) []api.FieldChange {
	out := make([]api.FieldChange, len(fs))
	for i, f := range fs {
		out[i] = api.FieldChange{Field: f.Field, Before: rawToAny(f.Before), After: rawToAny(f.After)}
	}
	return out
}

func rawToAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}
