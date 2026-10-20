package httpx

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"archmap/api/internal/platform/apperr"
)

// errorHandler wraps Echo's RFC 9457 handler: typed domain errors (apperr) are
// turned into problem+json with a matching status; everything else falls
// through to Echo's default handling (500 without leaking the message).
func errorHandler() echo.HTTPErrorHandler {
	inner := echo.ProblemDetailsHTTPErrorHandler(false)
	return func(c *echo.Context, err error) {
		var ae *apperr.Error
		if errors.As(err, &ae) {
			if ae.Kind == apperr.KindTooManyRequests && ae.RetryAfter > 0 {
				c.Response().Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(ae.RetryAfter.Seconds()))))
			}
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
	case apperr.KindTooManyRequests:
		return http.StatusTooManyRequests
	}
	return http.StatusInternalServerError
}
