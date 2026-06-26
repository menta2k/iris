package biz

import (
	"errors"
	"net/mail"
	"strings"

	"github.com/emersion/go-msgauth/dkim"
)

// DKIMPublicKeyFunc returns the published DKIM TXT record value (e.g.
// "v=DKIM1; k=rsa; p=...") for one of our own domain+selector pairs, and false
// when we hold no key for it. Derived from our stored private keys, so a
// successful verification proves WE signed the message — no DNS dependency.
type DKIMPublicKeyFunc func(domain, selector string) (txtRecord string, ok bool)

// SentRecipientFunc returns the recipient we recorded for a given outbound
// Message-ID, or "" when we have no record of sending it.
type SentRecipientFunc func(messageID string) string

// Feedback verification methods, most-reliable first.
const (
	FeedbackVerifiedTrace   = "supplemental-trace" // our X-KumoRef marker matched
	FeedbackVerifiedSendLog = "send-log"           // original Message-ID is in our send log
	FeedbackVerifiedDKIM    = "dkim"               // embedded original is DKIM-signed by us
)

// VerifyFeedback reports whether an ARF complaint is provably about mail WE sent,
// so suppressing the complainant is safe. kumod already guarantees the report is
// structurally valid RFC 5965 (it only emits a Feedback record when its parser
// succeeds); this adds provenance. The checks run cheapest-first and short-circuit:
//
//  1. supplemental-trace: kumod's X-KumoRef recipient equals the complaint recipient.
//  2. send-log: the embedded original's Message-ID is one we recorded sending.
//  3. dkim: the embedded original carries a DKIM signature that verifies against
//     one of our own keys.
//
// method is "" (and verified false) when none hold — caller should record the
// report but NOT suppress.
func VerifyFeedback(rec *KumoLogRecord, ourKey DKIMPublicKeyFunc, sentRecipient SentRecipientFunc) (verified bool, method string) {
	complaint := rec.ComplainantRecipient()

	if tr := rec.TraceRecipient(); tr != "" && complaint != "" && strings.EqualFold(tr, complaint) {
		return true, FeedbackVerifiedTrace
	}

	original := rec.OriginalMessage()
	if original != "" && sentRecipient != nil {
		if mid := originalMessageID(original); mid != "" && sentRecipient(mid) != "" {
			return true, FeedbackVerifiedSendLog
		}
	}

	if original != "" && ourKey != nil && dkimSignedByUs(original, ourKey) {
		return true, FeedbackVerifiedDKIM
	}

	return false, ""
}

// errNotOurKey is returned by the verify LookupTXT when a signature's domain or
// selector is not one we hold a key for — so go-msgauth treats it as a failure.
var errNotOurKey = errors.New("dkim: signature is not from one of our keys")

// dkimSignedByUs verifies that the embedded original message carries at least one
// DKIM signature that validates against one of OUR keys. kumod normalizes the
// original's line endings to LF, so we restore CRLF before canonicalization.
func dkimSignedByUs(original string, ourKey DKIMPublicKeyFunc) bool {
	msg := strings.ReplaceAll(strings.ReplaceAll(original, "\r\n", "\n"), "\n", "\r\n")

	lookup := func(query string) ([]string, error) {
		selector, domain, ok := splitDKIMQuery(query)
		if !ok {
			return nil, errNotOurKey
		}
		txt, ok := ourKey(domain, selector)
		if !ok {
			return nil, errNotOurKey
		}
		return []string{txt}, nil
	}

	verifications, err := dkim.VerifyWithOptions(strings.NewReader(msg), &dkim.VerifyOptions{LookupTXT: lookup})
	if err != nil {
		return false
	}
	for _, v := range verifications {
		if v != nil && v.Err == nil {
			return true // a signature validated against one of our keys
		}
	}
	return false
}

// splitDKIMQuery parses a DKIM key query name "<selector>._domainkey.<domain>"
// into its selector and domain.
func splitDKIMQuery(query string) (selector, domain string, ok bool) {
	const marker = "._domainkey."
	i := strings.Index(strings.ToLower(query), marker)
	if i <= 0 {
		return "", "", false
	}
	selector = query[:i]
	domain = query[i+len(marker):]
	if selector == "" || domain == "" {
		return "", "", false
	}
	return selector, domain, true
}

// originalMessageID extracts the Message-ID header from an embedded original
// message (which may be headers-only), trimmed of angle brackets.
func originalMessageID(original string) string {
	body := strings.ReplaceAll(strings.ReplaceAll(original, "\r\n", "\n"), "\n", "\r\n")
	// net/mail needs headers terminated by a blank line; append one so a
	// headers-only embed (text/rfc822-headers) still parses.
	msg, err := mail.ReadMessage(strings.NewReader(body + "\r\n\r\n"))
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(msg.Header.Get("Message-ID")), "<>")
}
