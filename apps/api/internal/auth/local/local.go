// Package local is the email + password sign-in provider (argon2id hashes in
// user_identities, issuer auth.LocalIssuer).
package local

import (
	"context"

	"archmap/api/internal/auth"
	"archmap/api/internal/platform/password"
)

// Store finds stored identities (implemented by the auth repository).
type Store interface {
	FindIdentity(ctx context.Context, issuer, subject string) (auth.IdentityRecord, bool, error)
}

type Provider struct {
	store      Store
	restricted bool
	// dummy is verified when the account does not exist, so unknown and known
	// emails take about the same time.
	dummy string
}

// New returns the local provider. restricted limits it to maintainers (AUTH_LOCAL_PASSWORD=maintainers).
func New(store Store, restricted bool) (*Provider, error) {
	dummy, err := password.Hash("archmap-dummy-password")
	if err != nil {
		return nil, err
	}
	return &Provider{store: store, restricted: restricted, dummy: dummy}, nil
}

func (p *Provider) Info() auth.Info {
	name := "Email and password"
	if p.restricted {
		name = "Administrator sign-in"
	}
	return auth.Info{ID: "local", DisplayName: name, Kind: auth.KindPassword, Issuer: auth.LocalIssuer, Restricted: p.restricted}
}

func (p *Provider) Begin(context.Context, *auth.Flow) (string, error) { return "", auth.ErrNotRedirect }

func (p *Provider) AutoProvision() bool { return false }

func (p *Provider) Complete(ctx context.Context, in auth.CompleteInput) (auth.Identity, error) {
	subject := auth.NormalizeLogin(in.Credentials.Username)
	hash := p.dummy
	rec, found, err := p.store.FindIdentity(ctx, auth.LocalIssuer, subject)
	if err != nil {
		return auth.Identity{}, err
	}
	if found && rec.PasswordHash != "" {
		hash = rec.PasswordHash
	}
	ok, err := password.Verify(in.Credentials.Password, hash)
	if err != nil {
		return auth.Identity{}, err
	}
	if !found || !ok || subject == "" {
		return auth.Identity{}, auth.ErrInvalidCredentials
	}
	return auth.Identity{Issuer: auth.LocalIssuer, Subject: subject, Email: subject}, nil
}
