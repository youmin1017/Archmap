package password

import (
	"crypto/rand"
	"fmt"
	"unicode/utf8"
)

// Policy limits (NIST SP 800-63B: length only, no composition rules).
const (
	MinLength = 12
	MaxLength = 256
)

// Validate checks a new password against the policy. The message is safe to show to users.
func Validate(pw string) error {
	switch {
	case utf8.RuneCountInString(pw) < MinLength:
		return fmt.Errorf("password must be at least %d characters", MinLength)
	case len(pw) > MaxLength:
		return fmt.Errorf("password must be at most %d bytes", MaxLength)
	}
	return nil
}

// Generate returns a random temporary password (26 base32 characters, 130 bits).
func Generate() string { return rand.Text() }
