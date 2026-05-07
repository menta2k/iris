package dsnstream

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // register charset decoders for go-message

	"github.com/menta2k/iris/backend/pkg/verp"
)

// Parse decodes one raw DSN body and returns a Parsed. The envelopeRcpt
// argument is the address the kumomta catcher wrote on the XADD — i.e.
// the value the bouncing MTA sent the DSN to.
//
// secret is the VERP HMAC key. When non-empty and the envelope-rcpt's
// local-part validates, Parsed.VerpToken + Parsed.MessageID are
// populated from the token and the consumer can skip the embedded
// headers lookup.
func Parse(envelopeRcpt, secret string, raw []byte) (*Parsed, error) {
	if len(raw) == 0 {
		return nil, errors.New("dsnstream: empty body")
	}
	out := &Parsed{
		EnvelopeRecipient: envelopeRcpt,
		RawSize:           len(raw),
		EmbeddedHeaders:   map[string]string{},
	}

	// VERP recovery happens before MIME parsing because the local-part
	// of the envelope recipient is sufficient to correlate even if the
	// body is malformed (some bouncing MTAs send free-form text).
	if envelopeRcpt != "" && secret != "" {
		if local, _, ok := splitAddress(envelopeRcpt); ok {
			if tok, ok := verp.FromLocalPart(local); ok {
				if msgID, err := verp.Decode(secret, tok); err == nil {
					out.VerpToken = tok
					out.MessageID = msgID
				}
			}
		}
	}

	// Outer message: don't fail the whole parse on a malformed outer
	// header — some bounces from very old MTAs are barely-valid mail.
	// We try the multipart path first and fall through to a "the body
	// is the diagnostic" path if it isn't structured.
	entity, err := message.Read(bytesReader(raw))
	if err != nil {
		// Not RFC 5322 conformant; nothing more we can extract.
		return out, nil
	}
	mr := entity.MultipartReader()
	if mr == nil {
		// Single-part bounce; some MTAs (or aliases on the way back)
		// reduce a DSN to a plain text/plain body. Fall through with
		// only the VERP-derived correlation populated.
		return out, nil
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// One bad part shouldn't kill the rest. Most often: a
			// content-encoded message/rfc822 sub-part with an unsupported
			// transfer encoding. We continue to the next part.
			continue
		}
		ctype, _, err := part.Header.ContentType()
		if err != nil {
			continue
		}
		switch ctype {
		case "message/delivery-status":
			fillFromDeliveryStatus(out, part.Body)
		case "message/rfc822", "text/rfc822-headers":
			fillFromEmbeddedOriginal(out, part.Body)
		}
	}
	return out, nil
}

// fillFromDeliveryStatus parses the per-message and per-recipient field
// blocks per RFC 3464 §2.1. Field blocks are header-style key/value pairs
// separated by blank lines. We grab the first per-recipient block — some
// MTAs send one block per recipient but in practice the first matches the
// envelope of the bounce.
func fillFromDeliveryStatus(out *Parsed, body io.Reader) {
	var blocks [][]headerKV
	current := []headerKV{}
	br := bufio.NewReader(body)
	for {
		line, err := br.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = []headerKV{}
			}
			if err == io.EOF {
				break
			}
			continue
		}
		// Continuation lines (RFC 5322 folded headers) start with whitespace.
		if (line[0] == ' ' || line[0] == '\t') && len(current) > 0 {
			current[len(current)-1].value += " " + strings.TrimSpace(trimmed)
		} else if k, v, ok := splitField(trimmed); ok {
			current = append(current, headerKV{key: k, value: v})
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	if len(blocks) == 0 {
		return
	}
	// Per-message block (block[0]) carries Reporting-MTA. Per-recipient
	// blocks (block[1..]) carry Final-Recipient/Action/Status.
	for _, kv := range blocks[0] {
		switch strings.ToLower(kv.key) {
		case "reporting-mta", "x-postfix-reporting-mta":
			out.RemoteMTA = stripMTAType(kv.value)
		}
	}
	for _, block := range blocks[1:] {
		for _, kv := range block {
			switch strings.ToLower(kv.key) {
			case "final-recipient":
				out.FinalRecipient = stripAddrType(kv.value)
			case "original-recipient":
				out.OriginalRecipient = stripAddrType(kv.value)
			case "action":
				out.Action = strings.ToLower(strings.TrimSpace(kv.value))
			case "status":
				out.Status = strings.TrimSpace(kv.value)
			case "diagnostic-code":
				out.DiagnosticCode = clip(strings.TrimSpace(kv.value), 1024)
			case "remote-mta":
				if out.RemoteMTA == "" {
					out.RemoteMTA = stripMTAType(kv.value)
				}
			}
		}
		// First block with at least one populated field wins.
		if out.FinalRecipient != "" || out.Action != "" || out.Status != "" {
			break
		}
	}
}

// fillFromEmbeddedOriginal reads the embedded original (full message or
// just headers) and pulls the values the consumer cares about: the
// Message-ID for fallback correlation, plus the X-Kumo-* tags for mail
// class / tenant / campaign denormalisation.
//
// All header keys are stored lower-cased so the Parsed accessor methods
// don't have to case-fold on every read.
func fillFromEmbeddedOriginal(out *Parsed, body io.Reader) {
	// net/mail.ReadMessage handles unfolding and CRLF/LF line endings,
	// which is friendlier than parsing by hand. It tolerates a missing
	// body (text/rfc822-headers parts).
	msg, err := mail.ReadMessage(body)
	if err != nil {
		return
	}
	for k, vs := range msg.Header {
		if len(vs) == 0 {
			continue
		}
		out.EmbeddedHeaders[strings.ToLower(k)] = strings.TrimSpace(vs[0])
	}
	if out.MessageID == "" {
		if mid := strings.TrimSpace(msg.Header.Get("Message-ID")); mid != "" {
			// Strip RFC 5322 angle brackets if present.
			mid = strings.TrimPrefix(strings.TrimSuffix(mid, ">"), "<")
			out.MessageID = mid
		}
	}
}

type headerKV struct{ key, value string }

func splitField(line string) (string, string, bool) {
	colon := strings.IndexByte(line, ':')
	if colon <= 0 {
		return "", "", false
	}
	return line[:colon], strings.TrimSpace(line[colon+1:]), true
}

// stripAddrType removes the RFC 3464 address-type prefix ("rfc822;").
// Both Original-Recipient and Final-Recipient carry this prefix.
func stripAddrType(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ';'); i >= 0 {
		return strings.TrimSpace(v[i+1:])
	}
	return v
}

// stripMTAType removes the "dns;" / "x500;" prefix from MTA fields.
func stripMTAType(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ';'); i >= 0 {
		return strings.TrimSpace(v[i+1:])
	}
	return v
}

// splitAddress returns local-part, domain, ok. It is RFC 5321 simple
// (no quoted-string parsing) — enough for the catcher's
// envelope-recipient because we control the format.
func splitAddress(addr string) (string, string, bool) {
	addr = strings.TrimSpace(addr)
	at := strings.LastIndexByte(addr, '@')
	if at <= 0 || at == len(addr)-1 {
		return "", "", false
	}
	return addr[:at], addr[at+1:], true
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// bytesReader is a tiny adapter so callers don't have to wrap raw []byte
// in bytes.NewReader at the call site.
func bytesReader(b []byte) io.Reader { return strings.NewReader(string(b)) }

// Used internally by fmt errors that need a context-rich wrap; pulled out
// of the hot path to keep the parser readable.
var _ = fmt.Sprint
