// Package secret provides symmetric (reversible) encryption for stored secrets
// that iris must later present to a third party — e.g. IMAP/POP3 mailbox
// passwords, which cannot be one-way hashed like login passwords.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Cipher encrypts/decrypts short secrets with AES-256-GCM. The key is derived
// from an operator-supplied passphrase (IRIS_MONITORING_KEY) via SHA-256, so any
// length passphrase yields a valid 32-byte key.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher builds a Cipher from the passphrase. An empty passphrase is an error
// (callers should treat that as "encryption unavailable").
func NewCipher(passphrase string) (*Cipher, error) {
	if passphrase == "" {
		return nil, errors.New("secret: empty passphrase")
	}
	sum := sha256.Sum256([]byte(passphrase))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("secret: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret: new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns base64(nonce || ciphertext) for the given plaintext. An empty
// plaintext encrypts to an empty string (so "no password set" round-trips).
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secret: nonce: %w", err)
	}
	ct := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt reverses Encrypt. An empty input decrypts to an empty string.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("secret: base64: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("secret: ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("secret: open: %w", err)
	}
	return string(pt), nil
}
