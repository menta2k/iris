package biz

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters (RFC 6238). SHA1 / 6 digits / 30s is the near-universal
// default that authenticator apps expect.
const (
	totpPeriod     = 30
	totpDigits     = 6
	totpSecretSize = 20 // 160-bit secret
)

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret returns a fresh base32-encoded TOTP secret.
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, totpSecretSize)
	if _, err := rand.Read(buf); err != nil {
		return "", Internal(err, "generate totp secret")
	}
	return base32NoPad.EncodeToString(buf), nil
}

// TOTPProvisioningURI builds the otpauth:// URI an authenticator app scans
// (rendered as a QR code by the client).
func TOTPProvisioningURI(secret, account, issuer string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", totpDigits))
	q.Set("period", fmt.Sprintf("%d", totpPeriod))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// VerifyTOTP checks a submitted code against the secret at time t, allowing a
// ±1 step skew to tolerate clock drift.
func VerifyTOTP(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	counter := uint64(t.Unix()) / totpPeriod
	for _, skew := range []int64{0, -1, 1} {
		if hmac.Equal([]byte(hotp(secret, uint64(int64(counter)+skew))), []byte(code)) {
			return true
		}
	}
	return false
}

// GenerateTOTP returns the valid TOTP code for the secret at time t. It is the
// counterpart to VerifyTOTP, used by tests and any first-party client that
// needs to produce a current code.
func GenerateTOTP(secret string, t time.Time) string {
	return hotp(secret, uint64(t.Unix())/totpPeriod)
}

// hotp computes the HOTP value (RFC 4226) for the secret and counter.
func hotp(secret string, counter uint64) string {
	key, err := base32NoPad.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return ""
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	val := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])
	return fmt.Sprintf("%0*d", totpDigits, val%1000000)
}
