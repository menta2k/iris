// Package verp encodes and verifies VERP (Variable Envelope Return Path)
// tokens used to correlate inbound DSNs back to the original send.
//
// The token format is intentionally short and url-safe so it fits inside
// the RFC 5321 64-octet local-part limit comfortably:
//
//	<hex16> "." <message_id>
//
// where hex16 is the lowercase hex of the first 8 bytes of
// HMAC-SHA256(secret, message_id). That's 16 + 1 + N hex chars, fitting
// even kumomta's 32-char hex message IDs (49 chars total) into the limit.
//
// The matching Lua emitter is in pkg/kumopolicy/render.go and MUST keep
// the format in lockstep. There's a round-trip test in this package and a
// renderer test that asserts the Lua produces the same hex prefix as Go
// for a fixed (secret, msgid) pair.
package verp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// LocalPartPrefix is the literal prepended to every VERP local-part. The
// inbound catcher routes by matching this prefix so non-VERP mail hitting
// the bounce domain (e.g. operator misuse) is dropped instead of getting
// fed into the consumer.
const LocalPartPrefix = "b"

// MaxTokenLen guards against a pathological message_id blowing past the
// SMTP local-part limit. Empirically kumomta IDs are 32 hex chars
// (16 bytes); 64 leaves headroom for any future format change.
const MaxTokenLen = 64

// PrefixHexLen is the length in characters of the keyed-hash prefix.
// 16 hex chars = 8 binary bytes = 64 bits, which gives ~2^32 forgery
// resistance — adequate when paired with TTL filtering and silent-drop
// on validation failure. Don't change this without bumping the format
// version in the local-part.
const PrefixHexLen = 16

var (
	// ErrInvalidFormat is returned by Decode when the token doesn't have
	// the expected prefix-dot-msgid shape.
	ErrInvalidFormat = errors.New("verp: token has invalid format")
	// ErrPrefixMismatch is returned by Decode when the keyed-hash prefix
	// doesn't match what the secret + msgid would produce. Treat as a
	// signal to drop the DSN silently — almost always backscatter.
	ErrPrefixMismatch = errors.New("verp: token prefix does not validate")
	// ErrEmptyInput is returned by Encode when secret or msgid is empty.
	ErrEmptyInput = errors.New("verp: secret and message_id required")
)

// Encode produces the local-part for a VERP envelope sender. The full
// MAIL FROM is then "<LocalPartPrefix>+<Encode-result>@<bounce-domain>".
//
// Example:
//
//	tok, _ := verp.Encode("supersekret", "cd7b9a40e3")
//	// tok = "<16hex>.cd7b9a40e3"
//	mail_from := "b+" + tok + "@bounces.example.com"
func Encode(secret, msgID string) (string, error) {
	if secret == "" || msgID == "" {
		return "", ErrEmptyInput
	}
	pre := keyedPrefix(secret, msgID)
	tok := pre + "." + msgID
	if len(tok) > MaxTokenLen {
		return "", fmt.Errorf("verp: token length %d exceeds max %d", len(tok), MaxTokenLen)
	}
	return tok, nil
}

// Decode parses and validates a VERP token. On success it returns the
// recovered message_id. On format / prefix errors it returns a typed
// error so the caller can decide between "drop silently as backscatter"
// (prefix mismatch) and "log as a bug" (truly malformed input).
func Decode(secret, token string) (string, error) {
	if secret == "" || token == "" {
		return "", ErrEmptyInput
	}
	dot := strings.IndexByte(token, '.')
	if dot != PrefixHexLen {
		return "", ErrInvalidFormat
	}
	prefix, msgID := token[:dot], token[dot+1:]
	if msgID == "" {
		return "", ErrInvalidFormat
	}
	expected := keyedPrefix(secret, msgID)
	if subtle.ConstantTimeCompare([]byte(prefix), []byte(expected)) != 1 {
		return "", ErrPrefixMismatch
	}
	return msgID, nil
}

// FromLocalPart strips the "b+" wrapping that Encode-callers add when
// building the MAIL FROM. Convenience for the inbound catcher that
// receives the raw envelope-recipient local-part.
//
// Returns ("", false) when the input doesn't carry the expected prefix.
// Both `b+xxx` and `b-xxx` shapes are accepted because some receivers
// strip `+` from MAIL FROM addresses on the way back.
func FromLocalPart(lp string) (string, bool) {
	const want = LocalPartPrefix
	if !strings.HasPrefix(lp, want) {
		return "", false
	}
	rest := lp[len(want):]
	if rest == "" {
		return "", false
	}
	switch rest[0] {
	case '+', '-':
		return rest[1:], true
	}
	return "", false
}

// keyedPrefix returns the lowercase-hex of the first 8 bytes of
// HMAC-SHA256(secret, msgid). Centralised so encoder + decoder + the
// renderer's Lua emitter all reference one definition.
func keyedPrefix(secret, msgID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msgID))
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum[:8])
}
