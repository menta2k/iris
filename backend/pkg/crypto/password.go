// Package crypto contains password hashing and constant-time helpers.
//
// Passwords are hashed with bcrypt. The cost factor is configurable but
// pinned at a minimum of 12 to defend against offline brute-force attacks
// even if the password_hash column is exfiltrated.
package crypto

import (
	"crypto/subtle"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// MinBcryptCost is the floor we accept; values below this are rejected at
// startup so a misconfigured deployment cannot weaken stored hashes.
const MinBcryptCost = 12

var (
	ErrInvalidCost     = errors.New("crypto: bcrypt cost below minimum")
	ErrPasswordEmpty   = errors.New("crypto: password is empty")
	ErrPasswordTooLong = errors.New("crypto: password exceeds bcrypt 72-byte limit")
)

// HashPassword returns a bcrypt hash of the password using the supplied cost.
// It refuses cost < MinBcryptCost and refuses passwords longer than 72 bytes
// (bcrypt's hard limit) to prevent silent truncation.
func HashPassword(password string, cost int) (string, error) {
	if password == "" {
		return "", ErrPasswordEmpty
	}
	if len(password) > 72 {
		return "", ErrPasswordTooLong
	}
	if cost < MinBcryptCost {
		return "", fmt.Errorf("%w: got %d, want >= %d", ErrInvalidCost, cost, MinBcryptCost)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword returns nil iff the candidate matches the stored hash.
// The error type intentionally does not leak which side mismatched.
func VerifyPassword(hashed, candidate string) error {
	if hashed == "" || candidate == "" {
		return bcrypt.ErrMismatchedHashAndPassword
	}
	if len(candidate) > 72 {
		return bcrypt.ErrMismatchedHashAndPassword
	}
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(candidate))
}

// ConstantTimeEqualString compares two strings in constant time, padding the
// shorter side. Used for opaque tokens where length-leak resistance matters.
func ConstantTimeEqualString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
