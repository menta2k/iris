package biz

import "testing"

func TestBounceVERPRoundTrip(t *testing.T) {
	secret := DeriveVerpKey("session-secret")
	if secret == "" {
		t.Fatal("derived key should be non-empty")
	}
	addr := EncodeBounceVERP(secret, "abc-123-msgid", "bounces.example.com")
	if addr != "b+"+verpSig(secret, "abc-123-msgid")+".abc-123-msgid@bounces.example.com" {
		t.Fatalf("unexpected verp addr: %s", addr)
	}
	mid, signed, ok := ParseBounceVERP(secret, addr)
	if !ok || !signed || mid != "abc-123-msgid" {
		t.Fatalf("decode failed: mid=%q signed=%v ok=%v", mid, signed, ok)
	}
	// Wrong secret: still parses the id, but signature fails (caller may proceed).
	if mid2, signed2, ok2 := ParseBounceVERP(DeriveVerpKey("other"), addr); !ok2 || signed2 || mid2 != "abc-123-msgid" {
		t.Fatalf("wrong-secret decode: mid=%q signed=%v ok=%v", mid2, signed2, ok2)
	}
	// Non-VERP address.
	if _, _, ok3 := ParseBounceVERP(secret, "plain@example.com"); ok3 {
		t.Fatal("plain address must not parse as VERP")
	}
	// Empty session secret disables VERP.
	if DeriveVerpKey("") != "" {
		t.Fatal("empty secret should yield empty key")
	}
}
