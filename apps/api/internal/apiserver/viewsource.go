package apiserver

import (
	"context"

	"github.com/google/uuid"

	"archmap/api/internal/changeset"
	"archmap/api/internal/model"
	"archmap/api/internal/platform/authctx"
)

type mainReader interface {
	Snapshot(ctx context.Context) (model.Snapshot, error)
}

// ViewModelSource gives views the model they are computed from: main, or a
// draft's effective model. It lives here because it joins two features
// (model and changeset) that must not depend on the view feature.
type ViewModelSource struct {
	main       mainReader
	changesets *changeset.Service
}

func NewViewModelSource(main mainReader, changesets *changeset.Service) *ViewModelSource {
	return &ViewModelSource{main: main, changesets: changesets}
}

func (s *ViewModelSource) Snapshot(ctx context.Context, changesetID *uuid.UUID) (model.Snapshot, error) {
	if _, err := authctx.Require(ctx); err != nil {
		return model.Snapshot{}, err
	}
	if changesetID != nil {
		return s.changesets.EffectiveModel(ctx, *changesetID)
	}
	return s.main.Snapshot(ctx)
}
