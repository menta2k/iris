// Command inject is a tiny SMTP injector for the e2e suite. It runs as a
// throwaway sidecar container on the same Docker network as kumod and submits a
// message to kumod's ESMTP listener (the docker subnet is a trusted relay host
// in the generated policy), so a test can feed mail through the real reception
// path. It is stdlib-only so it cross-compiles to a static binary the harness
// mounts into a stock alpine image.
package main

import (
	"flag"
	"fmt"
	"net/smtp"
	"os"
	"strings"
)

type headerList []string

func (h *headerList) String() string { return strings.Join(*h, ",") }
func (h *headerList) Set(v string) error {
	*h = append(*h, v)
	return nil
}

func main() {
	var (
		addr    = flag.String("addr", "127.0.0.1:2525", "kumod ESMTP listener host:port")
		from    = flag.String("from", "sender@probe.test", "envelope sender")
		to      = flag.String("to", "", "envelope recipient (required)")
		subject = flag.String("subject", "e2e", "Subject header")
		body    = flag.String("body", "hello from e2e\n", "message body")
		helo    = flag.String("helo", "injector.probe.test", "EHLO name the injector announces")
		headers headerList
	)
	flag.Var(&headers, "header", "extra header 'Name: Value' (repeatable)")
	flag.Parse()

	if *to == "" {
		fmt.Fprintln(os.Stderr, "inject: -to is required")
		os.Exit(2)
	}

	c, err := smtp.Dial(*addr)
	if err != nil {
		fail("dial", err)
	}
	defer c.Close()
	if err := c.Hello(*helo); err != nil {
		fail("ehlo", err)
	}
	if err := c.Mail(*from); err != nil {
		fail("mail from", err)
	}
	if err := c.Rcpt(*to); err != nil {
		fail("rcpt to", err)
	}
	w, err := c.Data()
	if err != nil {
		fail("data", err)
	}

	var msg strings.Builder
	fmt.Fprintf(&msg, "From: %s\r\n", *from)
	fmt.Fprintf(&msg, "To: %s\r\n", *to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", *subject)
	for _, h := range headers {
		fmt.Fprintf(&msg, "%s\r\n", strings.TrimRight(h, "\r\n"))
	}
	msg.WriteString("\r\n")
	msg.WriteString(*body)

	if _, err := w.Write([]byte(msg.String())); err != nil {
		fail("write body", err)
	}
	if err := w.Close(); err != nil {
		fail("close data", err)
	}
	if err := c.Quit(); err != nil {
		fail("quit", err)
	}
	fmt.Println("inject: ok")
}

func fail(stage string, err error) {
	fmt.Fprintf(os.Stderr, "inject: %s: %v\n", stage, err)
	os.Exit(1)
}
