// Package changeset implements drafts: every model modification is a change
// inside a changeset; main is untouched until a merge. It owns the effective
// model (main + a draft's active changes) and per-draft undo / redo.
package changeset

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusInReview  Status = "in_review"
	StatusMerged    Status = "merged"
	StatusRejected  Status = "rejected"
	StatusAbandoned Status = "abandoned"
)

func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusInReview, StatusMerged, StatusRejected, StatusAbandoned:
		return true
	}
	return false
}

type Op string

const (
	OpAdd    Op = "add"
	OpUpdate Op = "update"
	OpDelete Op = "delete"
)

type TargetType string

const (
	TargetElement   TargetType = "element"
	TargetRelation  TargetType = "relation"
	TargetOwner     TargetType = "owner"
	TargetOwnership TargetType = "ownership"
)

type Changeset struct {
	ID          uuid.UUID
	Title       string
	Description string
	AuthorID    uuid.UUID
	BaseVersion int
	Status      Status
	CreatedAt   time.Time
	MergedAt    *time.Time
	ApprovedBy  *uuid.UUID
	ApprovedAt  *time.Time
	// Kind is KindDirect for an automatically merged direct edit of main (ADR-072).
	Kind Kind
	// Reverts is the direct edit this changeset undoes.
	Reverts *uuid.UUID
}

type Kind string

const (
	KindDraft  Kind = "draft"
	KindDirect Kind = "direct"
)

// Version is one merged model version (see model_versions / model_snapshots).
type Version struct {
	Number      int
	ChangesetID uuid.UUID
	MergedBy    uuid.UUID
	MergedAt    time.Time
	Summary     string
	Kind        Kind
	Reverts     *uuid.UUID
}

// Change is one recorded operation. Patch semantics:
//   - add:    full new entity (normalized fields)
//   - update: only the changed fields; Before holds their previous values
//   - delete: empty object; Before holds the full entity snapshot
type Change struct {
	ID          uuid.UUID
	ChangesetID uuid.UUID
	Seq         int
	BatchID     uuid.UUID // one GUI edit; the undo / redo unit
	Op          Op
	TargetType  TargetType
	TargetID    uuid.UUID
	Patch       json.RawMessage
	Before      json.RawMessage
	UndoneAt    *time.Time
	CreatedAt   time.Time
}

// ChangeInput is a requested edit. For add, a zero TargetID means "generate one";
// callers may supply ids so later inputs in the same batch can reference them.
type ChangeInput struct {
	Op         Op
	TargetType TargetType
	TargetID   uuid.UUID
	Patch      json.RawMessage
	// Cascade allows deleting an element that still has children; the
	// descendants are deleted by explicit changes in the same batch.
	Cascade bool
}

type Filter struct {
	Status   *Status
	AuthorID *uuid.UUID
}

// Detail is a changeset with its full change log (undone changes included).
type Detail struct {
	Changeset
	Changes []Change
}
