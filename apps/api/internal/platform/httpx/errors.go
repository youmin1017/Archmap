package httpx

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"archmap/api/internal/platform/apperr"
	"archmap/api/internal/platform/authctx"
)

// errorHandler wraps Echo's RFC 9457 handler: typed domain errors (apperr) are
// turned into problem+json with a matching status; everything else falls
// through to Echo's default handling (500 without leaking the message).
func errorHandler() echo.HTTPErrorHandler {
	inner := echo.ProblemDetailsHTTPErrorHandler(false)
	return func(c *echo.Context, err error) {
		var ae *apperr.Error
		if errors.As(err, &ae) {
			err = &echo.ProblemError{Status: statusOf(ae.Kind), Detail: err.Error()}
		}
		inner(c, err)
	}
}

func statusOf(k apperr.Kind) int {
	switch k {
	case apperr.KindInvalid:
		return http.StatusBadRequest
	case apperr.KindUnauthenticated:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// ActorLookup resolves an email to an actor (implemented by the user repository).
type ActorLookup func(ctx context.Context, email string) (authctx.Actor, error)

// devAuth is a development-only stand-in for session auth: the X-Dev-User
// header names a seeded user, who becomes the request's actor. Requests
// without the header stay anonymous (services then answer 401).
func devAuth(lookup ActorLookup) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			email := c.Request().Header.Get("X-Dev-User")
			if email == "" {
				return next(c)
			}
			actor, err := lookup(c.Request().Context(), email)
			if err != nil {
				return err
			}
			req := c.Request()
			c.SetRequest(req.WithContext(authctx.With(req.Context(), actor)))
			return next(c)
		}
	}
}
