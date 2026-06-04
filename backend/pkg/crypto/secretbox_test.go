package crypto

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	plaintext := "JBSWY3DPEHPK3PXP"

	enc, err := EncryptSecret(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plaintext || strings.Contains(enc, plaintext) {
		t.Fatal("ciphertext leaks plaintext")
	}
	// Same plaintext encrypts differently each time (random nonce).
	enc2, _ := EncryptSecret(key, plaintext)
	if enc == enc2 {
		t.Fatal("nonce reuse: identical ciphertext for repeated encryption")
	}

	got, err := DecryptSecret(key, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Fatalf("round-trip = %q, want %q", got, plaintext)
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	enc, err := EncryptSecret([]byte("0123456789abcdef0123456789abcdef"), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptSecret([]byte("ffffffffffffffffffffffffffffffff"), enc); err == nil {
		t.Fatal("decrypt with wrong key must fail")
	}
}

func TestBadKeySize(t *testing.T) {
	if _, err := EncryptSecret([]byte("short"), "x"); err != ErrCryptoKeySize {
		t.Fatalf("expected ErrCryptoKeySize, got %v", err)
	}
}

func TestDecryptMalformed(t *testing.T) {
	key := []byte("0123456789abcdef")
	if _, err := DecryptSecret(key, "!!!not-base64!!!"); err != ErrCryptoMalformed {
		t.Fatalf("expected ErrCryptoMalformed, got %v", err)
	}
	if _, err := DecryptSecret(key, "QUJD"); err != ErrCryptoMalformed {
		t.Fatalf("short ciphertext: expected ErrCryptoMalformed, got %v", err)
	}
}
