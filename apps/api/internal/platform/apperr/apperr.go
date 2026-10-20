// Package apperr defines typed domain errors. Services and repositories return
// them; the HTTP layer (platform/httpx) maps them to RFC 9457 problem+json.
// The package has no HTTP or Echo dependency.
package apperr

import (
	"errors"
	"fmt"
	"time"
)

type Kind int

const (
	KindInvalid Kind = iota + 1
	KindNotFound
	KindForbidden
	KindConflict
	KindUnauthenticated
	KindTooManyRequests
)

func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindNotFound:
		return "not_found"
	case KindForbidden:
		return "forbidden"
	case KindConflict:
		return "conflict"
	case KindUnauthenticated:
		return "unauthenticated"
	case KindTooManyRequests:
		return "too_many_requests"
	default:
		return "unknown"
	}
}

type Error struct {
	Kind    Kind
	Message string
	// RetryAfter is set on KindTooManyRequests errors.
	RetryAfter time.Duration
}

func (e *Error) Error() string { return e.Message }

func Invalid(format string, args ...any) error {
	return &Error{Kind: KindInvalid, Message: fmt.Sprintf(format, args...)}
}

func NotFound(format string, args ...any) error {
	return &Error{Kind: KindNotFound, Message: fmt.Sprintf(format, args...)}
}

func Forbidden(format string, args ...any) error {
	return &Error{Kind: KindForbidden, Message: fmt.Sprintf(format, args...)}
}

func Conflict(format string, args ...any) error {
	return &Error{Kind: KindConflict, Message: fmt.Sprintf(format, args...)}
}

func Unauthenticated(format string, args ...any) error {
	return &Error{Kind: KindUnauthenticated, Message: fmt.Sprintf(format, args...)}
}

// TooManyRequests reports a rate limit; the caller may retry after retryAfter.
func TooManyRequests(retryAfter time.Duration, format string, args ...any) error {
	return &Error{Kind: KindTooManyRequests, Message: fmt.Sprintf(format, args...), RetryAfter: retryAfter}
}

// KindOf returns the Kind of err, or 0 when err is not (or does not wrap) an *Error.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return 0
}
