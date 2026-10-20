// Package authtest provides a scriptable fake auth.Provider for tests. It
// behaves like an external identity provider: as a redirect provider it hands
// out an authorize URL and accepts callbacks whose code names an identity.
package authtest

import (
	"context"
	"net/url"

	"archmap/api/internal/auth"
)

const Issuer = "https://idp.test"

type Provider struct {
	ID         string
	Kind       auth.Kind // default redirect
	Provision  bool
	Restricted bool
	// Identities maps a username (password kind) or callback code (redirect kind) to
	// the identity the provider asserts. Issuer defaults to the package Issuer.
	Identities map[string]auth.Identity
}

func (p *Provider) kind() auth.Kind {
	if p.Kind == "" {
		return auth.KindRedirect
	}
	return p.Kind
}

func (p *Provider) Info() auth.Info {
	return auth.Info{ID: p.ID, DisplayName: "Test IdP " + p.ID, Kind: p.kind(), Issuer: Issuer, Restricted: p.Restricted}
}

func (p *Provider) AutoProvision() bool { return p.Provision }

func (p *Provider) Begin(_ context.Context, f *auth.Flow) (string, error) {
	if p.kind() != auth.KindRedirect {
		return "", auth.ErrNotRedirect
	}
	f.Nonce = "nonce-" + f.State
	return Issuer + "/authorize?state=" + url.QueryEscape(f.State), nil
}

func (p *Provider) Complete(_ context.Context, in auth.CompleteInput) (auth.Identity, error) {
	key := in.Credentials.Username
	if p.kind() == auth.KindRedirect {
		if in.Flow == nil || in.Callback.Get("error") != "" {
			return auth.Identity{}, auth.ErrInvalidCredentials
		}
		key = in.Callback.Get("code")
	}
	id, ok := p.Identities[key]
	if !ok {
		return auth.Identity{}, auth.ErrInvalidCredentials
	}
	if id.Issuer == "" {
		id.Issuer = Issuer
	}
	return id, nil
}
