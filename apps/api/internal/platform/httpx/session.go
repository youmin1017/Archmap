package httpx

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
	"archmap/api/internal/platform/reqinfo"
)

// SessionResolver turns a session cookie into an actor and the session's CSRF
// token (implemented by auth.Service).
type SessionResolver interface {
	SessionCookie() string
	CookieSecure() bool
	Resolve(ctx context.Context, token string) (authctx.Actor, string, error)
}

// Problem types the web client reacts to.
const (
	ProblemCSRF           = "https://archmap.dev/problems/csrf"
	ProblemPasswordChange = "https://archmap.dev/problems/password-change-required"
)

// CSRFHeader carries the session's CSRF token on unsafe requests (ADR-056).
const CSRFHeader = "X-CSRF-Token"

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

// requestInfo puts the client IP (via Echo's IP extractor), user agent and
// cookies into the request context for services (platform/reqinfo).
func requestInfo() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			cookies := map[string]string{}
			for _, ck := range req.Cookies() {
				if _, seen := cookies[ck.Name]; !seen {
					cookies[ck.Name] = ck.Value
				}
			}
			info := reqinfo.Info{IP: c.RealIP(), UserAgent: req.UserAgent(), Cookies: cookies}
			c.SetRequest(req.WithContext(reqinfo.With(req.Context(), info)))
			return next(c)
		}
	}
}

// crossOrigin rejects unsafe cross-origin browser requests (Sec-Fetch-Site /
// Origin vs Host). It also protects login, where there is no session yet.
func crossOrigin(trusted []string) (echo.MiddlewareFunc, error) {
	cop := http.NewCrossOriginProtection()
	for _, o := range trusted {
		if err := cop.AddTrustedOrigin(o); err != nil {
			return nil, err
		}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if err := cop.Check(c.Request()); err != nil {
				return &echo.ProblemError{Status: http.StatusForbidden, Type: ProblemCSRF, Detail: "cross-origin request rejected"}
			}
			return next(c)
		}
	}, nil
}

// passwordChangeAllowed lists what a session with a maintainer-set password may still do.
func passwordChangeAllowed(path string) bool {
	return path == "/api/v1/me" || path == "/api/v1/me/password" || strings.HasPrefix(path, "/api/v1/auth/")
}

// sessionAuth resolves the session cookie into the request's actor, then
// requires the session's CSRF token on unsafe /api requests (login excepted:
// it has no session yet and is covered by crossOrigin). An invalid or expired
// cookie is cleared and the request continues anonymously; services answer 401.
func sessionAuth(sessions SessionResolver) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			name := sessions.SessionCookie()
			token := reqinfo.From(req.Context()).Cookie(name)
			if token == "" {
				return next(c)
			}
			actor, csrf, err := sessions.Resolve(req.Context(), token)
			if apperr.KindOf(err) == apperr.KindUnauthenticated {
				c.SetCookie(&http.Cookie{
					Name: name, Value: "", Path: "/", MaxAge: -1,
					HttpOnly: true, Secure: sessions.CookieSecure(), SameSite: http.SameSiteLaxMode,
				})
				return next(c)
			}
			if err != nil {
				return err
			}
			path := req.URL.Path
			if strings.HasPrefix(path, "/api/") && unsafeMethod(req.Method) && path != "/api/v1/auth/login" &&
				subtle.ConstantTimeCompare([]byte(req.Header.Get(CSRFHeader)), []byte(csrf)) != 1 {
				return &echo.ProblemError{Status: http.StatusForbidden, Type: ProblemCSRF, Detail: "missing or invalid CSRF token"}
			}
			if actor.PasswordMustChange && strings.HasPrefix(path, "/api/") && !passwordChangeAllowed(path) {
				return &echo.ProblemError{Status: http.StatusForbidden, Type: ProblemPasswordChange, Detail: "change your password to continue"}
			}
			c.SetRequest(req.WithContext(authctx.With(req.Context(), actor)))
			return next(c)
		}
	}
}
