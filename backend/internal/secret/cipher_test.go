package secret

import "testing"

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher("a-test-passphrase")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	for _, pt := range []string{"", "hunter2", "app-pw with spaces & symbols: €"} {
		enc, err := c.Encrypt(pt)
		if err != nil {
			t.Fatalf("encrypt %q: %v", pt, err)
		}
		if pt != "" && enc == pt {
			t.Errorf("ciphertext equals plaintext for %q", pt)
		}
		dec, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("decrypt %q: %v", pt, err)
		}
		if dec != pt {
			t.Errorf("round-trip = %q, want %q", dec, pt)
		}
	}
}

func TestCipherWrongKeyFails(t *testing.T) {
	a, _ := NewCipher("key-a")
	b, _ := NewCipher("key-b")
	enc, _ := a.Encrypt("secret")
	if _, err := b.Decrypt(enc); err == nil {
		t.Error("decrypt with wrong key should fail")
	}
}

func TestCipherEmptyPassphrase(t *testing.T) {
	if _, err := NewCipher(""); err == nil {
		t.Error("empty passphrase should error")
	}
}
