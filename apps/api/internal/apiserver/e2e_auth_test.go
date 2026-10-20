package apiserver_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"archmap/api/internal/auth"
	"archmap/api/internal/gen/api"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/platform/httpx"
	"archmap/api/internal/testdb"
)

const (
	jsonCT    = "application/json"
	problemCT = "application/problem+json"
)

// login posts credentials and returns the response and the resulting session (nil on failure).
func (c *client) login(provider, username, password string, headers map[string]string) (*httptest.ResponseRecorder, *session) {
	c.t.Helper()
	body := obj{"provider": provider, "username": username}
	if password != "" {
		body["password"] = password
	}
	var got api.Session
	rec := c.raw("POST", "/api/v1/auth/login", nil, body, &got, headers)
	if rec.Code != http.StatusOK {
		return rec, nil
	}
	return rec, &session{token: cookieValue(rec, sessionCookie), csrf: got.CsrfToken}
}

func problemType(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var p struct {
		Type string `json:"type"`
	}
	(&client{t: t}).decode(rec, &p)
	return p.Type
}

func TestHTTPLoginSessionAndCSRF(t *testing.T) {
	c, _ := newClient(t)

	var providers api.AuthProviderList
	c.expect(c.raw("GET", "/api/v1/auth/providers", nil, nil, &providers, nil), http.StatusOK, jsonCT)
	kinds := map[string]api.AuthProviderKind{}
	for _, p := range providers.Providers {
		kinds[p.Id] = p.Kind
	}
	if kinds["local"] != api.Password || kinds["dev"] != api.Password || kinds["idp"] != api.Redirect {
		t.Fatalf("providers = %+v", providers.Providers)
	}

	// Wrong credentials: one generic 401, no cookie.
	for _, cred := range [][2]string{{"editor@test.local", "wrong password!"}, {"nobody@test.local", testdb.Password}} {
		rec, s := c.login("local", cred[0], cred[1], nil)
		c.expect(rec, http.StatusUnauthorized, problemCT)
		if s != nil || !strings.Contains(rec.Body.String(), "invalid email or password") {
			t.Fatalf("login %s: %s", cred[0], rec.Body)
		}
	}

	rec, s := c.login("local", "Editor@Test.local", testdb.Password, nil)
	c.expect(rec, http.StatusOK, jsonCT)
	var cookie *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookie {
			cookie = ck
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || s.csrf == "" {
		t.Fatalf("session cookie = %+v, csrf %q", cookie, s.csrf)
	}

	var cur api.Session
	c.expect(c.raw("GET", "/api/v1/auth/session", s, nil, &cur, nil), http.StatusOK, jsonCT)
	if cur.CsrfToken != s.csrf || cur.Me.Role != "editor" || cur.Me.Provider != "local" {
		t.Fatalf("session = %+v", cur)
	}

	// Unsafe requests need the session's CSRF token.
	draft := obj{"title": "csrf"}
	noToken := &session{token: s.token}
	rec = c.raw("POST", "/api/v1/changesets", noToken, draft, nil, nil)
	c.expect(rec, http.StatusForbidden, problemCT)
	if problemType(t, rec) != httpx.ProblemCSRF {
		t.Fatalf("problem = %s", rec.Body)
	}
	c.expect(c.raw("POST", "/api/v1/changesets", &session{token: s.token, csrf: "forged"}, draft, nil, nil), http.StatusForbidden, problemCT)
	c.expect(c.raw("POST", "/api/v1/changesets", s, draft, nil, nil), http.StatusCreated, jsonCT)
	// Safe methods need no token.
	c.expect(c.raw("GET", "/api/v1/me", noToken, nil, nil, nil), http.StatusOK, jsonCT)

	// Cross-site browser requests are refused even with a valid token, and login too.
	crossSite := map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}
	c.expect(c.raw("POST", "/api/v1/changesets", s, draft, nil, crossSite), http.StatusForbidden, problemCT)
	rec, _ = c.login("local", "editor@test.local", testdb.Password, crossSite)
	c.expect(rec, http.StatusForbidden, problemCT)
	// The configured web origin is trusted.
	rec, _ = c.login("local", "editor@test.local", testdb.Password, map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "http://localhost:3000"})
	c.expect(rec, http.StatusOK, jsonCT)

	// Logout is CSRF-protected, ends the session and clears the cookie.
	c.expect(c.raw("POST", "/api/v1/auth/logout", noToken, nil, nil, nil), http.StatusForbidden, problemCT)
	rec = c.raw("POST", "/api/v1/auth/logout", s, nil, nil, nil)
	if rec.Code != http.StatusNoContent || cookieValue(rec, sessionCookie) != "" {
		t.Fatalf("logout = %d %v", rec.Code, rec.Result().Cookies())
	}
	rec = c.raw("GET", "/api/v1/me", s, nil, nil, nil)
	c.expect(rec, http.StatusUnauthorized, problemCT)
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("stale cookie not cleared: %q", rec.Header().Get("Set-Cookie"))
	}
	c.expect(c.raw("GET", "/api/v1/auth/session", nil, nil, nil, nil), http.StatusUnauthorized, problemCT)
}

func TestHTTPSessionExpiry(t *testing.T) {
	c, editor := newClient(t) // idle 1h, absolute 24h
	c.expect(c.do("GET", "/api/v1/me", editor, nil, nil), http.StatusOK, jsonCT)
	c.now = c.now.Add(59 * time.Minute)
	c.expect(c.do("GET", "/api/v1/me", editor, nil, nil), http.StatusOK, jsonCT)
	c.now = c.now.Add(61 * time.Minute)
	c.expect(c.do("GET", "/api/v1/me", editor, nil, nil), http.StatusUnauthorized, problemCT)

	c.forget(editor)
	c.expect(c.do("GET", "/api/v1/me", editor, nil, nil), http.StatusOK, jsonCT)
	for range 25 { // busy all day: the absolute limit still applies
		c.now = c.now.Add(59 * time.Minute)
		c.do("GET", "/api/v1/me", editor, nil, nil)
	}
	c.expect(c.do("GET", "/api/v1/me", editor, nil, nil), http.StatusUnauthorized, problemCT)
}

func TestHTTPDisableAndRoleChangeApplyImmediately(t *testing.T) {
	c, editor := newClient(t)
	const maintainer = "maintainer@test.local"
	var me api.Me
	c.expect(c.do("GET", "/api/v1/me", editor, nil, &me), http.StatusOK, jsonCT)
	user := "/api/v1/users/" + me.Id.String()

	// A demotion applies to the existing session.
	c.expect(c.do("PUT", user+"/role", maintainer, obj{"role": "viewer"}, nil), http.StatusOK, jsonCT)
	c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": "x"}, nil), http.StatusForbidden, problemCT)
	c.expect(c.do("PUT", user+"/role", maintainer, obj{"role": "editor"}, nil), http.StatusOK, jsonCT)
	c.expect(c.do("POST", "/api/v1/changesets", editor, obj{"title": "x"}, nil), http.StatusCreated, jsonCT)

	// Disabling ends the session at once and blocks new sign-ins until re-enabled.
	var u api.User
	c.expect(c.do("POST", user+"/disable", maintainer, nil, &u), http.StatusOK, jsonCT)
	if !u.Disabled {
		t.Fatalf("user = %+v", u)
	}
	c.expect(c.do("GET", "/api/v1/me", editor, nil, nil), http.StatusUnauthorized, problemCT)
	rec, _ := c.login("local", editor, testdb.Password, nil)
	c.expect(rec, http.StatusForbidden, problemCT)
	c.expect(c.do("POST", user+"/enable", maintainer, nil, nil), http.StatusOK, jsonCT)
	c.forget(editor)
	c.expect(c.do("GET", "/api/v1/me", editor, nil, nil), http.StatusOK, jsonCT)
}

func TestHTTPRoleEscalationAttempts(t *testing.T) {
	c, editor := newClient(t)
	var me api.Me
	c.expect(c.do("GET", "/api/v1/me", editor, nil, &me), http.StatusOK, jsonCT)
	self := "/api/v1/users/" + me.Id.String()

	c.expect(c.do("PUT", self+"/role", editor, obj{"role": "maintainer"}, nil), http.StatusForbidden, problemCT)
	c.expect(c.do("POST", "/api/v1/users", editor, obj{"name": "Sock", "email": "sock@test.local", "role": "maintainer"}, nil), http.StatusForbidden, problemCT)
	c.expect(c.do("POST", self+"/identities", editor, obj{"provider": "idp", "subject": "mine"}, nil), http.StatusForbidden, problemCT)
	c.expect(c.do("POST", self+"/password-reset", editor, obj{}, nil), http.StatusForbidden, problemCT)

	// Claims from an identity provider do not grant roles (stored roles, ADR-058).
	c.idp.Provision = true
	c.idp.Identities["admin-claims"] = auth.Identity{Subject: "sub-claims", Name: "Claimer", Claims: map[string]any{"role": "maintainer", "groups": []string{"maintainers"}}}
	s := c.redirectLogin(t, "admin-claims", "/")
	var got api.Me
	c.expect(c.raw("GET", "/api/v1/me", s, nil, &got, nil), http.StatusOK, jsonCT)
	if got.Role != "viewer" {
		t.Fatalf("provisioned role = %s", got.Role)
	}
}

// redirectLogin runs start + callback against the fake "idp" and returns the session.
func (c *client) redirectLogin(t *testing.T, code, returnTo string) *session {
	t.Helper()
	loc, flow := c.startRedirect(t, returnTo)
	rec := c.callback(flow, url.Values{"code": {code}, "state": {loc.Query().Get("state")}})
	if rec.Code != http.StatusFound || strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Fatalf("callback = %d %s", rec.Code, rec.Header().Get("Location"))
	}
	return &session{token: cookieValue(rec, sessionCookie)}
}

func (c *client) startRedirect(t *testing.T, returnTo string) (*url.URL, string) {
	t.Helper()
	rec := c.raw("GET", "/api/v1/auth/idp/start?return_to="+url.QueryEscape(returnTo), nil, nil, nil, nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return loc, cookieValue(rec, "archmap_flow")
}

func (c *client) callback(flow string, q url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/v1/auth/idp/callback?"+q.Encode(), nil)
	if flow != "" {
		req.AddCookie(&http.Cookie{Name: "archmap_flow", Value: flow})
	}
	rec := httptest.NewRecorder()
	c.srv.ServeHTTP(rec, req)
	return rec
}

func TestHTTPRedirectProviderAndLinking(t *testing.T) {
	c, editor := newClient(t)
	const maintainer = "maintainer@test.local"
	var me api.Me
	c.expect(c.do("GET", "/api/v1/me", editor, nil, &me), http.StatusOK, jsonCT)
	c.idp.Identities["ed"] = auth.Identity{Subject: "ed-sub", Email: editor}
	c.idp.Identities["twin"] = auth.Identity{Subject: "other-sub", Email: editor} // same email, other subject

	// Not linked yet and no provisioning: refused, back to the login page.
	loc, flow := c.startRedirect(t, "/changesets")
	rec := c.callback(flow, url.Values{"code": {"ed"}, "state": {loc.Query().Get("state")}})
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), "/login?error=denied") {
		t.Fatalf("unlinked callback = %d %s", rec.Code, rec.Header().Get("Location"))
	}

	// A maintainer links the identity explicitly; then it signs in as the editor.
	var linked api.Identity
	c.expect(c.do("POST", "/api/v1/users/"+me.Id.String()+"/identities", maintainer, obj{"provider": "idp", "subject": "ed-sub"}, &linked), http.StatusCreated, jsonCT)
	loc, flow = c.startRedirect(t, "/changesets?status=draft")
	rec = c.callback(flow, url.Values{"code": {"ed"}, "state": {loc.Query().Get("state")}})
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/changesets?status=draft" {
		t.Fatalf("callback = %d %s", rec.Code, rec.Header().Get("Location"))
	}
	s := &session{token: cookieValue(rec, sessionCookie)}
	var got api.Me
	c.expect(c.raw("GET", "/api/v1/me", s, nil, &got, nil), http.StatusOK, jsonCT)
	if got.Id != me.Id || got.Provider != "idp" {
		t.Fatalf("me = %+v", got)
	}

	// Same email, different subject: never attached to the editor.
	loc, flow = c.startRedirect(t, "/")
	rec = c.callback(flow, url.Values{"code": {"twin"}, "state": {loc.Query().Get("state")}})
	if !strings.HasPrefix(rec.Header().Get("Location"), "/login?error=denied") {
		t.Fatalf("same-email callback = %s", rec.Header().Get("Location"))
	}
	var ids api.IdentityList
	c.expect(c.do("GET", "/api/v1/users/"+me.Id.String()+"/identities", maintainer, nil, &ids), http.StatusOK, jsonCT)
	if len(ids.Identities) != 2 {
		t.Fatalf("identities = %+v", ids.Identities)
	}

	// Tampering: wrong state, missing flow cookie, replayed flow.
	loc, flow = c.startRedirect(t, "/")
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"wrong state":  c.callback(flow, url.Values{"code": {"ed"}, "state": {"forged"}}),
		"no flow":      c.callback("", url.Values{"code": {"ed"}, "state": {loc.Query().Get("state")}}),
		"replayed":     c.callback(flow, url.Values{"code": {"ed"}, "state": {loc.Query().Get("state")}}),
		"unknown code": c.redirectFail(t, "nope"),
	} {
		if !strings.HasPrefix(rec.Header().Get("Location"), "/login?error=failed") || cookieValue(rec, sessionCookie) != "" {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Header().Get("Location"))
		}
	}

	// Open redirects fall back to "/".
	loc, flow = c.startRedirect(t, "//evil.example/x")
	rec = c.callback(flow, url.Values{"code": {"ed"}, "state": {loc.Query().Get("state")}})
	if rec.Header().Get("Location") != "/" {
		t.Fatalf("return_to = %s", rec.Header().Get("Location"))
	}

	// Unlinking ends the sessions created with the identity.
	c.expect(c.do("DELETE", "/api/v1/users/"+me.Id.String()+"/identities/"+linked.Id.String(), maintainer, nil, nil), http.StatusNoContent, "")
	c.expect(c.raw("GET", "/api/v1/me", s, nil, nil, nil), http.StatusUnauthorized, problemCT)
	c.expect(c.do("GET", "/api/v1/me", editor, nil, nil), http.StatusOK, jsonCT) // the local session lives on
}

func (c *client) redirectFail(t *testing.T, code string) *httptest.ResponseRecorder {
	loc, flow := c.startRedirect(t, "/")
	return c.callback(flow, url.Values{"code": {code}, "state": {loc.Query().Get("state")}})
}

func TestHTTPUserLifecycleAndForcedPasswordChange(t *testing.T) {
	c, _ := newClient(t)
	const maintainer = "maintainer@test.local"

	var created api.CreatedUser
	c.expect(c.do("POST", "/api/v1/users", maintainer, obj{"name": "Nia New", "email": "nia@test.local", "role": "editor"}, &created), http.StatusCreated, jsonCT)
	if created.TemporaryPassword == nil || created.User.Role != "editor" || created.User.IdentityCount != 1 {
		t.Fatalf("created = %+v", created)
	}
	c.expect(c.do("POST", "/api/v1/users", maintainer, obj{"name": "Nia Again", "email": "NIA@test.local", "role": "viewer"}, nil), http.StatusConflict, problemCT)

	rec, s := c.login("local", "nia@test.local", *created.TemporaryPassword, nil)
	c.expect(rec, http.StatusOK, jsonCT)
	var cur api.Session
	c.expect(c.raw("GET", "/api/v1/auth/session", s, nil, &cur, nil), http.StatusOK, jsonCT)
	if !cur.Me.PasswordMustChange {
		t.Fatal("temporary password not flagged")
	}
	// Until the password is changed, only /me, /me/password and /auth/* work.
	rec = c.raw("GET", "/api/v1/model/elements", s, nil, nil, nil)
	c.expect(rec, http.StatusForbidden, problemCT)
	if problemType(t, rec) != httpx.ProblemPasswordChange {
		t.Fatalf("problem = %s", rec.Body)
	}
	c.expect(c.raw("PUT", "/api/v1/me/password", s, obj{"current_password": *created.TemporaryPassword, "new_password": "short"}, nil, nil), http.StatusBadRequest, problemCT)
	c.expect(c.raw("PUT", "/api/v1/me/password", s, obj{"current_password": *created.TemporaryPassword, "new_password": "nia's own passphrase"}, nil, nil), http.StatusNoContent, "")
	c.expect(c.raw("GET", "/api/v1/model/elements", s, nil, nil, nil), http.StatusOK, jsonCT)

	// A maintainer reset ends the user's sessions and forces a change again.
	var reset api.PasswordResetResult
	c.expect(c.do("POST", "/api/v1/users/"+created.User.Id.String()+"/password-reset", maintainer, obj{}, &reset), http.StatusOK, jsonCT)
	c.expect(c.raw("GET", "/api/v1/me", s, nil, nil, nil), http.StatusUnauthorized, problemCT)
	rec, _ = c.login("local", "nia@test.local", "nia's own passphrase", nil)
	c.expect(rec, http.StatusUnauthorized, problemCT)
	rec, s = c.login("local", "nia@test.local", *reset.TemporaryPassword, nil)
	c.expect(rec, http.StatusOK, jsonCT)
	c.expect(c.raw("GET", "/api/v1/model/elements", s, nil, nil, nil), http.StatusForbidden, problemCT)

	var list api.UserList
	c.expect(c.do("GET", "/api/v1/users", maintainer, nil, &list), http.StatusOK, jsonCT)
	for _, u := range list.Users {
		if u.Id == created.User.Id && u.LastLoginAt == nil {
			t.Fatalf("last login not recorded: %+v", u)
		}
	}
}

func TestHTTPLoginRateLimit(t *testing.T) {
	c, editor := newClient(t)
	for range 10 {
		rec, _ := c.login("local", editor, "not the password", nil)
		c.expect(rec, http.StatusUnauthorized, problemCT)
	}
	rec, _ := c.login("local", editor, testdb.Password, nil)
	c.expect(rec, http.StatusTooManyRequests, problemCT)
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 without Retry-After")
	}
	c.now = c.now.Add(16 * time.Minute)
	rec, _ = c.login("local", editor, testdb.Password, nil)
	c.expect(rec, http.StatusOK, jsonCT)
}

func TestHTTPLocalPasswordForMaintainersOnly(t *testing.T) {
	c, editor := newClientWith(t, clientOptions{localPassword: true})
	var providers api.AuthProviderList
	c.expect(c.raw("GET", "/api/v1/auth/providers", nil, nil, &providers, nil), http.StatusOK, jsonCT)
	if p := providers.Providers[0]; p.Id != "local" || !p.Restricted {
		t.Fatalf("local provider = %+v", p)
	}
	rec, _ := c.login("local", editor, testdb.Password, nil)
	c.expect(rec, http.StatusForbidden, problemCT)
	rec, _ = c.login("local", "maintainer@test.local", testdb.Password, nil)
	c.expect(rec, http.StatusOK, jsonCT)
}

// TestHTTPIndependentMaintainerMerge checks the four-eyes rule for protected
// elements with two real maintainer sessions.
func TestHTTPIndependentMaintainerMerge(t *testing.T) {
	for _, allowSelf := range []bool{false, true} {
		name := "self-merge off"
		if allowSelf {
			name = "self-merge on"
		}
		t.Run(name, func(t *testing.T) {
			c, _ := newClientWith(t, clientOptions{mergeAllowSelf: allowSelf})
			const a = "maintainer@test.local"
			const b = "maintainer2@test.local"
			testdb.User(t, c.pool, b, authctx.RoleMaintainer)

			var els api.ElementList
			c.expect(c.do("GET", "/api/v1/model/elements?q=api", a, nil, &els), http.StatusOK, jsonCT)
			apiID := els.Elements[0].Id
			draft := func(patch obj) string {
				var cs api.Changeset
				c.expect(c.do("POST", "/api/v1/changesets", a, obj{"title": "by A"}, &cs), http.StatusCreated, jsonCT)
				path := "/api/v1/changesets/" + cs.Id.String()
				c.expect(c.do("POST", path+"/changes", a, obj{"changes": []obj{
					{"op": "update", "target_type": "element", "target_id": apiID, "patch": patch},
				}}, nil), http.StatusCreated, jsonCT)
				c.expect(c.do("POST", path+"/submit", a, nil, nil), http.StatusOK, jsonCT)
				return path
			}

			// Protect the element first (A authors, B approves and merges).
			protect := draft(obj{"protected": true})
			c.expect(c.do("POST", protect+"/approve", b, nil, nil), http.StatusOK, jsonCT)
			c.expect(c.do("POST", protect+"/merge", b, nil, nil), http.StatusOK, jsonCT)

			// A change to the protected element authored by A.
			edit := draft(obj{"description": "edited while protected"})
			if allowSelf {
				// A may approve their own change, but a protected change approved by its
				// author cannot be merged by anyone (ADR-023).
				c.expect(c.do("POST", edit+"/approve", a, nil, nil), http.StatusOK, jsonCT)
				c.expect(c.do("POST", edit+"/merge", b, nil, nil), http.StatusForbidden, problemCT)
				c.expect(c.do("POST", edit+"/merge", a, nil, nil), http.StatusForbidden, problemCT)
				return
			}
			c.expect(c.do("POST", edit+"/approve", a, nil, nil), http.StatusForbidden, problemCT)
			c.expect(c.do("POST", edit+"/approve", b, nil, nil), http.StatusOK, jsonCT)
			c.expect(c.do("POST", edit+"/merge", a, nil, nil), http.StatusForbidden, problemCT)
			var merged api.MergeResult
			c.expect(c.do("POST", edit+"/merge", b, nil, &merged), http.StatusOK, jsonCT)
			if merged.Changeset.Status != "merged" || merged.Changeset.ApprovedBy == nil || *merged.Changeset.ApprovedBy == merged.Changeset.AuthorId {
				t.Fatalf("merge = %+v", merged.Changeset)
			}
		})
	}
}
