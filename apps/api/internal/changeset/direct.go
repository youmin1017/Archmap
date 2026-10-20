package changeset

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"archmap/api/internal/lint"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

// PolicySource tells whether main may be edited directly (review policy
// "direct", ADR-072).
type PolicySource interface {
	DirectEditsAllowed(ctx context.Context) (bool, error)
}

// WithPolicy enables direct edits when the source allows them.
func WithPolicy(src PolicySource) Option { return func(s *Service) { s.policy = src } }

// DirectOutcome is what happened to a direct edit.
type DirectOutcome string

const (
	// DirectMerged: the edit is a new main version.
	DirectMerged DirectOutcome = "merged"
	// DirectDraft: the edit adds lint errors; it was kept as a draft to fix.
	DirectDraft DirectOutcome = "draft"
	// DirectInReview: the edit touches a protected element; it was submitted for review.
	DirectInReview DirectOutcome = "in_review"
)

type DirectResult struct {
	Outcome   DirectOutcome
	Changeset Changeset
	Version   int    // the new main version when merged
	Reason    string // why it was not merged (empty when merged)
}

const (
	defaultDirectTitle = "Direct edit"
	maxTitleLen        = 200
)

// ApplyDirect edits main directly: the batch becomes a changeset based on the
// current main and is merged at once as a new version, all in one transaction
// under the main lock, so it is always a fast-forward. Only editors may do it,
// and only under the "direct" review policy. A batch that adds lint errors is
// kept as a draft instead, and one that touches a protected element is
// submitted for review: governance still applies (ADR-072).
func (s *Service) ApplyDirect(ctx context.Context, summary string, inputs []ChangeInput) (DirectResult, error) {
	if len(inputs) == 0 {
		return DirectResult{}, apperr.Invalid("at least one change is required")
	}
	if len(inputs) > maxBatchSize {
		return DirectResult{}, apperr.Invalid("at most %d changes per request", maxBatchSize)
	}
	actor, err := s.requireDirect(ctx)
	if err != nil {
		return DirectResult{}, err
	}
	title := strings.TrimSpace(summary)
	if title == "" {
		title = defaultDirectTitle
	}
	if utf8.RuneCountInString(title) > maxTitleLen {
		return DirectResult{}, apperr.Invalid("summary must be at most %d characters", maxTitleLen)
	}

	var res DirectResult
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := r.LockMain(ctx); err != nil {
			return err
		}
		current, err := r.CurrentVersion(ctx)
		if err != nil {
			return err
		}
		base, err := r.SnapshotAt(ctx, current)
		if err != nil {
			return err
		}
		st, err := Overlay(base, nil)
		if err != nil {
			return err
		}
		b, err := s.newBuilder(ctx, st)
		if err != nil {
			return err
		}
		for i, in := range inputs {
			if err := b.apply(in); err != nil {
				return fmt.Errorf("change %d: %w", i, err)
			}
		}
		if !actor.IsMaintainer() {
			for _, c := range b.out {
				if touchesProtectedFlag(c) {
					return apperr.Forbidden("only maintainers can change the protected flag")
				}
			}
		}
		result := st.Snapshot()
		if Between(base, result).Empty() {
			return apperr.Invalid("the changes do not change the model")
		}
		cs, err := r.InsertChangeset(ctx, Changeset{
			Title: title, AuthorID: actor.ID, BaseVersion: current, Status: StatusDraft,
		})
		if err != nil {
			return err
		}
		if err := storeChanges(ctx, r, cs.ID, b.out); err != nil {
			return err
		}
		res, err = s.finishDirect(ctx, r, actor, cs, base, result, current)
		return err
	})
	return res, err
}

// RevertDirect undoes a direct edit with a new direct edit that reverses it: the
// edit's version is merged back onto main (base = its version, mine = the version
// before it, theirs = main), so it fails when what it changed was changed again
// since. Only its author or a maintainer may undo it, once.
func (s *Service) RevertDirect(ctx context.Context, id uuid.UUID) (DirectResult, error) {
	actor, err := s.requireDirect(ctx)
	if err != nil {
		return DirectResult{}, err
	}
	var res DirectResult
	err = s.repo.InTx(ctx, func(r Repository) error {
		if err := r.LockMain(ctx); err != nil {
			return err
		}
		target, err := r.LockChangeset(ctx, id)
		if err != nil {
			return err
		}
		if target.Kind != KindDirect || target.Status != StatusMerged {
			return apperr.Conflict("only merged direct edits can be undone this way")
		}
		if target.AuthorID != actor.ID && !actor.IsMaintainer() {
			return apperr.Forbidden("only the author or a maintainer can undo this edit")
		}
		if reverted, err := r.IsReverted(ctx, id); err != nil {
			return err
		} else if reverted {
			return apperr.Conflict("this edit was already undone")
		}
		v, err := r.VersionOf(ctx, id)
		if err != nil {
			return err
		}
		after, err := r.SnapshotAt(ctx, v)
		if err != nil {
			return err
		}
		before, err := r.SnapshotAt(ctx, v-1)
		if err != nil {
			return err
		}
		current, err := r.CurrentVersion(ctx)
		if err != nil {
			return err
		}
		main, err := r.SnapshotAt(ctx, current)
		if err != nil {
			return err
		}
		merged := Merge3(after, before, main, nil)
		if len(merged.Conflicts) > 0 {
			return apperr.Conflict("cannot undo %q: what it changed was changed again since", target.Title)
		}
		diff := Between(main, merged.Result)
		if diff.Empty() {
			return apperr.Conflict("nothing to undo: main already matches the state before %q", target.Title)
		}

		// Record the reversal as regular changes so the changeset reads like any other.
		st, err := Overlay(main, nil)
		if err != nil {
			return err
		}
		b, err := s.newBuilder(ctx, st)
		if err != nil {
			return err
		}
		changes, err := diff.Changes(time.Now().UTC())
		if err != nil {
			return err
		}
		for _, c := range changes {
			in := ChangeInput{Op: c.Op, TargetType: c.TargetType, TargetID: c.TargetID, Patch: c.Patch}
			if c.Op == OpDelete && !b.st.exists(c.TargetType, c.TargetID) {
				continue // already removed by an earlier cascade in this batch
			}
			if c.Op == OpDelete && c.TargetType == TargetElement {
				in.Cascade = true
			}
			if err := b.apply(in); err != nil {
				return fmt.Errorf("undo %q: %w", target.Title, err)
			}
		}
		title := "Undo: " + target.Title
		if utf8.RuneCountInString(title) > maxTitleLen {
			title = string([]rune(title)[:maxTitleLen])
		}
		cs, err := r.InsertChangeset(ctx, Changeset{
			Title: title, AuthorID: actor.ID, BaseVersion: current, Status: StatusDraft, Reverts: &id,
		})
		if err != nil {
			return err
		}
		if err := storeChanges(ctx, r, cs.ID, b.out); err != nil {
			return err
		}
		res, err = s.finishDirect(ctx, r, actor, cs, main, st.Snapshot(), current)
		return err
	})
	return res, err
}

// requireDirect checks that the caller may edit and that the policy allows direct edits.
func (s *Service) requireDirect(ctx context.Context) (authctx.Actor, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return authctx.Actor{}, err
	}
	if !actor.CanEdit() {
		return authctx.Actor{}, apperr.Forbidden("role %q cannot edit the model", actor.Role)
	}
	allowed := false
	if s.policy != nil {
		if allowed, err = s.policy.DirectEditsAllowed(ctx); err != nil {
			return authctx.Actor{}, err
		}
	}
	if !allowed {
		return authctx.Actor{}, apperr.Forbidden("main can only be changed through reviewed drafts (review policy is required)")
	}
	return actor, nil
}

// finishDirect merges a direct edit, or keeps it as a draft (new lint errors) or
// submits it for review (protected elements). `base` is main (= the changeset's base).
func (s *Service) finishDirect(ctx context.Context, r Repository, actor authctx.Actor, cs Changeset,
	base, result model.Snapshot, current int,
) (DirectResult, error) {
	rules, err := s.lintRules(ctx)
	if err != nil {
		return DirectResult{}, err
	}
	// Only errors the edit introduces count: an error main already has must not block every edit.
	if n := newErrors(lint.Run(base, rules...), lint.Run(result, rules...)); n > 0 {
		return DirectResult{
			Outcome: DirectDraft, Changeset: cs,
			Reason: fmt.Sprintf("the edit adds %d lint error(s); it was saved as a draft to fix", n),
		}, nil
	}
	if touchesProtected(base, Between(base, result)) {
		cs, err := r.SetStatus(ctx, cs.ID, StatusInReview)
		if err != nil {
			return DirectResult{}, err
		}
		return DirectResult{
			Outcome: DirectInReview, Changeset: cs,
			Reason: "the edit changes a protected element; it was submitted for review",
		}, nil
	}
	if err := r.MarkDirect(ctx, cs.ID); err != nil {
		return DirectResult{}, err
	}
	version, merged, err := commit(ctx, r, cs, base, result, current, actor.ID)
	if err != nil {
		return DirectResult{}, err
	}
	return DirectResult{Outcome: DirectMerged, Changeset: merged, Version: version}, nil
}

// newErrors counts lint errors in `after` that `before` does not have.
func newErrors(before, after lint.Report) int {
	type key struct {
		rule, msg string
		target    uuid.UUID
	}
	had := map[key]bool{}
	for _, f := range before.Findings {
		if f.Severity == lint.SeverityError {
			had[key{f.Rule, f.Message, f.TargetID}] = true
		}
	}
	n := 0
	for _, f := range after.Findings {
		if f.Severity == lint.SeverityError && !had[key{f.Rule, f.Message, f.TargetID}] {
			n++
		}
	}
	return n
}

// newBuilder prepares a builder over st with the layer rules and principals.
func (s *Service) newBuilder(ctx context.Context, st *State) (*builder, error) {
	batch, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	b := &builder{st: st, batch: batch, now: time.Now().UTC()}
	if b.principals, err = s.loadPrincipals(ctx); err != nil {
		return nil, err
	}
	if s.layerRules != nil {
		if b.rules, err = s.layerRules.LayerRules(ctx); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// storeChanges records a built batch in a changeset (seq from 1).
func storeChanges(ctx context.Context, r Repository, id uuid.UUID, out []Change) error {
	for i, c := range out {
		c.ChangesetID, c.Seq = id, i+1
		if _, err := r.InsertChange(ctx, c); err != nil {
			return err
		}
	}
	return nil
}
