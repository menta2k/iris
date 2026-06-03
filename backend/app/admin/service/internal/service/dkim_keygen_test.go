package service

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestGenerateRSA1024(t *testing.T) {
	mat, err := DefaultKeyGenerator{}.Generate("rsa-1024")
	if err != nil {
		t.Fatalf("generate rsa-1024: %v", err)
	}
	if mat.Algorithm != "rsa-1024" {
		t.Fatalf("algorithm = %q, want rsa-1024", mat.Algorithm)
	}

	block, _ := pem.Decode([]byte(mat.PrivatePEM))
	if block == nil {
		t.Fatal("private PEM did not decode")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse pkcs8: %v", err)
	}
	rk, ok := key.(*rsa.PrivateKey)
	if !ok {
		t.Fatalf("key type = %T, want *rsa.PrivateKey", key)
	}
	if rk.N.BitLen() != 1024 {
		t.Fatalf("key size = %d bits, want 1024", rk.N.BitLen())
	}
}

func TestImportRSA1024RoundTrips(t *testing.T) {
	mat, err := DefaultKeyGenerator{}.Generate("rsa-1024")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// Declared algorithm matches.
	got, err := importPrivateKey(mat.PrivatePEM, "rsa-1024")
	if err != nil {
		t.Fatalf("import rsa-1024: %v", err)
	}
	if got.Algorithm != "rsa-1024" {
		t.Fatalf("imported algorithm = %q, want rsa-1024", got.Algorithm)
	}
	// Generic "rsa" declaration is accepted for any RSA size.
	if _, err := importPrivateKey(mat.PrivatePEM, "rsa"); err != nil {
		t.Fatalf("import with declared 'rsa' should accept rsa-1024: %v", err)
	}
}
