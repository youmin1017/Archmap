// Package auth signs people in and keeps their sessions. Sign-in methods are
// Providers that return a normalized Identity; the login pipeline in Service
// is the same for all of them (ADR-054): rate limit -> provider -> resolve the
// identity by (issuer, subject) -> provision or refuse -> disabled / restricted
// checks -> role resolution -> server-side session (ADR-055).
package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

// LocalIssuer is the issuer of local (email + password) identities; their subject is
// the normalized login email (ADR-053).
const LocalIssuer = "urn:archmap:local"

// NormalizeLogin is how local login names (emails) are compared and stored as subjects.
func NormalizeLogin(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

type Kind string

const (
	// KindPassword providers take credentials posted to the login endpoint.
	KindPassword Kind = "password"
	// KindRedirect providers send the browser to an identity provider and back to a callback.
	KindRedirect Kind = "redirect"
)

// Info describes a provider to clients (the login page renders from it).
type Info struct {
	ID          string
	DisplayName string
	Kind        Kind
	// Issuer of the identities this provider returns (used when a maintainer links one).
	Issuer string
	// Restricted providers may only be used by maintainers (AUTH_LOCAL_PASSWORD=maintainers).
	Restricted bool
	// UsernameOnly password providers take no password (development sign-in).
	UsernameOnly bool
}

// Identity is what every provider returns, normalized. It never decides a role
// and its email is never used to find an account.
type Identity struct {
	Provider      string
	Issuer        string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	// Claims are the provider's raw claims (groups, ...) for a future RoleResolver.
	Claims map[string]any
}

// Credentials are posted to a password-kind provider.
type Credentials struct {
	Username string
	Password string
}

// Flow is an in-flight redirect login, stored server-side between start and callback.
type Flow struct {
	ID           string
	Provider     string
	State        string
	Nonce        string
	PKCEVerifier string
	ReturnTo     string
	ExpiresAt    time.Time
}

// CompleteInput is either Credentials (password kind) or Callback + Flow (redirect kind).
type CompleteInput struct {
	Credentials Credentials
	Callback    url.Values
	Flow        *Flow
}

// Provider is one way to sign in. OIDC (M15) is another implementation.
type Provider interface {
	Info() Info
	// Begin starts a redirect login: it may fill the flow's nonce / PKCE verifier and
	// returns the URL to send the browser to. Password providers return ErrNotRedirect.
	Begin(ctx context.Context, f *Flow) (redirectURL string, err error)
	// Complete finishes a login and returns the normalized identity, or
	// ErrInvalidCredentials for any credential / callback failure.
	Complete(ctx context.Context, in CompleteInput) (Identity, error)
	// AutoProvision reports whether an unknown identity becomes a new (viewer) account.
	// When false, the identity must have been linked by a maintainer first.
	AutoProvision() bool
}

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrNotRedirect        = errors.New("provider does not use redirects")
)

// SafeReturnTo keeps post-login redirects on this site: only absolute paths
// ("/x"), never scheme-relative ("//host") or backslash tricks. Anything else is "/".
func SafeReturnTo(s string) string {
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.ContainsAny(s, "\\\r\n\t") {
		return "/"
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "/"
	}
	return s
}
