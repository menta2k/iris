package verp

import (
	"strings"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	const secret = "test-secret-32-bytes-long-enough"
	cases := []string{
		"cd7b9a40e3",
		"13b02d19488111f1bf65529929482d5b",
		"a",
	}
	for _, msgID := range cases {
		t.Run(msgID, func(t *testing.T) {
			tok, err := Encode(secret, msgID)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			got, err := Decode(secret, tok)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if got != msgID {
				t.Fatalf("round trip: got %q want %q", got, msgID)
			}
		})
	}
}

func TestDecodeRejectsBadPrefix(t *testing.T) {
	const secret = "test-secret"
	tok, err := Encode(secret, "abc")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// Flip one hex char in the prefix.
	bad := "f" + tok[1:]
	_, err = Decode(secret, bad)
	if err != ErrPrefixMismatch {
		t.Fatalf("expected ErrPrefixMismatch, got %v", err)
	}
}

func TestDecodeRejectsWrongSecret(t *testing.T) {
	tok, err := Encode("secret-1", "msg")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, err = Decode("secret-2", tok)
	if err != ErrPrefixMismatch {
		t.Fatalf("expected ErrPrefixMismatch, got %v", err)
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "no-dot", "tooshort.x", strings.Repeat("a", 16) + "."} {
		_, err := Decode("secret", bad)
		if err == nil {
			t.Errorf("expected error for %q, got nil", bad)
		}
	}
}

func TestFromLocalPart(t *testing.T) {
	cases := []struct {
		in       string
		ok       bool
		expected string
	}{
		{"b+abc.123", true, "abc.123"},
		{"b-abc.123", true, "abc.123"},
		{"b", false, ""},
		{"x+abc", false, ""},
		{"", false, ""},
	}
	for _, c := range cases {
		got, ok := FromLocalPart(c.in)
		if ok != c.ok || got != c.expected {
			t.Errorf("FromLocalPart(%q) = (%q,%v) want (%q,%v)",
				c.in, got, ok, c.expected, c.ok)
		}
	}
}

func TestEncodeRejectsEmpty(t *testing.T) {
	if _, err := Encode("", "msg"); err != ErrEmptyInput {
		t.Errorf("expected ErrEmptyInput for empty secret, got %v", err)
	}
	if _, err := Encode("secret", ""); err != ErrEmptyInput {
		t.Errorf("expected ErrEmptyInput for empty msgid, got %v", err)
	}
}

// TestKeyedPrefixIsStable locks the on-the-wire format. If this test fails
// because someone changed the hash construction, the corresponding Lua
// emitter in pkg/kumopolicy/render.go MUST also be updated, or the
// renderer's policy-vs-Go round-trip test will fail.
func TestKeyedPrefixIsStable(t *testing.T) {
	got := keyedPrefix("supersecret", "cd7b9a40e3")
	if len(got) != PrefixHexLen {
		t.Fatalf("prefix length: got %d want %d", len(got), PrefixHexLen)
	}
	// Computed once and pinned. Update only when format intentionally changes.
	const pinned = "b231905617d71aea"
	if got != pinned {
		t.Errorf("keyedPrefix drifted: got %q want %q", got, pinned)
	}
}
