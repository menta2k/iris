package biz

import (
	"strings"
	"testing"
)

func TestDKIMDomainValidate(t *testing.T) {
	assertReason(t, (&DKIMDomain{Domain: "example.com", Selector: "s1"}).Validate(), "")
	assertReason(t, (&DKIMDomain{Selector: "s1"}).Validate(), "DKIM_DOMAIN_REQUIRED")
	assertReason(t, (&DKIMDomain{Domain: "bad domain", Selector: "s1"}).Validate(), "DKIM_DOMAIN_INVALID")
	assertReason(t, (&DKIMDomain{Domain: "example.com"}).Validate(), "DKIM_SELECTOR_REQUIRED")
	assertReason(t, (&DKIMDomain{Domain: "example.com", Selector: "bad selector!"}).Validate(), "DKIM_SELECTOR_INVALID")
}

func TestDKIMPrivateKeyValidation(t *testing.T) {
	// Malformed PEM is rejected.
	assertReason(t, (&DKIMDomain{
		Domain: "example.com", Selector: "s1",
		PrivateKeyRef: "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----",
	}).Validate(), "DKIM_PRIVATE_KEY_INVALID")

	// A freshly generated RSA key is accepted, and Validate is a no-op on the
	// (write-only) key beyond parsing.
	pem, err := GenerateDKIMPrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	assertReason(t, (&DKIMDomain{Domain: "example.com", Selector: "s1", PrivateKeyRef: pem}).Validate(), "")
}

func TestGenerateDKIMKeyMaterial(t *testing.T) {
	pem, err := GenerateDKIMPrivateKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := ParseDKIMPrivateKey(pem); err != nil {
		t.Fatalf("generated key must parse: %v", err)
	}
	record, fingerprint, err := DKIMPublicRecord(pem)
	if err != nil {
		t.Fatalf("public record: %v", err)
	}
	if !strings.HasPrefix(record, "v=DKIM1; k=rsa; p=") {
		t.Fatalf("unexpected DKIM record: %q", record)
	}
	if !strings.HasPrefix(fingerprint, "sha256:") {
		t.Fatalf("unexpected fingerprint: %q", fingerprint)
	}
	if got := DKIMRecordName("s1", "example.com"); got != "s1._domainkey.example.com" {
		t.Fatalf("unexpected record name: %q", got)
	}
}

func TestDKIMDefaultStatus(t *testing.T) {
	d := &DKIMDomain{Domain: "example.com", Selector: "s1"}
	if err := d.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != DKIMNeedsAttention {
		t.Fatalf("expected default status needs_attention, got %q", d.Status)
	}
}
