// Package view implements saved perspectives over the model: which elements a
// view shows (by kind and scope, or a manual list) and where the user placed
// them. Views are layout-like: writing them never creates changes or versions.
package view

import (
	"time"

	"github.com/google/uuid"
)

type Kind string

const (
	KindLandscape  Kind = "landscape"
	KindContext    Kind = "context"
	KindContainer  Kind = "container"
	KindComponent  Kind = "component"
	KindDeployment Kind = "deployment"
	KindCustom     Kind = "custom"
	// KindLayers shows every element on a stacked plane per layer (3D); the layers belong to the view.
	KindLayers Kind = "layers"
	// KindUnowned shows the systems, containers, components and nodes without an
	// effective owner (ADR-063), with their ancestors as context.
	KindUnowned Kind = "unowned"
)

func (k Kind) Valid() bool {
	switch k {
	case KindLandscape, KindContext, KindContainer, KindComponent, KindDeployment, KindCustom, KindLayers, KindUnowned:
		return true
	}
	return false
}

// ScopeRule says whether a kind takes a scope element.
type ScopeRule int

const (
	ScopeForbidden ScopeRule = iota
	ScopeOptional
	ScopeRequired
)

func (k Kind) ScopeRule() ScopeRule {
	switch k {
	case KindContext, KindContainer, KindComponent:
		return ScopeRequired
	case KindDeployment:
		return ScopeOptional
	default:
		return ScopeForbidden
	}
}

type View struct {
	ID             uuid.UUID
	Name           string
	Kind           Kind
	ScopeElementID *uuid.UUID
	Description    string
	// Layers are the ordered layer names of a layers view, top to bottom.
	Layers    []string
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Element is one saved layout row. Included marks custom-view membership.
type Element struct {
	ElementID uuid.UUID
	X, Y      *float64
	W, H      *float64
	Collapsed bool
	Included  bool
}

// Input is the editable part of a view. Members (custom views) is replaced
// when non-nil and left alone when nil.
type Input struct {
	Name           string
	Kind           Kind
	ScopeElementID *uuid.UUID
	Description    string
	Layers         []string
	Members        []uuid.UUID
}
