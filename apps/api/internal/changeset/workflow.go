package changeset

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/lint"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

// DiffResult is a draft's net effect relative to the main version it is based on.
type DiffResult struct {
	Diff        Diff
	BaseVersion int
	MainVersion int // current main; MainVersion > BaseVersion means the draft is stale
}

// MergeResult is the outcome of Merge: either a new version, or the conflicts
// that need explicit resolution (nothing was changed in that case).
type MergeResult struct {
	Changeset Changeset
	Version   int
	Conflicts []Conflict
}

// openForReview loads a changeset that is a draft or in review (its effective
// model is meaningful) for read-only operations.
func (s *Service) loadOpen(ctx context.Context, r Repository, id uuid.UUID) (Changeset, error) {
	cs, err := r.GetChangeset(ctx, id)
	if err != nil {
		return Changeset{}, err
	}
	if cs.Status != StatusDraft && cs.Status != StatusInReview {
		return Changeset{}, apperr.Conflict("changeset is %s", cs.Status)
	}
	return cs, nil
}

// Diff returns the structured difference between the draft's base version and
// its effective model: exactly what merging would change, field by field.
func (s *Service) Diff(ctx context.Context, id uuid.UUID) (DiffResult, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return DiffResult{}, err
	}
	cs, err := s.loadOpen(ctx, s.repo, id)
	if err != nil {
		return DiffResult{}, err
	}
	base, err := s.repo.SnapshotAt(ctx, cs.BaseVersion)
	if err != nil {
		return DiffResult{}, err
	}
	st, err := s.state(ctx, s.repo, cs)
	if err != nil {
		return DiffResult{}, err
	}
	current, err := s.repo.CurrentVersion(ctx)
	if err != nil {
		return DiffResult{}, err
	}
	return DiffResult{Diff: Between(base, st.Snapshot()), BaseVersion: cs.BaseVersion, MainVersion: current}, nil
}

// Lint runs the rule engine on the draft's effective model.
func (s *Service) Lint(ctx context.Context, id uuid.UUID) (lint.Report, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return lint.Report{}, err
	}
	cs, err := s.loadOpen(ctx, s.repo, id)
	if err != nil {
		return lint.Report{}, err
	}
	st, err := s.state(ctx, s.repo, cs)
	if err != nil {
		return lint.Report{}, err
	}
	return lint.Run(st.Snapshot()), nil
}

// Submit moves a draft into review.
func (s *Service) Submit(ctx context.Context, id uuid.UUID) (Changeset, error) {
	return s.transition(ctx, id, func(a authctx.Actor, cs Changeset) error {
		if !a.CanEdit() || (cs.AuthorID != a.ID && !a.IsMaintainer()) {
			return apperr.Forbidden("only the author or a maintainer can submit this changeset")
		}
		if cs.Status != StatusDraft {
			return apperr.Conflict("changeset is %s; only drafts can be submitted", cs.Status)
		}
		return nil
	}, StatusInReview)
}

// Reject sends a changeset in review back as rejected (maintainers only).
func (s *Service) Reject(ctx context.Context, id uuid.UUID) (Changeset, error) {
	return s.transition(ctx, id, func(a authctx.Actor, cs Changeset) error {
		if !a.IsMaintainer() {
			return apperr.Forbidden("only a maintainer can reject a changeset")
		}
		if cs.Status != StatusInReview {
			return apperr.Conflict("changeset is %s; only changesets in review can be rejected", cs.Status)
		}
		return nil
	}, StatusRejected)
}

// Abandon closes a draft or a changeset in review without merging.
func (s *Service) Abandon(ctx context.Context, id uuid.UUID) (Changeset, error) {
	return s.transition(ctx, id, func(a authctx.Actor, cs Changeset) error {
		if !a.CanEdit() || (cs.AuthorID != a.ID && !a.IsMaintainer()) {
			return apperr.Forbidden("only the author or a maintainer can abandon this changeset")
		}
		if cs.Status != StatusDraft && cs.Status != StatusInReview {
			return apperr.Conflict("changeset is %s and cannot be abandoned", cs.Status)
		}
		return nil
	}, StatusAbandoned)
}

func (s *Service) transition(ctx context.Context, id uuid.UUID, check func(authctx.Actor, Changeset) error, to Status) (Changeset, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return Changeset{}, err
	}
	var out Changeset
	err = s.repo.InTx(ctx, func(r Repository) error {
		cs, err := r.LockChangeset(ctx, id)
		if err != nil {
			return err
		}
		if err := check(actor, cs); err != nil {
			return err
		}
		out, err = r.SetStatus(ctx, id, to)
		return err
	})
	return out, err
}

// Approve records a maintainer's sign-off on a changeset in review. Authors
// cannot approve their own changeset unless self-merge is enabled.
func (s *Service) Approve(ctx context.Context, id uuid.UUID) (Changeset, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return Changeset{}, err
	}
	if !actor.IsMaintainer() {
		return Changeset{}, apperr.Forbidden("only a maintainer can approve a changeset")
	}
	var out Changeset
	err = s.repo.InTx(ctx, func(r Repository) error {
		cs, err := r.LockChangeset(ctx, id)
		if err != nil {
			return err
		}
		if cs.Status != StatusInReview {
			return apperr.Conflict("changeset is %s; only changesets in review can be approved", cs.Status)
		}
		if cs.AuthorID == actor.ID && !s.allowSelfMerge {
			return apperr.Forbidden("authors cannot approve their own changeset")
		}
		out, err = r.ApproveChangeset(ctx, id, actor.ID)
		return err
	})
	return out, err
}

// mergeInputs gathers the three sides of a merge for a changeset.
type mergeInputs struct {
	base, mine, theirs model.Snapshot
	current            int
}

func (s *Service) mergeInputs(ctx context.Context, r Repository, cs Changeset) (mergeInputs, error) {
	base, err := r.SnapshotAt(ctx, cs.BaseVersion)
	if err != nil {
		return mergeInputs{}, err
	}
	st, err := s.state(ctx, r, cs)
	if err != nil {
		return mergeInputs{}, err
	}
	current, err := r.CurrentVersion(ctx)
	if err != nil {
		return mergeInputs{}, err
	}
	theirs, err := r.SnapshotAt(ctx, current)
	if err != nil {
		return mergeInputs{}, err
	}
	return mergeInputs{base: base, mine: st.Snapshot(), theirs: theirs, current: current}, nil
}

func resolutionMap(rs []Resolution) map[ResolutionKey]Choice {
	m := make(map[ResolutionKey]Choice, len(rs))
	for _, r := range rs {
		m[r.ResolutionKey] = r.Choice
	}
	return m
}

// merge runs the 3-way merge (or fast-forward when main has not moved).
func (s *Service) merge(ctx context.Context, r Repository, cs Changeset) (model.Snapshot, []Conflict, mergeInputs, error) {
	in, err := s.mergeInputs(ctx, r, cs)
	if err != nil {
		return model.Snapshot{}, nil, in, err
	}
	if in.current == cs.BaseVersion {
		return in.mine, nil, in, nil // fast-forward
	}
	rs, err := r.Resolutions(ctx, cs.ID)
	if err != nil {
		return model.Snapshot{}, nil, in, err
	}
	out := Merge3(in.base, in.mine, in.theirs, resolutionMap(rs))
	return out.Result, out.Conflicts, in, nil
}

// Conflicts reports what still blocks merging the changeset into the current main.
func (s *Service) Conflicts(ctx context.Context, id uuid.UUID) ([]Conflict, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return nil, err
	}
	cs, err := s.loadOpen(ctx, s.repo, id)
	if err != nil {
		return nil, err
	}
	_, conflicts, _, err := s.merge(ctx, s.repo, cs)
	return conflicts, err
}

// Resolve records explicit conflict resolutions (mine / theirs) and returns the
// conflicts that remain.
func (s *Service) Resolve(ctx context.Context, id uuid.UUID, rs []Resolution) ([]Conflict, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return nil, err
	}
	if !actor.CanEdit() {
		return nil, apperr.Forbidden("role %q cannot resolve conflicts", actor.Role)
	}
	if len(rs) == 0 {
		return nil, apperr.Invalid("at least one resolution is required")
	}
	for i, res := range rs {
		switch {
		case !res.Choice.Valid():
			return nil, apperr.Invalid("resolution %d: choice must be mine or theirs", i)
		case res.TargetType != TargetElement && res.TargetType != TargetRelation:
			return nil, apperr.Invalid("resolution %d: invalid target type %q", i, res.TargetType)
		case res.TargetID == uuid.Nil || res.Field == "":
			return nil, apperr.Invalid("resolution %d: target_id and field are required", i)
		}
	}
	var remaining []Conflict
	err = s.repo.InTx(ctx, func(r Repository) error {
		cs, err := r.LockChangeset(ctx, id)
		if err != nil {
			return err
		}
		if cs.AuthorID != actor.ID && !actor.IsMaintainer() {
			return apperr.Forbidden("only the author or a maintainer can resolve conflicts")
		}
		if cs.Status != StatusDraft && cs.Status != StatusInReview {
			return apperr.Conflict("changeset is %s", cs.Status)
		}
		if err := r.SaveResolutions(ctx, id, rs); err != nil {
			return err
		}
		_, remaining, _, err = s.merge(ctx, r, cs)
		return err
	})
	return remaining, err
}

// Merge applies an approved changeset to main as a new version. If main moved
// since the draft's base version it is rebased with a field-level 3-way merge;
// unresolved conflicts are returned and nothing is changed. Lint errors block
// the merge.
func (s *Service) Merge(ctx context.Context, id uuid.UUID) (MergeResult, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return MergeResult{}, err
	}
	if !actor.IsMaintainer() {
		return MergeResult{}, apperr.Forbidden("only a maintainer can merge a changeset")
	}
	var res MergeResult
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := r.LockMain(ctx); err != nil {
			return err
		}
		cs, err := r.LockChangeset(ctx, id)
		if err != nil {
			return err
		}
		if cs.Status != StatusInReview {
			return apperr.Conflict("changeset is %s; only changesets in review can be merged", cs.Status)
		}
		if cs.ApprovedBy == nil {
			return apperr.Conflict("changeset has not been approved")
		}
		if cs.AuthorID == actor.ID && !s.allowSelfMerge {
			return apperr.Forbidden("authors cannot merge their own changeset")
		}

		result, conflicts, in, err := s.merge(ctx, r, cs)
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			res = MergeResult{Changeset: cs, Conflicts: conflicts}
			return nil
		}
		if report := lint.Run(result); report.Errors() > 0 {
			return apperr.Conflict("merge blocked by %d lint error(s); see the lint report", report.Errors())
		}
		if touchesProtected(Between(in.base, in.mine)) &&
			(cs.AuthorID == actor.ID || *cs.ApprovedBy == cs.AuthorID) {
			return apperr.Forbidden("changes to protected elements need an approval and merge by a maintainer other than the author")
		}

		changes, err := Between(in.theirs, result).Changes(time.Now().UTC())
		if err != nil {
			return err
		}
		if err := r.MaterializeChanges(ctx, changes); err != nil {
			return err
		}
		version := in.current + 1
		snap, err := r.MainSnapshot(ctx)
		if err != nil {
			return err
		}
		if err := r.RecordVersion(ctx, version, cs.ID, actor.ID, cs.Title, snap); err != nil {
			return err
		}
		if err := r.MarkMerged(ctx, cs.ID); err != nil {
			return err
		}
		merged, err := r.GetChangeset(ctx, cs.ID)
		if err != nil {
			return err
		}
		res = MergeResult{Changeset: merged, Version: version}
		return nil
	})
	return res, err
}

// Versions lists merged model versions, newest first.
func (s *Service) Versions(ctx context.Context) ([]Version, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return nil, err
	}
	return s.repo.ListVersions(ctx)
}

// ModelAt returns main as of a merged version (read-only history).
func (s *Service) ModelAt(ctx context.Context, version int) (model.Snapshot, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return model.Snapshot{}, err
	}
	if version < 1 {
		return model.Snapshot{}, apperr.NotFound("version %d not found", version)
	}
	return s.repo.SnapshotAt(ctx, version)
}

// VersionDiff is what changed going from merged version `from` to version `to`
// (either order: comparing a newer version with an older one shows the reverse).
func (s *Service) VersionDiff(ctx context.Context, from, to int) (DiffResult, error) {
	a, err := s.ModelAt(ctx, from)
	if err != nil {
		return DiffResult{}, err
	}
	b, err := s.ModelAt(ctx, to)
	if err != nil {
		return DiffResult{}, err
	}
	return DiffResult{Diff: Between(a, b), BaseVersion: from, MainVersion: to}, nil
}

// touchesProtected reports whether the diff modifies or removes a protected element.
func touchesProtected(d Diff) bool {
	for _, m := range d.ModifiedElements {
		if m.Before.Protected {
			return true
		}
	}
	for _, e := range d.RemovedElements {
		if e.Protected {
			return true
		}
	}
	return false
}

// touchesProtectedFlag reports whether a change sets or clears the protected flag.
func touchesProtectedFlag(c Change) bool {
	if c.TargetType != TargetElement || c.Op == OpDelete {
		return false
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(c.Patch, &patch); err != nil {
		return false
	}
	v, ok := patch["protected"]
	if !ok {
		return false
	}
	if c.Op == OpAdd {
		return string(v) == "true"
	}
	return true
}
