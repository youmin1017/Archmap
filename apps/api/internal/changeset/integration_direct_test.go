package changeset_test

import (
	"sync"
	"testing"

	"archmap/api/internal/changeset"
	changesetrepo "archmap/api/internal/changeset/repository"
	"archmap/api/internal/platform/authctx"
	settingsrepo "archmap/api/internal/settings/repository"
)

// Concurrent direct edits are serialized by the main lock: both land, as consecutive versions.
func TestIntegrationConcurrentDirectEdits(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	it := newIntegration(t)
	if _, err := it.pool.Exec(t.Context(), `UPDATE settings SET review_policy = 'direct'`); err != nil {
		t.Fatal(err)
	}
	svc := changeset.NewService(changesetrepo.New(it.pool), changeset.WithPolicy(settingsrepo.New(it.pool)))
	api := it.mainElementByName(t, "API")
	web := it.mainElementByName(t, "Web Frontend")

	var wg sync.WaitGroup
	results := make([]changeset.DirectResult, 2)
	errs := make([]error, 2)
	for i, in := range []changeset.ChangeInput{
		updateEl(api.ID, obj{"technology": "Rust"}),
		updateEl(web.ID, obj{"technology": "Svelte"}),
	} {
		wg.Go(func() {
			results[i], errs[i] = svc.ApplyDirect(it.as(authctx.RoleEditor), "", []changeset.ChangeInput{in})
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if v := results[0].Version + results[1].Version; v != 2+3 {
		t.Fatalf("versions %d and %d, want 2 and 3", results[0].Version, results[1].Version)
	}
	if it.mainElementByName(t, "API").Technology != "Rust" || it.mainElementByName(t, "Web Frontend").Technology != "Svelte" {
		t.Fatal("an edit was lost")
	}
	vs, err := svc.Versions(it.as(authctx.RoleViewer))
	if err != nil {
		t.Fatal(err)
	}
	if vs[0].Kind != changeset.KindDirect || vs[len(vs)-1].Kind != changeset.KindDraft {
		t.Fatalf("version kinds: %+v", vs)
	}
}
