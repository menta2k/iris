package mailbox

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/knadh/go-pop3"

	"github.com/menta2k/iris/backend/internal/biz"
)

// pop3ScanLimit bounds how many of the most recent messages a POP3 fetch scans.
// POP3 has no server-side search, so iris retrieves headers newest-first and
// stops at this many messages to keep a large mailbox from stalling a probe.
const pop3ScanLimit = 50

// fetchPOP3 connects to the account's POP3 server and scans the most recent
// messages for the probe. POP3 exposes no folders, so a match yields an empty
// Folder (placement is treated as inbox — the message is in the primary mailbox).
func (f *Fetcher) fetchPOP3(ctx context.Context, acc *biz.MonitoringAccount, password, probeUID string) (biz.MailboxProbeResult, error) {
	var empty biz.MailboxProbeResult

	client := pop3.New(pop3.Opt{
		Host:        acc.Host,
		Port:        acc.Port,
		TLSEnabled:  acc.TLS,
		DialTimeout: f.timeout,
	})
	conn, err := client.NewConn()
	if err != nil {
		return empty, fmt.Errorf("pop3 dial %s:%d: %w", acc.Host, acc.Port, err)
	}
	defer func() { _ = conn.Quit() }()

	if err := conn.Auth(acc.Username, password); err != nil {
		return empty, fmt.Errorf("pop3 auth: %w", err)
	}

	count, _, err := conn.Stat()
	if err != nil {
		return empty, fmt.Errorf("pop3 stat: %w", err)
	}

	// Scan newest-first (higher ids are more recent), bounded by pop3ScanLimit.
	lowest := max(count-pop3ScanLimit+1, 1)
	for id := count; id >= lowest; id-- {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		raw, err := conn.RetrRaw(id)
		if err != nil {
			continue
		}
		headers := headerBlock(raw.Bytes())
		if strings.Contains(headers, probeUID) {
			return biz.MailboxProbeResult{Found: true, Folder: "", RawHeaders: headers}, nil
		}
	}
	return biz.MailboxProbeResult{Found: false}, nil
}

// headerBlock returns the RFC 5322 header section of a raw message (everything
// up to the first blank line), tolerating both CRLF and LF separators.
func headerBlock(raw []byte) string {
	if head, _, ok := bytes.Cut(raw, []byte("\r\n\r\n")); ok {
		return string(head)
	}
	if head, _, ok := bytes.Cut(raw, []byte("\n\n")); ok {
		return string(head)
	}
	return string(raw)
}
