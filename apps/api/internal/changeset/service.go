package changeset

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

const maxBatchSize = 500

// Repository is the storage the service needs (implemented in ./repository).
// InTx runs fn against a transaction-bound Repository.
type Repository interface {
	InTx(ctx context.Context, fn func(Repository) error) error

	InsertChangeset(ctx context.Context, cs Changeset) (Changeset, error)
	GetChangeset(ctx context.Context, id uuid.UUID) (Changeset, error)
	// LockChangeset reads the row FOR UPDATE; call inside InTx.
	LockChangeset(ctx context.Context, id uuid.UUID) (Changeset, error)
	ListChangesets(ctx context.Context, f Filter) ([]Changeset, error)
	CurrentVersion(ctx context.Context) (int, error)

	// MainSnapshot reads the current main tables; SnapshotAt returns main as of a
	// merged version (0 = empty model).
	MainSnapshot(ctx context.Context) (model.Snapshot, error)
	SnapshotAt(ctx context.Context, version int) (model.Snapshot, error)
	ListVersions(ctx context.Context) ([]Version, error)
	// LockMain serializes merges (transaction-scoped).
	LockMain(ctx context.Context) error

	ListChanges(ctx context.Context, changesetID uuid.UUID) ([]Change, error)
	ActiveChanges(ctx context.Context, changesetID uuid.UUID) ([]Change, error)
	MaxSeq(ctx context.Context, changesetID uuid.UUID) (int, error)
	InsertChange(ctx context.Context, c Change) (Change, error)
	DeleteUndone(ctx context.Context, changesetID uuid.UUID) error
	UndoLastBatch(ctx context.Context, changesetID uuid.UUID) ([]Change, error)
	RedoNextBatch(ctx context.Context, changesetID uuid.UUID) ([]Change, error)

	// Used by Bootstrap (and by the M2 merge): write changes into main.
	MaterializeChanges(ctx context.Context, changes []Change) error
	RecordVersion(ctx context.Context, number int, changesetID, mergedBy uuid.UUID, summary string, snapshot model.Snapshot) error
	MarkMerged(ctx context.Context, changesetID uuid.UUID) error

	ApproveChangeset(ctx context.Context, id, by uuid.UUID) (Changeset, error)
	SetStatus(ctx context.Context, id uuid.UUID, status Status) (Changeset, error)
	Resolutions(ctx context.Context, changesetID uuid.UUID) ([]Resolution, error)
	SaveResolutions(ctx context.Context, changesetID uuid.UUID, rs []Resolution) error
}

// LayerRuleSource provides the layer rules enforced on element changes.
type LayerRuleSource interface {
	LayerRules(ctx context.Context) ([]model.LayerRule, error)
}

type Service struct {
	repo           Repository
	allowSelfMerge bool
	layerRules     LayerRuleSource // nil = no layer rules
}

type Option func(*Service)

// WithSelfMerge lets a maintainer approve and merge their own changeset
// (off by default; protected-element changes always need an independent reviewer).
func WithSelfMerge(allow bool) Option { return func(s *Service) { s.allowSelfMerge = allow } }

// WithLayerRules enforces the given layer rules when elements are added or their layer changes.
func WithLayerRules(src LayerRuleSource) Option { return func(s *Service) { s.layerRules = src } }

func NewService(repo Repository, opts ...Option) *Service {
	s := &Service{repo: repo}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Create opens a new draft based on the current main version.
func (s *Service) Create(ctx context.Context, title, description string) (Changeset, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return Changeset{}, err
	}
	if !actor.CanEdit() {
		return Changeset{}, apperr.Forbidden("role %q cannot create changesets", actor.Role)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Changeset{}, apperr.Invalid("title must not be empty")
	}
	base, err := s.repo.CurrentVersion(ctx)
	if err != nil {
		return Changeset{}, err
	}
	return s.repo.InsertChangeset(ctx, Changeset{
		Title: title, Description: description, AuthorID: actor.ID,
		BaseVersion: base, Status: StatusDraft,
	})
}

func (s *Service) List(ctx context.Context, f Filter) ([]Changeset, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return nil, err
	}
	return s.repo.ListChangesets(ctx, f)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Detail, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return Detail{}, err
	}
	cs, err := s.repo.GetChangeset(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	changes, err := s.repo.ListChanges(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Changeset: cs, Changes: changes}, nil
}

// EffectiveModel returns main plus the draft's active changes. Main itself is
// never modified.
func (s *Service) EffectiveModel(ctx context.Context, id uuid.UUID) (model.Snapshot, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return model.Snapshot{}, err
	}
	cs, err := s.repo.GetChangeset(ctx, id)
	if err != nil {
		return model.Snapshot{}, err
	}
	if cs.Status != StatusDraft && cs.Status != StatusInReview {
		return model.Snapshot{}, apperr.Conflict("changeset is %s; its effective model is not available", cs.Status)
	}
	st, err := s.state(ctx, s.repo, cs)
	if err != nil {
		return model.Snapshot{}, err
	}
	return st.Snapshot(), nil
}

// state is the draft's effective model: main as of the draft's base version
// plus its active changes.
func (s *Service) state(ctx context.Context, r Repository, cs Changeset) (*State, error) {
	base, err := r.SnapshotAt(ctx, cs.BaseVersion)
	if err != nil {
		return nil, err
	}
	active, err := r.ActiveChanges(ctx, cs.ID)
	if err != nil {
		return nil, err
	}
	st, err := Overlay(base, active)
	if err != nil {
		return nil, fmt.Errorf("replay changeset %s: %w", cs.ID, err)
	}
	return st, nil
}

// AddChanges records one batch (one GUI edit) in a draft. Inputs are validated
// sequentially; if any fails nothing is stored. Recording a new batch discards
// the redo stack.
func (s *Service) AddChanges(ctx context.Context, id uuid.UUID, inputs []ChangeInput) ([]Change, error) {
	if len(inputs) == 0 {
		return nil, apperr.Invalid("at least one change is required")
	}
	if len(inputs) > maxBatchSize {
		return nil, apperr.Invalid("at most %d changes per request", maxBatchSize)
	}
	actor, err := authctx.Require(ctx)
	if err != nil {
		return nil, err
	}
	var stored []Change
	err = s.editDraft(ctx, id, func(r Repository, cs Changeset) error {
		st, err := s.state(ctx, r, cs)
		if err != nil {
			return err
		}
		batch, err := uuid.NewV7()
		if err != nil {
			return err
		}
		b := &builder{st: st, batch: batch, now: time.Now().UTC()}
		if s.layerRules != nil {
			if b.rules, err = s.layerRules.LayerRules(ctx); err != nil {
				return err
			}
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
		if err := r.DeleteUndone(ctx, id); err != nil {
			return err
		}
		seq, err := r.MaxSeq(ctx, id)
		if err != nil {
			return err
		}
		for _, c := range b.out {
			seq++
			c.ChangesetID, c.Seq = id, seq
			saved, err := r.InsertChange(ctx, c)
			if err != nil {
				return err
			}
			stored = append(stored, saved)
		}
		return nil
	})
	return stored, err
}

// Undo marks the latest active batch undone and returns its changes.
func (s *Service) Undo(ctx context.Context, id uuid.UUID) ([]Change, error) {
	var out []Change
	err := s.editDraft(ctx, id, func(r Repository, _ Changeset) error {
		var err error
		if out, err = r.UndoLastBatch(ctx, id); err != nil {
			return err
		}
		if len(out) == 0 {
			return apperr.Conflict("nothing to undo")
		}
		return nil
	})
	return out, err
}

// Redo restores the most recently undone batch and returns its changes.
func (s *Service) Redo(ctx context.Context, id uuid.UUID) ([]Change, error) {
	var out []Change
	err := s.editDraft(ctx, id, func(r Repository, _ Changeset) error {
		var err error
		if out, err = r.RedoNextBatch(ctx, id); err != nil {
			return err
		}
		if len(out) == 0 {
			return apperr.Conflict("nothing to redo")
		}
		return nil
	})
	return out, err
}

// editDraft runs fn in a transaction after locking the changeset and checking
// that the actor may edit it (editor+, author or maintainer) and that it is a draft.
func (s *Service) editDraft(ctx context.Context, id uuid.UUID, fn func(Repository, Changeset) error) error {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return err
	}
	if !actor.CanEdit() {
		return apperr.Forbidden("role %q cannot edit changesets", actor.Role)
	}
	return s.repo.InTx(ctx, func(r Repository) error {
		cs, err := r.LockChangeset(ctx, id)
		if err != nil {
			return err
		}
		if cs.AuthorID != actor.ID && !actor.IsMaintainer() {
			return apperr.Forbidden("only the author or a maintainer can edit this changeset")
		}
		if cs.Status != StatusDraft {
			return apperr.Conflict("changeset is %s; only drafts can be edited", cs.Status)
		}
		return fn(r, cs)
	})
}

// Bootstrap creates main's first version directly from inputs (version 1). It
// exists for the dev seed: main is read-only through the API, so the initial
// sample model needs a privileged path. It records a merged changeset exactly
// like a real merge would, and refuses to run once a version exists.
func (s *Service) Bootstrap(ctx context.Context, title string, inputs []ChangeInput) (Changeset, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return Changeset{}, err
	}
	if !actor.IsMaintainer() {
		return Changeset{}, apperr.Forbidden("only a maintainer can bootstrap the model")
	}
	var cs Changeset
	err = s.repo.InTx(ctx, func(r Repository) error {
		cur, err := r.CurrentVersion(ctx)
		if err != nil {
			return err
		}
		if cur != 0 {
			return apperr.Conflict("model already initialised at version %d", cur)
		}
		if cs, err = r.InsertChangeset(ctx, Changeset{
			Title: title, AuthorID: actor.ID, BaseVersion: 0, Status: StatusDraft,
		}); err != nil {
			return err
		}
		st, err := s.state(ctx, r, cs)
		if err != nil {
			return err
		}
		batch, err := uuid.NewV7()
		if err != nil {
			return err
		}
		b := &builder{st: st, batch: batch, now: time.Now().UTC()}
		for i, in := range inputs {
			if err := b.apply(in); err != nil {
				return fmt.Errorf("change %d: %w", i, err)
			}
		}
		var saved []Change
		for i, c := range b.out {
			c.ChangesetID, c.Seq = cs.ID, i+1
			sc, err := r.InsertChange(ctx, c)
			if err != nil {
				return err
			}
			saved = append(saved, sc)
		}
		if err := r.MaterializeChanges(ctx, saved); err != nil {
			return err
		}
		snap, err := r.MainSnapshot(ctx)
		if err != nil {
			return err
		}
		if err := r.RecordVersion(ctx, 1, cs.ID, actor.ID, title, snap); err != nil {
			return err
		}
		if err := r.MarkMerged(ctx, cs.ID); err != nil {
			return err
		}
		cs, err = r.GetChangeset(ctx, cs.ID)
		return err
	})
	return cs, err
}
