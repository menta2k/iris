package biz

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is the minimum acceptable password length. bcrypt itself
// truncates inputs beyond 72 bytes, which we reject explicitly rather than
// silently ignore.
const (
	MinPasswordLength = 12
	maxPasswordBytes  = 72
)

// HashPassword returns a bcrypt digest of the password after validating its
// strength. The digest is safe to store and compare with CheckPassword.
func HashPassword(password string) (string, error) {
	if err := ValidatePasswordStrength(password); err != nil {
		return "", err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", Internal(err, "hash password")
	}
	return string(h), nil
}

// CheckPassword reports whether password matches the stored bcrypt hash. It
// returns false for an empty hash (accounts with no usable password) so login
// is disabled for them.
func CheckPassword(hash, password string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ValidatePasswordStrength enforces basic complexity rules. It is intentionally
// simple (length-based) — defense in depth, not a substitute for MFA.
func ValidatePasswordStrength(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return Invalid("PASSWORD_TOO_SHORT", "password must be at least %d characters", MinPasswordLength)
	}
	if len(password) > maxPasswordBytes {
		return Invalid("PASSWORD_TOO_LONG", "password must be at most %d bytes", maxPasswordBytes)
	}
	if strings.TrimSpace(password) == "" {
		return Invalid("PASSWORD_REQUIRED", "password is required")
	}
	return nil
}
