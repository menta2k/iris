package data

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SMTPNotifier delivers alert emails over plain SMTP. It targets a configurable
// host:port (default the local KumoMTA loopback listener) with a dial+session
// timeout so a slow or dead relay can never block the monitor worker. No auth /
// STARTTLS — intended for a trusted local or on-network smarthost.
type SMTPNotifier struct {
	timeout time.Duration
}

// NewSMTPNotifier constructs the notifier.
func NewSMTPNotifier() *SMTPNotifier { return &SMTPNotifier{timeout: 15 * time.Second} }

// Notify sends a plain-text email to the recipients via host (host:port).
func (n *SMTPNotifier) Notify(ctx context.Context, host, from string, to []string, subject, body string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost:25"
	}
	if len(to) == 0 {
		return fmt.Errorf("no recipients")
	}
	deadline := time.Now().Add(n.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	conn, err := net.DialTimeout("tcp", host, n.timeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", host, err)
	}
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, hostOnly(host))
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer c.Close()

	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT %s: %w", rcpt, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(buildMessage(from, to, subject, body)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return c.Quit()
}

// buildMessage renders a minimal RFC 5322 plain-text message.
func buildMessage(from string, to []string, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	b.WriteString("\r\n")
	return []byte(b.String())
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}
