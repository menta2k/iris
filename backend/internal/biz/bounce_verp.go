package biz

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// VERP (Variable Envelope Return Path) encodes the original message id into the
// envelope sender so that asynchronous bounces (DSNs) routed back to the bounce
// domain can be correlated to the message — and thus the original recipient —
// that they belong to. Format (matches the previous Iris release):
//
//	b+<hmac16>.<msgid>@<bounce_domain>
//
// where hmac16 is the first 16 hex chars of HMAC-SHA256(secret, msgid). The
// HMAC is an anti-forgery signal; the message id is the correlation key.
const verpPrefixLen = 16

// DeriveVerpKey derives a stable VERP signing key from the deployment session
// secret, so the policy generator and the DSN worker agree on the same key
// without a separate secret store. Empty input yields an empty key (VERP off).
func DeriveVerpKey(sessionSecret string) string {
	if strings.TrimSpace(sessionSecret) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(sessionSecret))
	mac.Write([]byte("iris-bounce-verp-v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

// verpSig returns the 16-hex-char HMAC prefix, matching the policy's Lua
// (kumo.digest.hmac_sha256 → lowercase hex → first 16 chars).
func verpSig(secret, msgID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msgID))
	full := hex.EncodeToString(mac.Sum(nil))
	if len(full) > verpPrefixLen {
		return full[:verpPrefixLen]
	}
	return full
}

// EncodeBounceVERP builds the VERP envelope sender for a message id.
func EncodeBounceVERP(secret, msgID, bounceDomain string) string {
	return fmt.Sprintf("b+%s.%s@%s", verpSig(secret, msgID), msgID, bounceDomain)
}

// ParseBounceVERP extracts the message id from a VERP address. ok reports
// whether addr is in VERP form at all; signed reports whether the HMAC matched
// (callers may proceed on a valid id even when the signature is stale/forged,
// since the id itself is the unguessable correlation key, but should log it).
func ParseBounceVERP(secret, addr string) (msgID string, signed, ok bool) {
	local, _, found := strings.Cut(strings.ToLower(strings.TrimSpace(addr)), "@")
	if !found || !strings.HasPrefix(local, "b+") {
		return "", false, false
	}
	sig, mid, found := strings.Cut(local[2:], ".")
	if !found || mid == "" {
		return "", false, false
	}
	signed = hmac.Equal([]byte(sig), []byte(verpSig(secret, mid)))
	return mid, signed, true
}
