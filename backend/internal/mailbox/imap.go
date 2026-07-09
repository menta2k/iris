package mailbox

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/menta2k/iris/backend/internal/biz"
)

// fetchIMAP connects to the account's IMAP server and searches each configured
// folder for the probe. The first folder containing a match wins; its name is
// returned so the caller can classify placement (INBOX vs Spam/Junk).
func (f *Fetcher) fetchIMAP(ctx context.Context, acc *biz.MonitoringAccount, password, probeUID string) (biz.MailboxProbeResult, error) {
	var empty biz.MailboxProbeResult
	addr := net.JoinHostPort(acc.Host, strconv.Itoa(acc.Port))

	conn, err := dialConn(ctx, addr, acc.TLS, acc.Host, f.timeout)
	if err != nil {
		return empty, fmt.Errorf("imap dial %s: %w", addr, err)
	}
	// Bound every subsequent read/write by the context deadline.
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	client := imapclient.New(conn, nil)
	defer client.Close()

	if err := client.Login(acc.Username, password).Wait(); err != nil {
		return empty, fmt.Errorf("imap login: %w", err)
	}
	defer func() { _ = client.Logout().Wait() }()

	// Match either the correlation header or the uid in the Subject; providers
	// that strip X- headers still match on Subject.
	criteria := &imap.SearchCriteria{
		Or: [][2]imap.SearchCriteria{{
			{Header: []imap.SearchCriteriaHeaderField{{Key: biz.ProbeUIDHeader, Value: probeUID}}},
			{Header: []imap.SearchCriteriaHeaderField{{Key: "Subject", Value: probeUID}}},
		}},
	}

	for _, folder := range foldersFor(acc) {
		if _, err := client.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
			// Folder may not exist on this provider (e.g. "[Gmail]/Spam"); skip it.
			continue
		}
		data, err := client.UIDSearch(criteria, nil).Wait()
		if err != nil {
			continue
		}
		uids := data.AllUIDs()
		if len(uids) == 0 {
			continue
		}
		raw := fetchHeaders(client, uids[len(uids)-1])
		return biz.MailboxProbeResult{Found: true, Folder: folder, RawHeaders: raw}, nil
	}
	return biz.MailboxProbeResult{Found: false}, nil
}

// fetchHeaders fetches just the header block of a message by UID (best-effort;
// returns "" if the fetch fails — the probe is still recorded as found).
func fetchHeaders(client *imapclient.Client, uid imap.UID) string {
	opts := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{Specifier: imap.PartSpecifierHeader, Peek: true}},
	}
	msgs, err := client.Fetch(imap.UIDSetNum(uid), opts).Collect()
	if err != nil || len(msgs) == 0 {
		return ""
	}
	for _, bs := range msgs[0].BodySection {
		return string(bs.Bytes)
	}
	return ""
}

// dialConn opens a TCP (optionally TLS) connection. The context deadline is
// authoritative when present (set by the caller from the configured fetch
// timeout); otherwise the fallback timeout applies.
func dialConn(ctx context.Context, addr string, useTLS bool, serverName string, timeout time.Duration) (net.Conn, error) {
	d := &net.Dialer{}
	if dl, ok := ctx.Deadline(); ok {
		d.Deadline = dl
	} else {
		d.Timeout = timeout
	}
	if !useTLS {
		return d.DialContext(ctx, "tcp", addr)
	}
	return tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: serverName})
}
