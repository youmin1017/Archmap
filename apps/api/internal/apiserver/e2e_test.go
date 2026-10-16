package apiserver_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"archmap/api/internal/apiserver"
	"archmap/api/internal/changeset"
	changesetrepo "archmap/api/internal/changeset/repository"
	changesettransport "archmap/api/internal/changeset/transport"
	"archmap/api/internal/gen/api"
	"archmap/api/internal/health"
	"archmap/api/internal/layerrule"
	layerrulerepo "archmap/api/internal/layerrule/repository"
	layerruletransport "archmap/api/internal/layerrule/transport"
	"archmap/api/internal/model"
	modelrepo "archmap/api/internal/model/repository"
	modeltransport "archmap/api/internal/model/transport"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/platform/httpx"
	"archmap/api/internal/seed"
	"archmap/api/internal/testdb"
	"archmap/api/internal/user"
	userrepo "archmap/api/internal/user/repository"
	usertransport "archmap/api/internal/user/transport"
	"archmap/api/internal/view"
	viewrepo "archmap/api/internal/view/repository"
	viewtransport "archmap/api/internal/view/transport"
)

type client struct {
	t   *testing.T
	srv http.Handler
}

// do sends a request as the given dev user ("" = anonymous) and decodes JSON into out when non-nil.
func (c *client) do(method, path, user string, body any, out any) *httptest.ResponseRecorder {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("X-Dev-User", user)
	}
	rec := httptest.NewRecorder()
	c.srv.ServeHTTP(rec, req)
	if out != nil && rec.Code < 300 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			c.t.Fatalf("decode %s %s: %v\n%s", method, path, err, rec.Body)
		}
	}
	return rec
}

func (c *client) decode(rec *httptest.ResponseRecorder, out any) {
	c.t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		c.t.Fatalf("decode: %v: %s", err, rec.Body)
	}
}

func (c *client) expect(rec *httptest.ResponseRecorder, status int, contentType string) {
	c.t.Helper()
	if rec.Code != status {
		c.t.Fatalf("status = %d, want %d; body=%s", rec.Code, status, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, contentType) {
		c.t.Fatalf("content-type = %q, want %q", got, contentType)
	}
}

func newClient(t *testing.T) (*client, string) {
	t.Helper()
	pool := testdb.Pool(t)
	actors := testdb.Actors(t, pool)

	layerRules := layerrule.NewService(layerrulerepo.New(pool))
	csSvc := changeset.NewService(changesetrepo.New(pool), changeset.WithLayerRules(layerRules))
	if _, err := csSvc.Bootstrap(authctx.With(t.Context(), actors[authctx.RoleMaintainer]), "Sample", seed.SampleModel()); err != nil {
		t.Fatal(err)
	}

	e, err := httpx.New(httpx.Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry:       prometheus.NewRegistry(),
		DevActorLookup: userrepo.New(pool).ActorByEmail,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := apiserver.New(
		health.NewHandler(health.NewService("test", pool)),
		modeltransport.NewHandler(model.NewService(modelrepo.New(pool))),
		changesettransport.NewHandler(csSvc),
		usertransport.NewHandler(user.NewService(userrepo.New(pool))),
		viewtransport.NewHandler(view.NewService(viewrepo.New(pool), apiserver.NewViewModelSource(modelrepo.New(pool), csSvc))),
		layerruletransport.NewHandler(layerRules),
	)
	api.RegisterHandlers(e, api.NewStrictHandler(server, nil))
	return &client{t: t, srv: e}, "editor@test.local"
}

func TestHTTPDraftWorkflow(t *testing.T) {
	c, editor := newClient(t)
	const viewer, maintainer = "viewer@test.local", "maintainer@test.local"

	// Authentication and authorization map to problem+json.
	c.expect(c.do("GET", "/api/v1/model/elements", "", nil, nil), http.StatusUnauthorized, "application/problem+json")
	c.expect(c.do("GET", "/api/v1/me", "ghost@test.local", nil, nil), http.StatusUnauthorized, "application/problem+json")
	c.expect(c.do("POST", "/api/v1/changesets", viewer, obj{"title": "x"}, nil), http.StatusForbidden, "application/problem+json")
	c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"description": "no title"}, nil), http.StatusBadRequest, "application/problem+json")

	var me api.Me
	c.expect(c.do("GET", "/api/v1/me", maintainer, nil, &me), http.StatusOK, "application/json")
	if me.Role != api.Role("maintainer") {
		t.Fatalf("me = %+v", me)
	}

	// Main as seeded.
	var els api.ElementList
	c.expect(c.do("GET", "/api/v1/model/elements?q=api", viewer, nil, &els), http.StatusOK, "application/json")
	if len(els.Elements) != 1 || els.Elements[0].Name != "API" {
		t.Fatalf("search = %+v", els.Elements)
	}
	apiEl := els.Elements[0]
	var single api.Element
	c.expect(c.do("GET", "/api/v1/model/elements/"+apiEl.Id.String(), viewer, nil, &single), http.StatusOK, "application/json")
	c.expect(c.do("GET", "/api/v1/model/elements/00000000-0000-7000-8000-000000000999", viewer, nil, nil), http.StatusNotFound, "application/problem+json")

	// Open a draft and edit it.
	var cs api.Changeset
	c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": "Rename API"}, &cs), http.StatusCreated, "application/json")
	if cs.Status != "draft" || cs.BaseVersion != 1 {
		t.Fatalf("changeset = %+v", cs)
	}
	base := "/api/v1/changesets/" + cs.Id.String()

	var added api.ChangeList
	c.expect(c.do("POST", base+"/changes", editor, obj{"changes": []obj{
		{"op": "update", "target_type": "element", "target_id": apiEl.Id, "patch": obj{"name": "Public API"}},
		{"op": "add", "target_type": "element", "patch": obj{"kind": "container", "name": "Cache", "parent_id": apiEl.ParentId}},
	}}, &added), http.StatusCreated, "application/json")
	if len(added.Changes) != 2 || added.Changes[0].Before == nil || (*added.Changes[0].Before)["name"] != "API" {
		t.Fatalf("changes = %+v", added.Changes)
	}

	// Invalid edits are 400, unknown ids 404, body schema violations 400.
	c.expect(c.do("POST", base+"/changes", editor, obj{"changes": []obj{
		{"op": "add", "target_type": "element", "patch": obj{"kind": "banana", "name": "x"}},
	}}, nil), http.StatusBadRequest, "application/problem+json")
	c.expect(c.do("POST", base+"/changes", editor, obj{"changes": []obj{}}, nil), http.StatusBadRequest, "application/problem+json")
	c.expect(c.do("POST", base+"/changes", viewer, obj{"changes": []obj{
		{"op": "delete", "target_type": "element", "target_id": apiEl.Id},
	}}, nil), http.StatusForbidden, "application/problem+json")

	// Effective model shows the draft; main is unchanged.
	var eff api.Model
	c.expect(c.do("GET", base+"/model", viewer, nil, &eff), http.StatusOK, "application/json")
	names := map[string]bool{}
	for _, e := range eff.Elements {
		names[e.Name] = true
	}
	if !names["Public API"] || !names["Cache"] || names["API"] {
		t.Fatalf("effective names = %v", names)
	}
	c.expect(c.do("GET", "/api/v1/model/elements/"+apiEl.Id.String(), viewer, nil, &single), http.StatusOK, "application/json")
	if single.Name != "API" {
		t.Fatalf("main API renamed to %q", single.Name)
	}

	// Undo / redo round trip.
	var undone api.ChangeList
	c.expect(c.do("DELETE", base+"/changes/last", editor, nil, &undone), http.StatusOK, "application/json")
	if len(undone.Changes) != 2 || undone.Changes[0].UndoneAt == nil {
		t.Fatalf("undone = %+v", undone.Changes)
	}
	c.expect(c.do("DELETE", base+"/changes/last", editor, nil, nil), http.StatusConflict, "application/problem+json")
	c.expect(c.do("POST", base+"/changes/redo", editor, nil, nil), http.StatusOK, "application/json")
	c.expect(c.do("POST", base+"/changes/redo", editor, nil, nil), http.StatusConflict, "application/problem+json")

	// Detail and list.
	var detail api.ChangesetDetail
	c.expect(c.do("GET", base, viewer, nil, &detail), http.StatusOK, "application/json")
	if len(detail.Changes) != 2 {
		t.Fatalf("detail changes = %d", len(detail.Changes))
	}
	var list api.ChangesetList
	c.expect(c.do("GET", "/api/v1/changesets?status=draft", viewer, nil, &list), http.StatusOK, "application/json")
	if len(list.Changesets) != 1 {
		t.Fatalf("list = %+v", list.Changesets)
	}
	c.expect(c.do("GET", "/api/v1/changesets?status=bogus", viewer, nil, nil), http.StatusBadRequest, "application/problem+json")
}

type obj = map[string]any
