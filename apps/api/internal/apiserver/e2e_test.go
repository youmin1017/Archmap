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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"archmap/api/internal/apiserver"
	"archmap/api/internal/auth"
	"archmap/api/internal/auth/authtest"
	"archmap/api/internal/auth/devlogin"
	"archmap/api/internal/auth/local"
	authrepo "archmap/api/internal/auth/repository"
	authtransport "archmap/api/internal/auth/transport"
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
	"archmap/api/internal/settings"
	settingsrepo "archmap/api/internal/settings/repository"
	settingstransport "archmap/api/internal/settings/transport"
	"archmap/api/internal/testdb"
	"archmap/api/internal/user"
	userrepo "archmap/api/internal/user/repository"
	usertransport "archmap/api/internal/user/transport"
	"archmap/api/internal/view"
	viewrepo "archmap/api/internal/view/repository"
	viewtransport "archmap/api/internal/view/transport"
)

const sessionCookie = "archmap_session" // the harness runs with insecure (dev) cookies

// session is a signed-in browser: its cookie and CSRF token.
type session struct {
	token, csrf string
}

type client struct {
	t    *testing.T
	srv  http.Handler
	pool *pgxpool.Pool
	// now is the clock of the auth service; tests move it to expire sessions.
	now time.Time
	// sessions caches one dev-provider session per user ("" key never used).
	sessions map[string]session
	// idp is the fake redirect provider registered as "idp".
	idp *authtest.Provider
}

// do sends a request as the given user ("" = anonymous) and decodes JSON into out when non-nil.
// The user signs in through the dev provider once; the session cookie and CSRF token are reused.
// A user who cannot sign in (unknown, disabled) acts anonymously.
func (c *client) do(method, path, user string, body any, out any) *httptest.ResponseRecorder {
	c.t.Helper()
	var s *session
	if user != "" {
		s = c.session(user)
	}
	return c.raw(method, path, s, body, out, nil)
}

// session returns the cached session of user, signing in when there is none.
func (c *client) session(user string) *session {
	c.t.Helper()
	if s, ok := c.sessions[user]; ok {
		return &s
	}
	var got api.Session
	rec := c.raw("POST", "/api/v1/auth/login", nil, obj{"provider": "dev", "username": user}, &got, nil)
	if rec.Code != http.StatusOK {
		return nil
	}
	s := session{token: cookieValue(rec, sessionCookie), csrf: got.CsrfToken}
	c.sessions[user] = s
	return &s
}

// forget drops a cached session so the next request signs in again.
func (c *client) forget(user string) { delete(c.sessions, user) }

// raw sends one request with an optional session (cookie + CSRF header) and extra headers.
func (c *client) raw(method, path string, s *session, body any, out any, headers map[string]string) *httptest.ResponseRecorder {
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
	if s != nil {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: s.token})
		req.Header.Set(httpx.CSRFHeader, s.csrf)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
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

// cookieValue returns the value the response set for a cookie ("" when absent or cleared).
func cookieValue(rec *httptest.ResponseRecorder, name string) string {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == name && ck.MaxAge >= 0 {
			return ck.Value
		}
	}
	return ""
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

type clientOptions struct {
	mergeAllowSelf bool
	localPassword  bool // restrict local password sign-in to maintainers
}

func newClient(t *testing.T) (*client, string) {
	return newClientWith(t, clientOptions{})
}

// newClientWith builds the full HTTP stack the way cmd/api does, with the dev, local
// and fake redirect ("idp") providers and a controllable clock.
func newClientWith(t *testing.T, o clientOptions) (*client, string) {
	t.Helper()
	pool := testdb.Pool(t)
	actors := testdb.Actors(t, pool)
	c := &client{t: t, pool: pool, now: time.Now(), sessions: map[string]session{}}

	layerRules := layerrule.NewService(layerrulerepo.New(pool))
	csSvc := changeset.NewService(changesetrepo.New(pool), changeset.WithLayerRules(layerRules), changeset.WithSelfMerge(o.mergeAllowSelf),
		changeset.WithPrincipals(userrepo.NewGroups(pool)), changeset.WithPolicy(settingsrepo.New(pool)))
	if _, err := csSvc.Bootstrap(authctx.With(t.Context(), actors[authctx.RoleMaintainer]), "Sample", seed.SampleModel()); err != nil {
		t.Fatal(err)
	}

	repo := authrepo.New(pool)
	localProvider, err := local.New(repo, o.localPassword)
	if err != nil {
		t.Fatal(err)
	}
	c.idp = &authtest.Provider{ID: "idp", Identities: map[string]auth.Identity{}}
	authSvc, err := auth.NewService(repo, []auth.Provider{localProvider, devlogin.New(), c.idp}, auth.Options{
		IdleTimeout: time.Hour, MaxAge: 24 * time.Hour, Now: func() time.Time { return c.now },
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e, err := httpx.New(httpx.Options{
		Logger:      log,
		Registry:    prometheus.NewRegistry(),
		CORSOrigins: []string{"http://localhost:3000"},
		Sessions:    authSvc,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := apiserver.New(
		authtransport.NewHandler(authSvc, log),
		health.NewHandler(health.NewService("test", pool)),
		modeltransport.NewHandler(model.NewService(modelrepo.New(pool))),
		changesettransport.NewHandler(csSvc),
		usertransport.NewHandler(user.NewService(userrepo.New(pool), user.WithProviders(authSvc))),
		usertransport.NewGroupHandler(user.NewGroupService(userrepo.NewGroups(pool))),
		viewtransport.NewHandler(view.NewService(viewrepo.New(pool), apiserver.NewViewModelSource(modelrepo.New(pool), csSvc))),
		layerruletransport.NewHandler(layerRules),
		settingstransport.NewHandler(settings.NewService(settingsrepo.New(pool))),
	)
	api.RegisterHandlers(e, api.NewStrictHandler(server, nil))
	c.srv = e
	return c, "editor@test.local"
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
