package biz

import "testing"

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("a-strong-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" || hash == "a-strong-password" {
		t.Fatal("hash must be non-empty and not the plaintext")
	}
	if !CheckPassword(hash, "a-strong-password") {
		t.Fatal("correct password should verify")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Fatal("wrong password must not verify")
	}
	if CheckPassword("", "anything") {
		t.Fatal("empty hash must never verify (login disabled)")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	if err := ValidatePasswordStrength("short"); err == nil {
		t.Fatal("expected too-short password rejected")
	}
	if err := ValidatePasswordStrength("this-is-long-enough"); err != nil {
		t.Fatalf("expected acceptable password, got %v", err)
	}
	// bcrypt's 72-byte input limit is enforced explicitly.
	long := make([]byte, 73)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidatePasswordStrength(string(long)); err == nil {
		t.Fatal("expected over-length password rejected")
	}
}
