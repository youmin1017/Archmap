package user_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"archmap/api/internal/auth"
	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/testdb"
	"archmap/api/internal/user"
	userrepo "archmap/api/internal/user/repository"
)

type issuers map[string]string

func (i issuers) ExternalIssuer(id string) (string, bool) { v, ok := i[id]; return v, ok }

type env struct {
	svc    *user.Service
	m1, m2 authctx.Actor // maintainers with local passwords
	editor authctx.Actor
}

func newEnv(t *testing.T) env {
	t.Helper()
	pool := testdb.Pool(t)
	return env{
		svc:    user.NewService(userrepo.New(pool), user.WithProviders(issuers{"idp": "https://idp.test"})),
		m1:     testdb.User(t, pool, "m1@test.local", authctx.RoleMaintainer),
		m2:     testdb.User(t, pool, "m2@test.local", authctx.RoleMaintainer),
		editor: testdb.User(t, pool, "ed@test.local", authctx.RoleEditor),
	}
}

func (e env) as(a authctx.Actor) context.Context { return authctx.With(context.Background(), a) }

func localIdentity(t *testing.T, e env, id uuid.UUID) uuid.UUID {
	t.Helper()
	ids, err := e.svc.Identities(e.as(e.m1), id)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range ids {
		if x.Issuer == auth.LocalIssuer {
			return x.ID
		}
	}
	t.Fatalf("user %s has no local identity", id)
	return uuid.Nil
}

// TestBreakGlassInvariant: at least one enabled maintainer with a local password always remains.
func TestBreakGlassInvariant(t *testing.T) {
	tests := []struct {
		name string
		// prepare runs first (as m1) and must succeed; act is checked against want.
		prepare func(t *testing.T, e env)
		act     func(t *testing.T, e env) error
		want    apperr.Kind
	}{
		{
			name: "demote one of two maintainers",
			act: func(t *testing.T, e env) error {
				_, err := e.svc.SetRole(e.as(e.m1), e.m2.ID, authctx.RoleEditor)
				return err
			},
		},
		{
			name: "demote the last maintainer",
			prepare: func(t *testing.T, e env) {
				_, err := e.svc.SetRole(e.as(e.m1), e.m2.ID, authctx.RoleEditor)
				noErr(t, err)
			},
			act: func(t *testing.T, e env) error {
				_, err := e.svc.SetRole(e.as(e.m1), e.m1.ID, authctx.RoleViewer)
				return err
			},
			want: apperr.KindConflict,
		},
		{
			name:    "disable the other maintainer when it is the last one with a password",
			prepare: func(t *testing.T, e env) { noErr(t, e.svc.Unlink(e.as(e.m2), e.m1.ID, localIdentity(t, e, e.m1.ID))) },
			act:     func(t *testing.T, e env) error { _, err := e.svc.Disable(e.as(e.m1), e.m2.ID); return err },
			want:    apperr.KindConflict,
		},
		{
			name:    "unlink the last local password of the last break-glass maintainer",
			prepare: func(t *testing.T, e env) { _, err := e.svc.Disable(e.as(e.m1), e.m2.ID); noErr(t, err) },
			act: func(t *testing.T, e env) error {
				return e.svc.Unlink(e.as(e.m1), e.m1.ID, localIdentity(t, e, e.m1.ID))
			},
			want: apperr.KindConflict,
		},
		{
			name: "unlink a maintainer's password while another break-glass maintainer exists",
			act: func(t *testing.T, e env) error {
				return e.svc.Unlink(e.as(e.m1), e.m2.ID, localIdentity(t, e, e.m2.ID))
			},
		},
		{
			name: "disable yourself",
			act:  func(t *testing.T, e env) error { _, err := e.svc.Disable(e.as(e.m1), e.m1.ID); return err },
			want: apperr.KindConflict,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			if tc.prepare != nil {
				tc.prepare(t, e)
			}
			if err := tc.act(t, e); apperr.KindOf(err) != tc.want {
				t.Fatalf("err = %v, want kind %v", err, tc.want)
			}
		})
	}
}

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestManagementNeedsMaintainer: no user-management operation is open to other roles.
func TestManagementNeedsMaintainer(t *testing.T) {
	e := newEnv(t)
	ctx := e.as(e.editor)
	ops := map[string]func() error{
		"list":    func() error { _, err := e.svc.List(ctx); return err },
		"promote": func() error { _, err := e.svc.SetRole(ctx, e.editor.ID, authctx.RoleMaintainer); return err },
		"create": func() error {
			_, _, err := e.svc.Create(ctx, user.NewUser{Name: "x", Email: "x@test.local", Role: authctx.RoleMaintainer})
			return err
		},
		"disable":    func() error { _, err := e.svc.Disable(ctx, e.m1.ID); return err },
		"enable":     func() error { _, err := e.svc.Enable(ctx, e.m1.ID); return err },
		"reset":      func() error { _, err := e.svc.ResetPassword(ctx, e.m1.ID, ""); return err },
		"identities": func() error { _, err := e.svc.Identities(ctx, e.m1.ID); return err },
		"link":       func() error { _, err := e.svc.Link(ctx, e.editor.ID, "idp", "me"); return err },
		"unlink":     func() error { return e.svc.Unlink(ctx, e.m1.ID, uuid.New()) },
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			if err := op(); apperr.KindOf(err) != apperr.KindForbidden {
				t.Fatalf("err = %v, want forbidden", err)
			}
		})
	}
	if _, err := e.svc.List(context.Background()); apperr.KindOf(err) != apperr.KindUnauthenticated {
		t.Fatalf("anonymous list: %v", err)
	}
}

func TestCreateResetLink(t *testing.T) {
	e := newEnv(t)
	ctx := e.as(e.m1)

	tests := []struct {
		name string
		in   user.NewUser
		want apperr.Kind
	}{
		{"generated password", user.NewUser{Name: "Ann", Email: " Ann@Test.local ", Role: authctx.RoleEditor}, 0},
		{"chosen password", user.NewUser{Name: "Bo", Email: "bo@test.local", Role: authctx.RoleViewer, Password: "long enough secret"}, 0},
		{"short password", user.NewUser{Name: "Cy", Email: "cy@test.local", Role: authctx.RoleViewer, Password: "short"}, apperr.KindInvalid},
		{"duplicate sign-in email", user.NewUser{Name: "Ann 2", Email: "ann@test.local", Role: authctx.RoleViewer}, apperr.KindConflict},
		{"missing name", user.NewUser{Email: "dee@test.local", Role: authctx.RoleViewer}, apperr.KindInvalid},
		{"bad email", user.NewUser{Name: "Eve", Email: "eve", Role: authctx.RoleViewer}, apperr.KindInvalid},
		{"bad role", user.NewUser{Name: "Fay", Email: "fay@test.local", Role: "root"}, apperr.KindInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, temp, err := e.svc.Create(ctx, tc.in)
			if apperr.KindOf(err) != tc.want {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if err != nil {
				return
			}
			if (tc.in.Password == "") != (temp != "") {
				t.Fatalf("temporary password = %q for chosen %q", temp, tc.in.Password)
			}
			if u.Role != tc.in.Role || u.IdentityCount != 1 || u.Disabled {
				t.Fatalf("user = %+v", u)
			}
		})
	}

	// Linking is explicit and keyed by (issuer, subject); a taken identity conflicts.
	x, err := e.svc.Link(ctx, e.editor.ID, "idp", "sub-123")
	if err != nil || x.Issuer != "https://idp.test" || x.Subject != "sub-123" {
		t.Fatalf("link = %+v, %v", x, err)
	}
	if _, err := e.svc.Link(ctx, e.m2.ID, "idp", "sub-123"); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("second link of the same identity: %v", err)
	}
	if _, err := e.svc.Link(ctx, e.m2.ID, "local", "m2@test.local"); apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("link of a non-external provider: %v", err)
	}
	if err := e.svc.Unlink(ctx, e.editor.ID, x.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Unlink(ctx, e.m2.ID, x.ID); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("unlink of another user's identity: %v", err)
	}

	temp, err := e.svc.ResetPassword(ctx, e.editor.ID, "")
	if err != nil || len(temp) < 12 {
		t.Fatalf("reset = %q, %v", temp, err)
	}
	if _, err := e.svc.ResetPassword(ctx, e.editor.ID, "tiny"); apperr.KindOf(err) != apperr.KindInvalid {
		t.Fatalf("short reset: %v", err)
	}
	if _, err := e.svc.Disable(ctx, uuid.New()); apperr.KindOf(err) != apperr.KindNotFound {
		t.Fatalf("disable unknown: %v", err)
	}
}
