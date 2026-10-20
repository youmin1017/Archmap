package auth_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/auth"
	"archmap/api/internal/auth/authtest"
	"archmap/api/internal/auth/local"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/platform/password"
	"archmap/api/internal/platform/reqinfo"
)

type fixture struct {
	repo  *memRepo
	svc   *auth.Service
	idp   *authtest.Provider
	clock time.Time
	ctx   context.Context
}

type fixtureOptions struct {
	provision  bool
	restricted bool
	roles      auth.RoleResolver
}

func newFixture(t *testing.T, o fixtureOptions) *fixture {
	t.Helper()
	f := &fixture{repo: newMemRepo(), clock: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)}
	f.idp = &authtest.Provider{ID: "idp", Provision: o.provision, Restricted: o.restricted, Identities: map[string]auth.Identity{}}
	pwIdP := &authtest.Provider{ID: "pw", Kind: auth.KindPassword, Provision: o.provision, Restricted: o.restricted, Identities: f.idp.Identities}
	lp, err := local.New(f.repo, o.restricted)
	if err != nil {
		t.Fatal(err)
	}
	f.svc, err = auth.NewService(f.repo, []auth.Provider{lp, f.idp, pwIdP}, auth.Options{
		IdleTimeout: time.Hour, MaxAge: 8 * time.Hour, Roles: o.roles,
		Now: func() time.Time { return f.clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	f.ctx = reqinfo.With(context.Background(), reqinfo.Info{IP: "203.0.113.7", UserAgent: "test"})
	return f
}

// redirectLogin runs start + callback for an identity code on the "idp" provider.
func (f *fixture) redirectLogin(code string) (auth.LoginResult, error) {
	target, flowID, err := f.svc.BeginRedirect(f.ctx, "idp", "/changesets")
	if err != nil {
		return auth.LoginResult{}, err
	}
	u, _ := url.Parse(target)
	res, _, err := f.svc.Callback(f.ctx, "idp", flowID, url.Values{"code": {code}, "state": {u.Query().Get("state")}}, "")
	return res, err
}

func TestLoginScenarios(t *testing.T) {
	const email = "ada@example.test"
	tests := []struct {
		name string
		opts fixtureOptions
		// existing user (subject "sub-1") and their state; asserted identity is the "code" identity
		existingRole     authctx.Role
		existingDisabled bool
		identity         auth.Identity
		wantKind         apperr.Kind
		wantRole         authctx.Role
		wantSameUser     bool
		wantNewUser      bool
	}{
		{
			name:     "new user is provisioned as viewer",
			opts:     fixtureOptions{provision: true},
			identity: auth.Identity{Subject: "sub-new", Email: "new@example.test", Name: "New"},
			wantRole: authctx.RoleViewer, wantNewUser: true,
		},
		{
			name:     "new user without provisioning is refused",
			opts:     fixtureOptions{provision: false},
			identity: auth.Identity{Subject: "sub-new", Email: "new@example.test"},
			wantKind: apperr.KindForbidden,
		},
		{
			name:         "existing user keeps id and role",
			existingRole: authctx.RoleEditor,
			identity:     auth.Identity{Subject: "sub-1", Email: email},
			wantRole:     authctx.RoleEditor, wantSameUser: true,
		},
		{
			name:         "disabled user is refused",
			existingRole: authctx.RoleEditor, existingDisabled: true,
			identity: auth.Identity{Subject: "sub-1", Email: email},
			wantKind: apperr.KindForbidden,
		},
		{
			name:         "same email, other subject is never linked (provisioned separately)",
			opts:         fixtureOptions{provision: true},
			existingRole: authctx.RoleMaintainer,
			identity:     auth.Identity{Subject: "sub-2", Email: email, EmailVerified: true},
			wantRole:     authctx.RoleViewer, wantNewUser: true,
		},
		{
			name:         "same email, other subject without provisioning is refused",
			existingRole: authctx.RoleMaintainer,
			identity:     auth.Identity{Subject: "sub-2", Email: email, EmailVerified: true},
			wantKind:     apperr.KindForbidden,
		},
		{
			name:     "claims cannot escalate a new user",
			opts:     fixtureOptions{provision: true},
			identity: auth.Identity{Subject: "sub-new", Claims: map[string]any{"role": "maintainer", "groups": []string{"admins"}}},
			wantRole: authctx.RoleViewer, wantNewUser: true,
		},
		{
			name:         "claims cannot escalate an existing user",
			existingRole: authctx.RoleViewer,
			identity:     auth.Identity{Subject: "sub-1", Email: email, Claims: map[string]any{"role": "maintainer"}},
			wantRole:     authctx.RoleViewer, wantSameUser: true,
		},
		{
			name:         "restricted provider refuses non-maintainers",
			opts:         fixtureOptions{restricted: true},
			existingRole: authctx.RoleEditor,
			identity:     auth.Identity{Subject: "sub-1", Email: email},
			wantKind:     apperr.KindForbidden,
		},
		{
			name:         "restricted provider admits maintainers",
			opts:         fixtureOptions{restricted: true},
			existingRole: authctx.RoleMaintainer,
			identity:     auth.Identity{Subject: "sub-1", Email: email},
			wantRole:     authctx.RoleMaintainer, wantSameUser: true,
		},
	}
	for _, tc := range tests {
		for _, kind := range []string{"redirect", "password"} {
			t.Run(tc.name+"/"+kind, func(t *testing.T) {
				f := newFixture(t, tc.opts)
				var existing uuid.UUID
				if tc.existingRole != "" {
					existing = f.repo.addUser("Ada", email, tc.existingRole, authtest.Issuer, "sub-1", "")
					f.repo.users[existing].disabled = tc.existingDisabled
				}
				f.idp.Identities["code"] = tc.identity
				usersBefore := len(f.repo.users)

				var res auth.LoginResult
				var err error
				if kind == "redirect" {
					res, err = f.redirectLogin("code")
				} else {
					res, err = f.svc.Login(f.ctx, "pw", auth.Credentials{Username: "code"}, "")
				}

				if tc.wantKind != 0 {
					if apperr.KindOf(err) != tc.wantKind {
						t.Fatalf("err = %v, want kind %v", err, tc.wantKind)
					}
					if f.repo.sessionCount() != 0 || len(f.repo.users) != usersBefore {
						t.Fatalf("refused login left %d sessions, %d users (before %d)", f.repo.sessionCount(), len(f.repo.users), usersBefore)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				a := res.Session.Actor
				if a.Role != tc.wantRole || res.Token == "" || res.Session.CSRFToken == "" {
					t.Fatalf("session = %+v", res.Session)
				}
				if tc.wantSameUser && a.ID != existing {
					t.Fatalf("logged in as %s, want existing %s", a.ID, existing)
				}
				if tc.wantNewUser && (a.ID == existing || len(f.repo.users) != usersBefore+1) {
					t.Fatalf("want a new user; got %s (existing %s), users %d -> %d", a.ID, existing, usersBefore, len(f.repo.users))
				}
				if existing != uuid.Nil {
					if n := len(f.repo.identitiesOf(existing)); n != 1 {
						t.Fatalf("existing user has %d identities, want 1 (nothing linked)", n)
					}
					if f.repo.users[existing].role != tc.existingRole {
						t.Fatalf("existing role changed to %s", f.repo.users[existing].role)
					}
				}
				got, csrf, err := f.svc.Resolve(f.ctx, res.Token)
				if err != nil || got.ID != a.ID || csrf != res.Session.CSRFToken {
					t.Fatalf("Resolve = %+v, %q, %v", got, csrf, err)
				}
			})
		}
	}
}

func TestLocalLogin(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	hash, err := password.Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	id := f.repo.addUser("Ada", "ada@example.test", authctx.RoleEditor, auth.LocalIssuer, "ada@example.test", hash)

	tests := []struct {
		name, user, pass string
		wantErr          apperr.Kind
	}{
		{"correct, case and spaces normalized", "  ADA@example.test ", "correct horse battery", 0},
		{"wrong password", "ada@example.test", "wrong horse battery", apperr.KindUnauthenticated},
		{"unknown user", "bob@example.test", "correct horse battery", apperr.KindUnauthenticated},
		{"empty", "", "", apperr.KindUnauthenticated},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := f.svc.Login(f.ctx, "local", auth.Credentials{Username: tc.user, Password: tc.pass}, "")
			if apperr.KindOf(err) != tc.wantErr {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err == nil && res.Session.Actor.ID != id {
				t.Fatalf("actor = %+v", res.Session.Actor)
			}
			if err != nil && err.Error() != "invalid email or password" {
				t.Fatalf("credential errors must be generic, got %q", err)
			}
		})
	}
	if _, err := f.svc.Login(f.ctx, "idp", auth.Credentials{Username: "x"}, ""); apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("password login on a redirect provider: %v", err)
	}
}

func TestLoginRotatesSession(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	f.repo.addUser("Ada", "a@x.test", authctx.RoleViewer, authtest.Issuer, "sub-1", "")
	f.idp.Identities["code"] = auth.Identity{Subject: "sub-1"}
	first, err := f.svc.Login(f.ctx, "pw", auth.Credentials{Username: "code"}, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.Login(f.ctx, "pw", auth.Credentials{Username: "code"}, first.Token)
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token {
		t.Fatal("login reused the session token")
	}
	if _, _, err := f.svc.Resolve(f.ctx, first.Token); apperr.KindOf(err) != apperr.KindUnauthenticated {
		t.Fatalf("old session still valid: %v", err)
	}
}

func TestCallbackRejects(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	f.repo.addUser("Ada", "a@x.test", authctx.RoleViewer, authtest.Issuer, "sub-1", "")
	f.idp.Identities["code"] = auth.Identity{Subject: "sub-1"}

	start := func(t *testing.T) (state, flowID string) {
		target, flowID, err := f.svc.BeginRedirect(f.ctx, "idp", "/x")
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(target)
		return u.Query().Get("state"), flowID
	}
	tests := []struct {
		name string
		run  func(t *testing.T) error
	}{
		{"missing flow cookie", func(t *testing.T) error {
			state, _ := start(t)
			_, _, err := f.svc.Callback(f.ctx, "idp", "", url.Values{"code": {"code"}, "state": {state}}, "")
			return err
		}},
		{"wrong state", func(t *testing.T) error {
			_, flowID := start(t)
			_, _, err := f.svc.Callback(f.ctx, "idp", flowID, url.Values{"code": {"code"}, "state": {"forged"}}, "")
			return err
		}},
		{"expired flow", func(t *testing.T) error {
			state, flowID := start(t)
			f.clock = f.clock.Add(11 * time.Minute)
			_, _, err := f.svc.Callback(f.ctx, "idp", flowID, url.Values{"code": {"code"}, "state": {state}}, "")
			return err
		}},
		{"replayed flow", func(t *testing.T) error {
			state, flowID := start(t)
			q := url.Values{"code": {"code"}, "state": {state}}
			if _, _, err := f.svc.Callback(f.ctx, "idp", flowID, q, ""); err != nil {
				t.Fatal(err)
			}
			_, _, err := f.svc.Callback(f.ctx, "idp", flowID, q, "")
			return err
		}},
		{"identity provider error", func(t *testing.T) error {
			state, flowID := start(t)
			_, _, err := f.svc.Callback(f.ctx, "idp", flowID, url.Values{"error": {"access_denied"}, "state": {state}}, "")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(t); apperr.KindOf(err) != apperr.KindUnauthenticated {
				t.Fatalf("err = %v, want unauthenticated", err)
			}
		})
	}
	if _, _, err := f.svc.BeginRedirect(f.ctx, "local", "/"); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("redirect on a password provider: %v", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	f := newFixture(t, fixtureOptions{}) // idle 1h, absolute 8h
	f.repo.addUser("Ada", "a@x.test", authctx.RoleViewer, authtest.Issuer, "sub-1", "")
	f.idp.Identities["code"] = auth.Identity{Subject: "sub-1"}
	login := func() string {
		res, err := f.svc.Login(f.ctx, "pw", auth.Credentials{Username: "code"}, "")
		if err != nil {
			t.Fatal(err)
		}
		return res.Token
	}
	resolve := func(token string) error { _, _, err := f.svc.Resolve(f.ctx, token); return err }

	// Idle: activity slides the idle deadline; 61 quiet minutes end the session.
	tok := login()
	for range 3 {
		f.clock = f.clock.Add(50 * time.Minute)
		if err := resolve(tok); err != nil {
			t.Fatalf("active session expired: %v", err)
		}
	}
	f.clock = f.clock.Add(61 * time.Minute)
	if err := resolve(tok); apperr.KindOf(err) != apperr.KindUnauthenticated {
		t.Fatalf("idle session err = %v", err)
	}

	// Absolute: even a busy session ends after the max age.
	tok = login()
	for range 9 {
		f.clock = f.clock.Add(55 * time.Minute)
		_ = resolve(tok)
	}
	if err := resolve(tok); apperr.KindOf(err) != apperr.KindUnauthenticated {
		t.Fatalf("session older than max age err = %v", err)
	}

	// Logout ends a session; the sweeper removes expired rows.
	tok = login()
	if err := f.svc.Logout(f.ctx, tok); err != nil {
		t.Fatal(err)
	}
	if err := resolve(tok); apperr.KindOf(err) != apperr.KindUnauthenticated {
		t.Fatalf("logged-out session err = %v", err)
	}
	if err := f.svc.Sweep(f.ctx); err != nil || f.repo.sessionCount() != 0 {
		t.Fatalf("sweep: %v, %d sessions left", err, f.repo.sessionCount())
	}
}

func TestDisabledUserLosesSession(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	id := f.repo.addUser("Ada", "a@x.test", authctx.RoleEditor, authtest.Issuer, "sub-1", "")
	f.idp.Identities["code"] = auth.Identity{Subject: "sub-1"}
	res, err := f.svc.Login(f.ctx, "pw", auth.Credentials{Username: "code"}, "")
	if err != nil {
		t.Fatal(err)
	}
	f.repo.users[id].disabled = true
	if _, _, err := f.svc.Resolve(f.ctx, res.Token); apperr.KindOf(err) != apperr.KindUnauthenticated {
		t.Fatalf("disabled user's session err = %v", err)
	}
}

func TestRateLimit(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	hash, _ := password.Hash("correct horse battery")
	f.repo.addUser("Ada", "ada@x.test", authctx.RoleViewer, auth.LocalIssuer, "ada@x.test", hash)
	try := func(pw string) error {
		_, err := f.svc.Login(f.ctx, "local", auth.Credentials{Username: "ada@x.test", Password: pw}, "")
		return err
	}
	for i := range 10 {
		if err := try("wrong"); apperr.KindOf(err) != apperr.KindUnauthenticated {
			t.Fatalf("attempt %d: %v", i+1, err)
		}
	}
	err := try("correct horse battery")
	var ae *apperr.Error
	if apperr.KindOf(err) != apperr.KindTooManyRequests || !asAppErr(err, &ae) || ae.RetryAfter <= 0 {
		t.Fatalf("11th attempt err = %v, want 429 with retry-after", err)
	}
	f.clock = f.clock.Add(16 * time.Minute)
	if err := try("correct horse battery"); err != nil {
		t.Fatalf("after the window: %v", err)
	}
	// A success clears the per-account counter.
	for range 9 {
		_ = try("wrong")
	}
	if err := try("correct horse battery"); err != nil {
		t.Fatal(err)
	}
	for range 9 {
		_ = try("wrong")
	}
	if err := try("correct horse battery"); err != nil {
		t.Fatalf("counter not reset by success: %v", err)
	}
}

func asAppErr(err error, out **apperr.Error) bool {
	ae, ok := err.(*apperr.Error)
	*out = ae
	return ok
}

func TestChangePassword(t *testing.T) {
	f := newFixture(t, fixtureOptions{})
	hash, _ := password.Hash("correct horse battery")
	f.repo.addUser("Ada", "ada@x.test", authctx.RoleViewer, auth.LocalIssuer, "ada@x.test", hash)
	login := func(pw string) (auth.LoginResult, error) {
		return f.svc.Login(f.ctx, "local", auth.Credentials{Username: "ada@x.test", Password: pw}, "")
	}
	a, err := login("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	b, err := login("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	ctx := authctx.With(f.ctx, a.Session.Actor)

	for _, tc := range []struct{ name, current, next, want string }{
		{"wrong current", "nope", "a brand new passphrase", "not correct"},
		{"too short", "correct horse battery", "short", "at least 12"},
		{"unchanged", "correct horse battery", "correct horse battery", "different"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := f.svc.ChangePassword(ctx, a.Token, tc.current, tc.next)
			if apperr.KindOf(err) != apperr.KindInvalid || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	if err := f.svc.ChangePassword(ctx, a.Token, "correct horse battery", "a brand new passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Resolve(f.ctx, a.Token); err != nil {
		t.Fatalf("current session ended: %v", err)
	}
	if _, _, err := f.svc.Resolve(f.ctx, b.Token); apperr.KindOf(err) != apperr.KindUnauthenticated {
		t.Fatalf("other session survived: %v", err)
	}
	if _, err := login("a brand new passphrase"); err != nil {
		t.Fatal(err)
	}
}

// groupRoles is what an M15 claims mapper could look like; it proves the extension point.
type groupRoles struct{}

func (groupRoles) Default() authctx.Role { return authctx.RoleViewer }

func (groupRoles) Resolve(_ context.Context, stored authctx.Role, id auth.Identity) (authctx.Role, error) {
	if g, _ := id.Claims["groups"].([]string); len(g) > 0 && g[0] == "architects" {
		return authctx.RoleEditor, nil
	}
	return stored, nil
}

func TestRoleResolverExtensionPoint(t *testing.T) {
	f := newFixture(t, fixtureOptions{provision: true, roles: groupRoles{}})
	f.idp.Identities["code"] = auth.Identity{Subject: "sub-9", Claims: map[string]any{"groups": []string{"architects"}}}
	res, err := f.redirectLogin("code")
	if err != nil {
		t.Fatal(err)
	}
	if res.Session.Actor.Role != authctx.RoleEditor || f.repo.users[res.Session.Actor.ID].role != authctx.RoleEditor {
		t.Fatalf("role = %s (stored %s)", res.Session.Actor.Role, f.repo.users[res.Session.Actor.ID].role)
	}
}

func TestSafeReturnTo(t *testing.T) {
	for in, want := range map[string]string{
		"/changesets/1?x=y":        "/changesets/1?x=y",
		"/":                        "/",
		"":                         "/",
		"//evil.example":           "/",
		"/\\evil.example":          "/",
		"https://evil.example/":    "/",
		"javascript:alert(1)":      "/",
		"changesets":               "/",
		"/ok\r\nSet-Cookie: x=y":   "/",
		"/%2F%2Fevil.example/path": "/%2F%2Fevil.example/path",
	} {
		if got := auth.SafeReturnTo(in); got != want {
			t.Errorf("SafeReturnTo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWindowLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := auth.NewWindowLimiter(2, time.Minute)
	l.Fail("k", now)
	if _, blocked := l.Blocked("k", now); blocked {
		t.Fatal("blocked after 1 failure")
	}
	l.Fail("k", now.Add(10*time.Second))
	wait, blocked := l.Blocked("k", now.Add(20*time.Second))
	if !blocked || wait != 40*time.Second {
		t.Fatalf("Blocked = %v, %v", wait, blocked)
	}
	if _, blocked := l.Blocked("other", now); blocked {
		t.Fatal("keys are independent")
	}
	if _, blocked := l.Blocked("k", now.Add(time.Minute)); blocked {
		t.Fatal("window did not reset")
	}
	l.Reset("k")
	if _, blocked := l.Blocked("k", now.Add(20*time.Second)); blocked {
		t.Fatal("Reset did not clear")
	}
}
