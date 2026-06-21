//go:build e2e

package e2e

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/menta2k/iris/backend/internal/biz"
)

// TestDKIMSigning proves the generated DKIM signing path works in a real kumod:
// a message From a configured ready DKIM domain leaves with a DKIM-Signature
// that cryptographically verifies against the matching public key.
//
// kumod reads the RSA private key from the path the generated policy references
// (DKIM_BY_DOMAIN[domain].key); the harness drops a freshly-generated key there.
// Verification supplies the public key via a custom TXT lookup, so no DNS is
// needed.
func TestDKIMSigning(t *testing.T) {
	requireE2E(t)
	requireDocker(t)

	const (
		domain   = "signed.test"
		selector = "e2e"
	)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	dkimTXT := "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(pubDER)

	// The private key is now stored inline (PEM), so it is rendered into the
	// policy as KeySource key_data — no key file to mount.
	snap := routingSnapshot(kumodIP)
	snap.DKIM = []*biz.DKIMDomain{
		{ID: "d1", Domain: domain, Selector: selector, PrivateKeyRef: string(keyPEM), Status: biz.DKIMReady},
	}
	r := startRig(t, snap)

	r.injectAs("newsletter@"+domain, "user@sink.test", "X-Mail-Class: bulk")
	msgs := r.waitForSink(1, 30*time.Second)
	msg := msgs[0]

	// The signature must be present and name our domain + selector.
	if !strings.Contains(msg.Data, "DKIM-Signature:") {
		t.Fatalf("no DKIM-Signature header in delivered message:\n%s\n--- kumod logs ---\n%s",
			headerBlock(msg.Data), lastLines(r.kumodLogs(), 25))
	}
	if !strings.Contains(msg.Data, "d="+domain) || !strings.Contains(msg.Data, "s="+selector) {
		t.Fatalf("DKIM-Signature missing d=%s / s=%s:\n%s", domain, selector, headerBlock(msg.Data))
	}

	// Cryptographically verify against the public key (provided directly, so no
	// DNS lookup is required).
	verifications, err := dkim.VerifyWithOptions(strings.NewReader(normalizeCRLF(msg.Data)), &dkim.VerifyOptions{
		LookupTXT: func(query string) ([]string, error) {
			want := selector + "._domainkey." + domain
			if strings.TrimSuffix(query, ".") != want {
				return nil, fmt.Errorf("unexpected dkim query %q", query)
			}
			return []string{dkimTXT}, nil
		},
	})
	if err != nil {
		t.Fatalf("dkim verify: %v", err)
	}
	if len(verifications) == 0 {
		t.Fatal("no DKIM verifications produced")
	}
	for _, v := range verifications {
		if v.Err != nil {
			t.Errorf("DKIM verification for d=%s failed: %v", v.Domain, v.Err)
		}
		if v.Domain != domain {
			t.Errorf("verified domain = %q, want %q", v.Domain, domain)
		}
	}
}

// headerBlock returns just the header section (up to the blank line) for tidy
// failure output.
func headerBlock(data string) string {
	if i := strings.Index(data, "\r\n\r\n"); i >= 0 {
		return data[:i]
	}
	return data
}

// normalizeCRLF ensures the message uses CRLF line endings, which DKIM
// verification requires. The sink records data verbatim from the SMTP DATA
// stream (already CRLF), but guard against any lone LFs.
func normalizeCRLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}
