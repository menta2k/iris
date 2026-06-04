package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// AES-GCM symmetric encryption for secrets stored at rest (e.g. the MFA TOTP
// secret). The key must be 16, 24, or 32 bytes (AES-128/192/256). Ciphertext
// is returned base64(std)-encoded as nonce||ciphertext||tag so it round-trips
// through a text DB column.

var (
	ErrCryptoKeySize   = errors.New("crypto: key must be 16, 24, or 32 bytes")
	ErrCryptoMalformed = errors.New("crypto: malformed ciphertext")
)

// EncryptSecret seals plaintext with AES-GCM under key and returns a base64
// string. A fresh random nonce is prepended so the same plaintext encrypts
// differently each time.
func EncryptSecret(key []byte, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptSecret reverses EncryptSecret.
func DecryptSecret(key []byte, encoded string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrCryptoMalformed
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", ErrCryptoMalformed
	}
	nonce, ct := raw[:ns], raw[ns:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", ErrCryptoMalformed
	}
	return string(plaintext), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	switch len(key) {
	case 16, 24, 32:
	default:
		return nil, ErrCryptoKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
