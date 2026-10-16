package changeset_test

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/apperr"
)

// memRepo is an in-memory changeset.Repository for service tests. InTx is
// serialized by a mutex and does not roll back (tests assert error paths by
// checking that nothing observable was stored).
type memRepo struct {
	mu         sync.Mutex
	main       model.Snapshot
	version    int
	changesets map[uuid.UUID]changeset.Changeset
	changes    []changeset.Change
	snapshots  map[int]model.Snapshot
	versions   []changeset.Version
	resolution map[uuid.UUID][]changeset.Resolution
}

func newMemRepo(main model.Snapshot) *memRepo {
	return &memRepo{
		main: main, changesets: map[uuid.UUID]changeset.Changeset{},
		snapshots:  map[int]model.Snapshot{0: main}, // the initial model is "version 0"
		resolution: map[uuid.UUID][]changeset.Resolution{},
	}
}

func (m *memRepo) InTx(_ context.Context, fn func(changeset.Repository) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := slices.Clone(m.changes)
	if err := fn(m); err != nil {
		m.changes = snapshot // emulate rollback of change rows
		return err
	}
	return nil
}

func (m *memRepo) InsertChangeset(_ context.Context, cs changeset.Changeset) (changeset.Changeset, error) {
	cs.ID, _ = uuid.NewV7()
	cs.CreatedAt = time.Now()
	m.changesets[cs.ID] = cs
	return cs, nil
}

func (m *memRepo) GetChangeset(_ context.Context, id uuid.UUID) (changeset.Changeset, error) {
	cs, ok := m.changesets[id]
	if !ok {
		return changeset.Changeset{}, apperr.NotFound("changeset %s not found", id)
	}
	return cs, nil
}

func (m *memRepo) LockChangeset(ctx context.Context, id uuid.UUID) (changeset.Changeset, error) {
	return m.GetChangeset(ctx, id)
}

func (m *memRepo) ListChangesets(_ context.Context, f changeset.Filter) ([]changeset.Changeset, error) {
	var out []changeset.Changeset
	for _, cs := range m.changesets {
		if f.Status != nil && cs.Status != *f.Status {
			continue
		}
		if f.AuthorID != nil && cs.AuthorID != *f.AuthorID {
			continue
		}
		out = append(out, cs)
	}
	return out, nil
}

func (m *memRepo) CurrentVersion(context.Context) (int, error) { return m.version, nil }

func (m *memRepo) MainSnapshot(context.Context) (model.Snapshot, error) { return m.main, nil }

func (m *memRepo) SnapshotAt(_ context.Context, v int) (model.Snapshot, error) {
	if s, ok := m.snapshots[v]; ok {
		return s, nil
	}
	switch {
	case v == m.version:
		return m.main, nil
	case v == 0:
		return model.Snapshot{}, nil
	}
	return model.Snapshot{}, apperr.NotFound("version %d not found", v)
}

func (m *memRepo) ListVersions(context.Context) ([]changeset.Version, error) {
	out := slices.Clone(m.versions)
	slices.Reverse(out)
	return out, nil
}

func (m *memRepo) LockMain(context.Context) error { return nil }

func (m *memRepo) ApproveChangeset(_ context.Context, id, by uuid.UUID) (changeset.Changeset, error) {
	cs := m.changesets[id]
	now := time.Now()
	cs.ApprovedBy, cs.ApprovedAt = &by, &now
	m.changesets[id] = cs
	return cs, nil
}

func (m *memRepo) SetStatus(_ context.Context, id uuid.UUID, st changeset.Status) (changeset.Changeset, error) {
	cs := m.changesets[id]
	cs.Status = st
	if st == changeset.StatusInReview {
		cs.ApprovedBy, cs.ApprovedAt = nil, nil
	}
	m.changesets[id] = cs
	return cs, nil
}

func (m *memRepo) Resolutions(_ context.Context, id uuid.UUID) ([]changeset.Resolution, error) {
	return m.resolution[id], nil
}

func (m *memRepo) SaveResolutions(_ context.Context, id uuid.UUID, rs []changeset.Resolution) error {
	for _, r := range rs {
		replaced := false
		for i, old := range m.resolution[id] {
			if old.ResolutionKey == r.ResolutionKey {
				m.resolution[id][i] = r
				replaced = true
			}
		}
		if !replaced {
			m.resolution[id] = append(m.resolution[id], r)
		}
	}
	return nil
}

func (m *memRepo) forChangeset(id uuid.UUID, activeOnly bool) []changeset.Change {
	var out []changeset.Change
	for _, c := range m.changes {
		if c.ChangesetID == id && (!activeOnly || c.UndoneAt == nil) {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b changeset.Change) int { return a.Seq - b.Seq })
	return out
}

func (m *memRepo) ListChanges(_ context.Context, id uuid.UUID) ([]changeset.Change, error) {
	return m.forChangeset(id, false), nil
}

func (m *memRepo) ActiveChanges(_ context.Context, id uuid.UUID) ([]changeset.Change, error) {
	return m.forChangeset(id, true), nil
}

func (m *memRepo) MaxSeq(_ context.Context, id uuid.UUID) (int, error) {
	max := 0
	for _, c := range m.forChangeset(id, false) {
		max = c.Seq
	}
	return max, nil
}

func (m *memRepo) InsertChange(_ context.Context, c changeset.Change) (changeset.Change, error) {
	c.ID, _ = uuid.NewV7()
	m.changes = append(m.changes, c)
	return c, nil
}

func (m *memRepo) DeleteUndone(_ context.Context, id uuid.UUID) error {
	m.changes = slices.DeleteFunc(m.changes, func(c changeset.Change) bool {
		return c.ChangesetID == id && c.UndoneAt != nil
	})
	return nil
}

func (m *memRepo) UndoLastBatch(_ context.Context, id uuid.UUID) ([]changeset.Change, error) {
	active := m.forChangeset(id, true)
	if len(active) == 0 {
		return nil, nil
	}
	batch := active[len(active)-1].BatchID
	now := time.Now()
	var out []changeset.Change
	for i := range m.changes {
		c := &m.changes[i]
		if c.ChangesetID == id && c.UndoneAt == nil && c.BatchID == batch {
			c.UndoneAt = &now
			out = append(out, *c)
		}
	}
	return out, nil
}

func (m *memRepo) RedoNextBatch(_ context.Context, id uuid.UUID) ([]changeset.Change, error) {
	var undone []changeset.Change
	for _, c := range m.forChangeset(id, false) {
		if c.UndoneAt != nil {
			undone = append(undone, c)
		}
	}
	if len(undone) == 0 {
		return nil, nil
	}
	batch := undone[0].BatchID // lowest seq = most recently undone
	var out []changeset.Change
	for i := range m.changes {
		c := &m.changes[i]
		if c.ChangesetID == id && c.BatchID == batch && c.UndoneAt != nil {
			c.UndoneAt = nil
			out = append(out, *c)
		}
	}
	return out, nil
}

func (m *memRepo) MaterializeChanges(_ context.Context, changes []changeset.Change) error {
	st, err := changeset.Overlay(m.main, changes)
	if err != nil {
		return err
	}
	m.main = st.Snapshot()
	return nil
}

func (m *memRepo) RecordVersion(_ context.Context, number int, csID, by uuid.UUID, summary string, snap model.Snapshot) error {
	m.version = number
	m.snapshots[number] = snap
	m.versions = append(m.versions, changeset.Version{Number: number, ChangesetID: csID, MergedBy: by, MergedAt: time.Now(), Summary: summary})
	return nil
}

func (m *memRepo) MarkMerged(_ context.Context, id uuid.UUID) error {
	cs := m.changesets[id]
	cs.Status = changeset.StatusMerged
	m.changesets[id] = cs
	return nil
}
