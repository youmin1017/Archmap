package model

import (
	"context"

	"github.com/google/uuid"

	"archmap/api/internal/platform/authctx"
)

type ElementFilter struct {
	ParentID *uuid.UUID
	Query    string
}

type RelationFilter struct {
	SourceID *uuid.UUID
	TargetID *uuid.UUID
}

// Reader is the read side of main-model storage (implemented by repository).
type Reader interface {
	ListElements(ctx context.Context, f ElementFilter) ([]Element, error)
	GetElement(ctx context.Context, id uuid.UUID) (Element, error)
	ListRelations(ctx context.Context, f RelationFilter) ([]Relation, error)
	GetRelation(ctx context.Context, id uuid.UUID) (Relation, error)
	Snapshot(ctx context.Context) (Snapshot, error)
}

// Service exposes the read-only main model. Main is never written through the
// API; modifications happen in changesets (see package changeset).
type Service struct {
	r Reader
}

func NewService(r Reader) *Service { return &Service{r: r} }

func (s *Service) ListElements(ctx context.Context, f ElementFilter) ([]Element, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return nil, err
	}
	return s.r.ListElements(ctx, f)
}

func (s *Service) GetElement(ctx context.Context, id uuid.UUID) (Element, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return Element{}, err
	}
	return s.r.GetElement(ctx, id)
}

func (s *Service) ListRelations(ctx context.Context, f RelationFilter) ([]Relation, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return nil, err
	}
	return s.r.ListRelations(ctx, f)
}

func (s *Service) GetRelation(ctx context.Context, id uuid.UUID) (Relation, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return Relation{}, err
	}
	return s.r.GetRelation(ctx, id)
}

// Snapshot returns all of main (elements, relations, owners, ownerships).
func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return Snapshot{}, err
	}
	return s.r.Snapshot(ctx)
}

// Effective returns the inherited values of every element of main (ADR-064).
func (s *Service) Effective(ctx context.Context) (map[uuid.UUID]Effective, error) {
	snap, err := s.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return ComputeEffective(snap), nil
}
