package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/platform/password"
	"archmap/api/internal/platform/reqinfo"
)

// IdentityRecord is a stored identity with the state of its user.
type IdentityRecord struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Role         authctx.Role
	Disabled     bool
	PasswordHash string // local identities only
}

// SessionRecord is a new session row.
type SessionRecord struct {
	TokenHash     []byte
	UserID        uuid.UUID
	IdentityID    uuid.UUID
	Provider      string
	CSRFToken     string
	CreatedAt     time.Time
	IdleExpiresAt time.Time
	ExpiresAt     time.Time
	UserAgent     string
	IP            string
}

// ActiveSession is a live session of an enabled user.
type ActiveSession struct {
	Actor      authctx.Actor
	CSRFToken  string
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

// Repository is the storage the service needs (implemented in ./repository).
type Repository interface {
	FindIdentity(ctx context.Context, issuer, subject string) (IdentityRecord, bool, error)
	// Provision creates a user with the given role and the identity, in one transaction.
	Provision(ctx context.Context, id Identity, role authctx.Role) (IdentityRecord, error)
	RecordLogin(ctx context.Context, rec IdentityRecord, email string, at time.Time) error
	SetRole(ctx context.Context, userID uuid.UUID, role authctx.Role) error

	CreateSession(ctx context.Context, s SessionRecord) error
	// GetSession returns the session unless it is expired or its user is disabled.
	GetSession(ctx context.Context, tokenHash []byte, now time.Time) (ActiveSession, bool, error)
	TouchSession(ctx context.Context, tokenHash []byte, now, idleExpiresAt time.Time) error
	DeleteSession(ctx context.Context, tokenHash []byte) error
	DeleteOtherSessions(ctx context.Context, userID uuid.UUID, keep []byte) error
	DeleteExpired(ctx context.Context, now time.Time) error

	SaveFlow(ctx context.Context, f Flow) error
	// TakeFlow returns and deletes a flow (single use).
	TakeFlow(ctx context.Context, id string) (Flow, bool, error)

	LocalPassword(ctx context.Context, userID uuid.UUID) (identityID uuid.UUID, hash string, ok bool, err error)
	SetPassword(ctx context.Context, identityID uuid.UUID, hash string) error
}

type Options struct {
	IdleTimeout time.Duration
	MaxAge      time.Duration
	// CookieSecure selects the __Host- cookie name and the Secure attribute.
	CookieSecure bool
	Roles        RoleResolver
	// UserLimiter counts failures per provider + username, IPLimiter per client IP.
	UserLimiter Limiter
	IPLimiter   Limiter
	Now         func() time.Time
	Logger      *slog.Logger
}

type Service struct {
	repo      Repository
	providers map[string]Provider
	order     []string
	opts      Options
}

const (
	flowTTL      = 10 * time.Minute
	touchEvery   = time.Minute
	cookieSecure = "__Host-archmap_session"
	cookiePlain  = "archmap_session"
	flowSecure   = "__Host-archmap_flow"
	flowPlain    = "archmap_flow"
)

func NewService(repo Repository, providers []Provider, opts Options) (*Service, error) {
	if opts.IdleTimeout <= 0 || opts.MaxAge <= 0 {
		return nil, errors.New("auth: session timeouts must be positive")
	}
	if opts.Roles == nil {
		opts.Roles = StoredRoles{}
	}
	if opts.UserLimiter == nil {
		opts.UserLimiter = NewWindowLimiter(10, 15*time.Minute)
	}
	if opts.IPLimiter == nil {
		opts.IPLimiter = NewWindowLimiter(50, 15*time.Minute)
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	s := &Service{repo: repo, providers: map[string]Provider{}, opts: opts}
	for _, p := range providers {
		id := p.Info().ID
		if _, dup := s.providers[id]; dup || id == "" {
			return nil, fmt.Errorf("auth: duplicate or empty provider id %q", id)
		}
		s.providers[id] = p
		s.order = append(s.order, id)
	}
	return s, nil
}

// SessionCookie and FlowCookie are the cookie names for this configuration.
func (s *Service) SessionCookie() string {
	if s.opts.CookieSecure {
		return cookieSecure
	}
	return cookiePlain
}

func (s *Service) FlowCookie() string {
	if s.opts.CookieSecure {
		return flowSecure
	}
	return flowPlain
}

func (s *Service) CookieSecure() bool { return s.opts.CookieSecure }

// Providers lists the sign-in methods in configuration order.
func (s *Service) Providers() []Info {
	out := make([]Info, len(s.order))
	for i, id := range s.order {
		out[i] = s.providers[id].Info()
	}
	return out
}

// ExternalIssuer returns the issuer of a redirect-kind provider, for linking identities.
func (s *Service) ExternalIssuer(providerID string) (string, bool) {
	p, ok := s.providers[providerID]
	if !ok || p.Info().Kind != KindRedirect {
		return "", false
	}
	return p.Info().Issuer, true
}

// Session is what a client learns about its session.
type Session struct {
	Actor     authctx.Actor
	CSRFToken string
	ExpiresAt time.Time
}

// LoginResult carries the new raw token (for the cookie) and the session.
type LoginResult struct {
	Token   string
	Session Session
}

// Login signs in with a password-kind provider. oldToken (the browser's current
// session cookie, may be empty) is revoked so a login never reuses a session.
func (s *Service) Login(ctx context.Context, providerID string, cred Credentials, oldToken string) (LoginResult, error) {
	p, ok := s.providers[providerID]
	if !ok || p.Info().Kind != KindPassword {
		return LoginResult{}, apperr.Invalid("unknown sign-in method %q", providerID)
	}
	now := s.opts.Now()
	ipKey := "ip:" + reqinfo.From(ctx).IP
	userKey := "user:" + providerID + ":" + NormalizeLogin(cred.Username)
	if err := s.checkLimits(now, ipKey, userKey); err != nil {
		return LoginResult{}, err
	}
	id, err := p.Complete(ctx, CompleteInput{Credentials: cred})
	if errors.Is(err, ErrInvalidCredentials) {
		s.fail(now, ipKey, userKey)
		return LoginResult{}, apperr.Unauthenticated("invalid email or password")
	}
	if err != nil {
		return LoginResult{}, err
	}
	res, err := s.establish(ctx, p, id, oldToken)
	if err != nil {
		if apperr.KindOf(err) == apperr.KindForbidden {
			s.fail(now, ipKey, userKey)
		}
		return LoginResult{}, err
	}
	s.opts.UserLimiter.Reset(userKey)
	return res, nil
}

// BeginRedirect starts a redirect-kind login and returns the identity provider URL
// and the flow id to keep in the flow cookie.
func (s *Service) BeginRedirect(ctx context.Context, providerID, returnTo string) (redirectURL, flowID string, err error) {
	p, ok := s.providers[providerID]
	if !ok || p.Info().Kind != KindRedirect {
		return "", "", apperr.NotFound("unknown sign-in method %q", providerID)
	}
	f := Flow{
		ID: rand.Text(), Provider: providerID, State: rand.Text(),
		ReturnTo: SafeReturnTo(returnTo), ExpiresAt: s.opts.Now().Add(flowTTL),
	}
	redirectURL, err = p.Begin(ctx, &f)
	if err != nil {
		return "", "", err
	}
	if err := s.repo.SaveFlow(ctx, f); err != nil {
		return "", "", err
	}
	return redirectURL, f.ID, nil
}

// Callback finishes a redirect-kind login. It returns where to send the browser.
func (s *Service) Callback(ctx context.Context, providerID, flowID string, params url.Values, oldToken string) (LoginResult, string, error) {
	p, ok := s.providers[providerID]
	if !ok || p.Info().Kind != KindRedirect {
		return LoginResult{}, "/", apperr.NotFound("unknown sign-in method %q", providerID)
	}
	now := s.opts.Now()
	ipKey := "ip:" + reqinfo.From(ctx).IP
	if err := s.checkLimits(now, ipKey); err != nil {
		return LoginResult{}, "/", err
	}
	if flowID == "" {
		return LoginResult{}, "/", apperr.Unauthenticated("sign-in expired; start again")
	}
	f, found, err := s.repo.TakeFlow(ctx, flowID)
	if err != nil {
		return LoginResult{}, "/", err
	}
	state := params.Get("state")
	if !found || f.Provider != providerID || !now.Before(f.ExpiresAt) ||
		subtle.ConstantTimeCompare([]byte(state), []byte(f.State)) != 1 {
		s.fail(now, ipKey)
		return LoginResult{}, "/", apperr.Unauthenticated("sign-in expired or was tampered with; start again")
	}
	id, err := p.Complete(ctx, CompleteInput{Callback: params, Flow: &f})
	if errors.Is(err, ErrInvalidCredentials) {
		s.fail(now, ipKey)
		return LoginResult{}, f.ReturnTo, apperr.Unauthenticated("the identity provider did not confirm the sign-in")
	}
	if err != nil {
		return LoginResult{}, f.ReturnTo, err
	}
	res, err := s.establish(ctx, p, id, oldToken)
	return res, f.ReturnTo, err
}

// establish is the provider-independent part of every login.
func (s *Service) establish(ctx context.Context, p Provider, id Identity, oldToken string) (LoginResult, error) {
	info := p.Info()
	id.Provider = info.ID
	if id.Issuer == "" || id.Subject == "" {
		return LoginResult{}, fmt.Errorf("auth: provider %s returned an identity without issuer or subject", info.ID)
	}
	// The only lookup key is (issuer, subject); email is never consulted (ADR-059).
	rec, found, err := s.repo.FindIdentity(ctx, id.Issuer, id.Subject)
	if err != nil {
		return LoginResult{}, err
	}
	if !found {
		if !p.AutoProvision() {
			return LoginResult{}, apperr.Forbidden("no account is linked to this sign-in; ask a maintainer to add it")
		}
		if rec, err = s.repo.Provision(ctx, id, s.opts.Roles.Default()); err != nil {
			return LoginResult{}, err
		}
		s.opts.Logger.InfoContext(ctx, "provisioned user", "user_id", rec.UserID, "provider", info.ID)
	}
	if rec.Disabled {
		return LoginResult{}, apperr.Forbidden("this account is disabled")
	}
	role, err := s.opts.Roles.Resolve(ctx, rec.Role, id)
	if err != nil {
		return LoginResult{}, err
	}
	if role != rec.Role {
		if err := s.repo.SetRole(ctx, rec.UserID, role); err != nil {
			return LoginResult{}, err
		}
	}
	if info.Restricted && role != authctx.RoleMaintainer {
		return LoginResult{}, apperr.Forbidden("password sign-in is limited to maintainers; use single sign-on")
	}

	now := s.opts.Now()
	if err := s.repo.RecordLogin(ctx, rec, id.Email, now); err != nil {
		return LoginResult{}, err
	}
	if oldToken != "" {
		if err := s.repo.DeleteSession(ctx, hashToken(oldToken)); err != nil {
			return LoginResult{}, err
		}
	}
	token := rand.Text()
	ri := reqinfo.From(ctx)
	sess := SessionRecord{
		TokenHash: hashToken(token), UserID: rec.UserID, IdentityID: rec.ID, Provider: info.ID,
		CSRFToken: rand.Text(), CreatedAt: now,
		IdleExpiresAt: now.Add(s.opts.IdleTimeout), ExpiresAt: now.Add(s.opts.MaxAge),
		UserAgent: truncate(ri.UserAgent, 512), IP: ri.IP,
	}
	if err := s.repo.CreateSession(ctx, sess); err != nil {
		return LoginResult{}, err
	}
	active, ok, err := s.repo.GetSession(ctx, sess.TokenHash, now)
	if err != nil {
		return LoginResult{}, err
	}
	if !ok {
		return LoginResult{}, errors.New("auth: new session not found")
	}
	return LoginResult{Token: token, Session: Session{Actor: active.Actor, CSRFToken: active.CSRFToken, ExpiresAt: active.ExpiresAt}}, nil
}

// Resolve turns a session cookie into the request's actor and CSRF token.
// Expired sessions, unknown tokens and disabled users are Unauthenticated.
func (s *Service) Resolve(ctx context.Context, token string) (authctx.Actor, string, error) {
	if token == "" {
		return authctx.Actor{}, "", apperr.Unauthenticated("not signed in")
	}
	now := s.opts.Now()
	h := hashToken(token)
	sess, ok, err := s.repo.GetSession(ctx, h, now)
	if err != nil {
		return authctx.Actor{}, "", err
	}
	if !ok {
		return authctx.Actor{}, "", apperr.Unauthenticated("session expired or signed out")
	}
	if now.Sub(sess.LastSeenAt) >= touchEvery {
		if err := s.repo.TouchSession(ctx, h, now, now.Add(s.opts.IdleTimeout)); err != nil {
			return authctx.Actor{}, "", err
		}
	}
	return sess.Actor, sess.CSRFToken, nil
}

// Current returns the caller's session (GET /auth/session).
func (s *Service) Current(ctx context.Context, token string) (Session, error) {
	sess, ok, err := s.repo.GetSession(ctx, hashToken(token), s.opts.Now())
	if err != nil {
		return Session{}, err
	}
	if token == "" || !ok {
		return Session{}, apperr.Unauthenticated("not signed in")
	}
	return Session{Actor: sess.Actor, CSRFToken: sess.CSRFToken, ExpiresAt: sess.ExpiresAt}, nil
}

// Logout deletes the session; signing out without one is not an error.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repo.DeleteSession(ctx, hashToken(token))
}

// ChangePassword changes the caller's local password and ends their other sessions.
func (s *Service) ChangePassword(ctx context.Context, token, current, next string) error {
	actor, err := authctx.Require(ctx)
	if err != nil {
		return err
	}
	now := s.opts.Now()
	key := "password:" + actor.ID.String()
	if err := s.checkLimits(now, key); err != nil {
		return err
	}
	identityID, hash, ok, err := s.repo.LocalPassword(ctx, actor.ID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Conflict("this account has no local password")
	}
	match, err := password.Verify(current, hash)
	if err != nil {
		return err
	}
	if !match {
		s.fail(now, key)
		return apperr.Invalid("the current password is not correct")
	}
	if err := password.Validate(next); err != nil {
		return apperr.Invalid("%s", err.Error())
	}
	if next == current {
		return apperr.Invalid("choose a password different from the current one")
	}
	newHash, err := password.Hash(next)
	if err != nil {
		return err
	}
	if err := s.repo.SetPassword(ctx, identityID, newHash); err != nil {
		return err
	}
	return s.repo.DeleteOtherSessions(ctx, actor.ID, hashToken(token))
}

// Sweep deletes expired sessions and flows.
func (s *Service) Sweep(ctx context.Context) error { return s.repo.DeleteExpired(ctx, s.opts.Now()) }

// RunSweeper calls Sweep every interval until ctx is done.
func (s *Service) RunSweeper(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
				s.opts.Logger.ErrorContext(ctx, "session sweep failed", "error", err)
			}
		}
	}
}

// limiter picks the per-IP limiter for "ip:" keys and the per-account one otherwise.
func (s *Service) limiter(key string) Limiter {
	if strings.HasPrefix(key, "ip:") {
		return s.opts.IPLimiter
	}
	return s.opts.UserLimiter
}

func (s *Service) checkLimits(now time.Time, keys ...string) error {
	for _, key := range keys {
		if wait, blocked := s.limiter(key).Blocked(key, now); blocked {
			return apperr.TooManyRequests(wait, "too many failed sign-in attempts; try again in %s", wait.Round(time.Second))
		}
	}
	return nil
}

func (s *Service) fail(now time.Time, keys ...string) {
	for _, key := range keys {
		s.limiter(key).Fail(key, now)
	}
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
