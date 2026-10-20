package apperr_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"archmap/api/internal/platform/apperr"
)

func TestKindOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want apperr.Kind
	}{
		{"invalid", apperr.Invalid("bad %s", "x"), apperr.KindInvalid},
		{"not found", apperr.NotFound("nope"), apperr.KindNotFound},
		{"forbidden", apperr.Forbidden("no"), apperr.KindForbidden},
		{"conflict", apperr.Conflict("clash"), apperr.KindConflict},
		{"unauthenticated", apperr.Unauthenticated("who"), apperr.KindUnauthenticated},
		{"too many requests", apperr.TooManyRequests(time.Minute, "slow down"), apperr.KindTooManyRequests},
		{"wrapped", fmt.Errorf("ctx: %w", apperr.NotFound("nope")), apperr.KindNotFound},
		{"foreign", errors.New("boom"), 0},
		{"nil", nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := apperr.KindOf(tc.err); got != tc.want {
				t.Fatalf("KindOf = %v, want %v", got, tc.want)
			}
		})
	}
}
