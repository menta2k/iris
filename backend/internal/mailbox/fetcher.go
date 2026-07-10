// Package mailbox implements the phase-2 mailbox fetch for inbox-placement
// monitoring: it connects to a monitored IMAP/POP3 account and locates a probe
// message by its unique id (the X-Iris-Probe-Id header or the uid embedded in
// the subject), returning the folder it was found in and its raw headers.
package mailbox

import (
	"context"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// DefaultTimeout bounds a single mailbox fetch (dial + login + search + header
// fetch) when the caller's context carries no deadline.
const DefaultTimeout = 30 * time.Second

// Fetcher dispatches to the IMAP or POP3 implementation based on the account's
// protocol. It satisfies biz.MailboxFetcher.
type Fetcher struct {
	timeout time.Duration
}

var _ biz.MailboxFetcher = (*Fetcher)(nil)

// NewFetcher constructs a Fetcher. A non-positive timeout uses DefaultTimeout.
func NewFetcher(timeout time.Duration) *Fetcher {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Fetcher{timeout: timeout}
}

// Fetch searches the account's mailbox for the probe identified by probeUID.
func (f *Fetcher) Fetch(ctx context.Context, acc *biz.MonitoringAccount, password, probeUID string) (biz.MailboxProbeResult, error) {
	ctx, cancel := f.withTimeout(ctx)
	defer cancel()
	switch acc.Protocol {
	case biz.MonitorProtocolPOP3:
		return f.fetchPOP3(ctx, acc, password, probeUID)
	default:
		return f.fetchIMAP(ctx, acc, password, probeUID)
	}
}

// Verify connects and authenticates to the mailbox (without searching) to check
// the account's connection parameters and credentials. Returns nil on success.
func (f *Fetcher) Verify(ctx context.Context, acc *biz.MonitoringAccount, password string) error {
	ctx, cancel := f.withTimeout(ctx)
	defer cancel()
	switch acc.Protocol {
	case biz.MonitorProtocolPOP3:
		return f.verifyPOP3(ctx, acc, password)
	default:
		return f.verifyIMAP(ctx, acc, password)
	}
}

// withTimeout ensures the context has a deadline so a hung server cannot stall a
// fetch forever.
func (f *Fetcher) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, f.timeout)
}

// foldersFor returns the folders to search, defaulting to INBOX.
func foldersFor(acc *biz.MonitoringAccount) []string {
	if len(acc.CheckFolders) == 0 {
		return []string{"INBOX"}
	}
	return acc.CheckFolders
}
