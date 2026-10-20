// Package devlogin is the development-only sign-in provider: it signs in as an
// existing local account by email without a password. It replaces the old
// X-Dev-User header (ADR-054) and creates an ordinary session, so development
// exercises the real session and CSRF path. cmd/api registers it only when
// AUTH_DEV_LOGIN=true, which config.Load refuses outside APP_ENV=development.
package devlogin

import (
	"context"

	"archmap/api/internal/auth"
)

type Provider struct{}

func New() Provider { return Provider{} }

func (Provider) Info() auth.Info {
	return auth.Info{ID: "dev", DisplayName: "Development sign-in", Kind: auth.KindPassword, Issuer: auth.LocalIssuer, UsernameOnly: true}
}

func (Provider) Begin(context.Context, *auth.Flow) (string, error) { return "", auth.ErrNotRedirect }

// AutoProvision is false: only accounts that already exist can be used.
func (Provider) AutoProvision() bool { return false }

func (Provider) Complete(_ context.Context, in auth.CompleteInput) (auth.Identity, error) {
	subject := auth.NormalizeLogin(in.Credentials.Username)
	if subject == "" {
		return auth.Identity{}, auth.ErrInvalidCredentials
	}
	return auth.Identity{Issuer: auth.LocalIssuer, Subject: subject, Email: subject}, nil
}
