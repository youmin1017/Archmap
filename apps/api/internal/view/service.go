package view

import (
	"context"
	"math"
	"strings"

	"github.com/google/uuid"

	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

const (
	maxNameLength   = 120
	maxLayoutItems  = 5000
	maxMembersCount = 5000
)

// Repository is the storage the service needs (implemented in ./repository).
type Repository interface {
	InTx(ctx context.Context, fn func(Repository) error) error
	Insert(ctx context.Context, v View) (View, error)
	Get(ctx context.Context, id uuid.UUID) (View, error)
	List(ctx context.Context) ([]View, error)
	Update(ctx context.Context, v View) (View, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Elements(ctx context.Context, viewID uuid.UUID) ([]Element, error)
	SetMembers(ctx context.Context, viewID uuid.UUID, ids []uuid.UUID) error
	// SetLayout replaces the saved layout of a view (membership is untouched).
	SetLayout(ctx context.Context, viewID uuid.UUID, layout []Element) error
}

// ModelSource supplies the model a view is computed from: main, or a draft's
// effective model when changesetID is set.
type ModelSource interface {
	Snapshot(ctx context.Context, changesetID *uuid.UUID) (model.Snapshot, error)
}

type Service struct {
	repo Repository
	src  ModelSource
}

func NewService(repo Repository, src ModelSource) *Service { return &Service{repo: repo, src: src} }

// Detail is what a view shows plus its saved layout.
type Detail struct {
	View   View
	Result Result
	Layout []Element
}

func (s *Service) List(ctx context.Context) ([]View, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return nil, err
	}
	return s.repo.List(ctx)
}

func (s *Service) Create(ctx context.Context, in Input) (View, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return View{}, err
	}
	in, err = validate(in)
	if err != nil {
		return View{}, err
	}
	var out View
	err = s.repo.InTx(ctx, func(r Repository) error {
		out, err = r.Insert(ctx, View{
			Name: in.Name, Kind: in.Kind, ScopeElementID: in.ScopeElementID,
			Description: in.Description, CreatedBy: actor.ID, Layers: in.Layers,
		})
		if err != nil {
			return err
		}
		if in.Kind == KindCustom && in.Members != nil {
			return r.SetMembers(ctx, out.ID, in.Members)
		}
		return nil
	})
	return out, err
}

// Update replaces name, kind, scope and description; Members replaces the
// member list of a custom view when non-nil.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in Input) (View, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return View{}, err
	}
	in, err = validate(in)
	if err != nil {
		return View{}, err
	}
	var out View
	err = s.repo.InTx(ctx, func(r Repository) error {
		cur, err := r.Get(ctx, id)
		if err != nil {
			return err
		}
		if cur.CreatedBy != actor.ID && !actor.IsMaintainer() {
			return apperr.Forbidden("only the creator or a maintainer can change this view")
		}
		cur.Name, cur.Kind, cur.ScopeElementID, cur.Description, cur.Layers = in.Name, in.Kind, in.ScopeElementID, in.Description, in.Layers
		if out, err = r.Update(ctx, cur); err != nil {
			return err
		}
		if in.Kind == KindCustom && in.Members != nil {
			return r.SetMembers(ctx, id, in.Members)
		}
		return nil
	})
	return out, err
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	actor, err := requireEditor(ctx)
	if err != nil {
		return err
	}
	return s.repo.InTx(ctx, func(r Repository) error {
		cur, err := r.Get(ctx, id)
		if err != nil {
			return err
		}
		if cur.CreatedBy != actor.ID && !actor.IsMaintainer() {
			return apperr.Forbidden("only the creator or a maintainer can delete this view")
		}
		return r.Delete(ctx, id)
	})
}

// Model computes what the view shows in main (or in the given draft).
func (s *Service) Model(ctx context.Context, id uuid.UUID, changesetID *uuid.UUID) (Detail, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return Detail{}, err
	}
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	layout, err := s.repo.Elements(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	snap, err := s.src.Snapshot(ctx, changesetID)
	if err != nil {
		return Detail{}, err
	}
	var members []uuid.UUID
	for _, l := range layout {
		if l.Included {
			members = append(members, l.ElementID)
		}
	}
	return Detail{View: v, Result: Compute(v, members, snap), Layout: layout}, nil
}

// SetLayout replaces the saved layout. It is shared by everyone who can see
// the view (last write wins) and never creates changes or versions.
func (s *Service) SetLayout(ctx context.Context, id uuid.UUID, layout []Element) ([]Element, error) {
	if _, err := requireEditor(ctx); err != nil {
		return nil, err
	}
	if len(layout) > maxLayoutItems {
		return nil, apperr.Invalid("at most %d layout entries", maxLayoutItems)
	}
	seen := map[uuid.UUID]bool{}
	for i, l := range layout {
		switch {
		case l.ElementID == uuid.Nil:
			return nil, apperr.Invalid("layout entry %d: element_id is required", i)
		case seen[l.ElementID]:
			return nil, apperr.Invalid("layout entry %d: duplicate element %s", i, l.ElementID)
		case !finite(l.X) || !finite(l.Y) || !finite(l.W) || !finite(l.H):
			return nil, apperr.Invalid("layout entry %d: coordinates must be finite numbers", i)
		case (l.W != nil && *l.W < 0) || (l.H != nil && *l.H < 0):
			return nil, apperr.Invalid("layout entry %d: size must not be negative", i)
		}
		seen[l.ElementID] = true
	}
	var out []Element
	err := s.repo.InTx(ctx, func(r Repository) error {
		if _, err := r.Get(ctx, id); err != nil {
			return err
		}
		if err := r.SetLayout(ctx, id, layout); err != nil {
			return err
		}
		var err error
		out, err = r.Elements(ctx, id)
		return err
	})
	return out, err
}

func requireEditor(ctx context.Context) (authctx.Actor, error) {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return authctx.Actor{}, err
	}
	if !actor.CanEdit() {
		return authctx.Actor{}, apperr.Forbidden("role %q cannot change views", actor.Role)
	}
	return actor, nil
}

func finite(f *float64) bool { return f == nil || (!math.IsNaN(*f) && !math.IsInf(*f, 0)) }

func validate(in Input) (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "":
		return in, apperr.Invalid("name must not be empty")
	case len([]rune(in.Name)) > maxNameLength:
		return in, apperr.Invalid("name must be at most %d characters", maxNameLength)
	case !in.Kind.Valid():
		return in, apperr.Invalid("invalid view kind %q", in.Kind)
	}
	switch rule := in.Kind.ScopeRule(); {
	case rule == ScopeRequired && in.ScopeElementID == nil:
		return in, apperr.Invalid("a %s view needs a scope element", in.Kind)
	case rule == ScopeForbidden && in.ScopeElementID != nil:
		return in, apperr.Invalid("a %s view does not take a scope element", in.Kind)
	}
	layers, err := validateLayers(in.Kind, in.Layers)
	if err != nil {
		return in, err
	}
	in.Layers = layers
	if in.Members != nil {
		if in.Kind != KindCustom {
			return in, apperr.Invalid("only custom views have a member list")
		}
		if len(in.Members) > maxMembersCount {
			return in, apperr.Invalid("at most %d members", maxMembersCount)
		}
		seen := map[uuid.UUID]bool{}
		uniq := in.Members[:0:0]
		for _, m := range in.Members {
			if !seen[m] {
				seen[m] = true
				uniq = append(uniq, m)
			}
		}
		in.Members = uniq
	}
	return in, nil
}

const maxLayers = 12

// validateLayers normalizes the layer list of a layers view and rejects it for other kinds.
func validateLayers(kind Kind, layers []string) ([]string, error) {
	if kind != KindLayers {
		if len(layers) > 0 {
			return nil, apperr.Invalid("only layers views have a layer list")
		}
		return []string{}, nil
	}
	if len(layers) == 0 {
		return nil, apperr.Invalid("a layers view needs at least one layer")
	}
	if len(layers) > maxLayers {
		return nil, apperr.Invalid("at most %d layers", maxLayers)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(layers))
	for _, l := range layers {
		l = strings.TrimSpace(l)
		switch {
		case l == "":
			return nil, apperr.Invalid("layer names must not be empty")
		case len([]rune(l)) > model.MaxLayerLength || strings.ContainsAny(l, "\r\n"):
			return nil, apperr.Invalid("layer names must be a single line of at most %d characters", model.MaxLayerLength)
		case seen[strings.ToLower(l)]:
			return nil, apperr.Invalid("duplicate layer %q", l)
		}
		seen[strings.ToLower(l)] = true
		out = append(out, l)
	}
	return out, nil
}
