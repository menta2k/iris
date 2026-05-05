package crypto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerifyPassword(t *testing.T) {
	const pw = "correct horse battery staple!"
	hash, err := HashPassword(pw, MinBcryptCost)
	require.NoError(t, err)
	require.NotEqual(t, pw, hash)

	require.NoError(t, VerifyPassword(hash, pw))
	require.Error(t, VerifyPassword(hash, "wrong"))
}

func TestHashPasswordRejectsLowCost(t *testing.T) {
	_, err := HashPassword("xxxxxxxxxxxx", MinBcryptCost-1)
	require.ErrorIs(t, err, ErrInvalidCost)
}

func TestHashPasswordRejectsEmpty(t *testing.T) {
	_, err := HashPassword("", MinBcryptCost)
	require.ErrorIs(t, err, ErrPasswordEmpty)
}

func TestHashPasswordRejectsOver72Bytes(t *testing.T) {
	_, err := HashPassword(strings.Repeat("x", 73), MinBcryptCost)
	require.ErrorIs(t, err, ErrPasswordTooLong)
}

func TestVerifyPasswordRejectsOversized(t *testing.T) {
	hash, err := HashPassword("normal-password!", MinBcryptCost)
	require.NoError(t, err)
	err = VerifyPassword(hash, strings.Repeat("y", 73))
	require.ErrorIs(t, err, bcrypt.ErrMismatchedHashAndPassword)
}

func TestConstantTimeEqualString(t *testing.T) {
	require.True(t, ConstantTimeEqualString("abc", "abc"))
	require.False(t, ConstantTimeEqualString("abc", "abd"))
	require.False(t, ConstantTimeEqualString("abc", "abcd"))
	require.False(t, ConstantTimeEqualString("", "x"))
}
