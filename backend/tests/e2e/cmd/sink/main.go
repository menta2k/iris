// Command sink is a tiny SMTP sink used by the e2e suite. It runs inside a
// container on the same Docker network as kumod, accepts mail kumod delivers,
// and records what it saw (EHLO, envelope, full DATA) so the test can assert
// how kumod routed the message. A small HTTP control API (reachable from the
// host) lets a test read captured messages and program per-recipient responses
// so bounce/DSN scenarios can force 4xx/5xx.
//
// It is stdlib-only on purpose: it must cross-compile to a static linux binary
// the harness mounts into a stock alpine image, and it must not perturb the
// backend module's dependencies.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

// captured is one message the sink received, with the routing-relevant envelope
// and the EHLO the connecting kumod egress source announced (which identifies
// the VMTA that won the route).
type captured struct {
	EHLO     string   `json:"ehlo"`
	MailFrom string   `json:"mailFrom"`
	Rcpts    []string `json:"rcpts"`
	Data     string   `json:"data"`
}

// rule programs a non-250 response for recipients containing Match, applied at
// the chosen SMTP stage ("rcpt" or "data").
type rule struct {
	Match string `json:"match"`
	Stage string `json:"stage"`
	Code  int    `json:"code"`
	Text  string `json:"text"`
}

type sink struct {
	mu    sync.Mutex
	msgs  []captured
	rules []rule
}

func main() {
	smtpAddr := envOr("SINK_SMTP_ADDR", "0.0.0.0:25")
	httpAddr := envOr("SINK_HTTP_ADDR", "0.0.0.0:8025")
	s := &sink{}

	ln, err := net.Listen("tcp", smtpAddr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sink: smtp listen:", err)
		os.Exit(1)
	}
	go s.serveHTTP(httpAddr)
	fmt.Println("sink: smtp on", smtpAddr, "http on", httpAddr)
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// responseFor returns a programmed (code, text, true) for a recipient at a given
// stage, or ok=false to fall through to the default 250.
func (s *sink) responseFor(rcpt, stage string) (int, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rules {
		st := r.Stage
		if st == "" {
			st = "rcpt"
		}
		if st == stage && r.Match != "" && strings.Contains(rcpt, r.Match) {
			return r.Code, r.Text, true
		}
	}
	return 0, "", false
}

func (s *sink) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	write := func(s string) { _, _ = w.WriteString(s); _ = w.Flush() }

	write("220 sink ready\r\n")
	var cur captured
	var lastRcpt string
	inData := false
	var data strings.Builder

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		if inData {
			if line == ".\r\n" || line == ".\n" {
				inData = false
				cur.Data = data.String()
				if code, text, ok := s.responseFor(lastRcpt, "data"); ok {
					// Capture the message even when we reject at data, so a
					// bounce scenario can still inspect what was sent.
					s.record(cur)
					cur = captured{}
					write(fmt.Sprintf("%d %s\r\n", code, text))
					continue
				}
				s.record(cur)
				cur = captured{}
				write("250 2.0.0 OK queued\r\n")
				continue
			}
			data.WriteString(line)
			continue
		}

		t := strings.TrimRight(line, "\r\n")
		u := strings.ToUpper(t)
		switch {
		case strings.HasPrefix(u, "EHLO"), strings.HasPrefix(u, "HELO"):
			cur.EHLO = strings.TrimSpace(t[4:])
			write("250-sink\r\n250 OK\r\n")
		case strings.HasPrefix(u, "MAIL FROM"):
			cur.MailFrom = extractAddr(t)
			write("250 2.1.0 OK\r\n")
		case strings.HasPrefix(u, "RCPT TO"):
			lastRcpt = extractAddr(t)
			cur.Rcpts = append(cur.Rcpts, lastRcpt)
			if code, text, ok := s.responseFor(lastRcpt, "rcpt"); ok {
				cur.Rcpts = cur.Rcpts[:len(cur.Rcpts)-1]
				write(fmt.Sprintf("%d %s\r\n", code, text))
				continue
			}
			write("250 2.1.5 OK\r\n")
		case strings.HasPrefix(u, "DATA"):
			data.Reset()
			inData = true
			write("354 send data\r\n")
		case strings.HasPrefix(u, "RSET"):
			cur = captured{}
			write("250 OK\r\n")
		case strings.HasPrefix(u, "QUIT"):
			write("221 bye\r\n")
			return
		default:
			write("250 OK\r\n")
		}
	}
}

func (s *sink) record(m captured) {
	s.mu.Lock()
	s.msgs = append(s.msgs, m)
	s.mu.Unlock()
}

// extractAddr pulls the address out of "MAIL FROM:<a@b>" / "RCPT TO:<a@b>".
func extractAddr(line string) string {
	if i := strings.IndexByte(line, '<'); i >= 0 {
		if j := strings.IndexByte(line[i:], '>'); j > 0 {
			return line[i+1 : i+j]
		}
	}
	if i := strings.IndexByte(line, ':'); i >= 0 {
		return strings.TrimSpace(line[i+1:])
	}
	return ""
}

func (s *sink) serveHTTP(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/messages", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		out := append([]captured(nil), s.msgs...)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/behavior", func(w http.ResponseWriter, req *http.Request) {
		var r rule
		if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.rules = append(s.rules, r)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/reset", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		s.msgs, s.rules = nil, nil
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	_ = http.ListenAndServe(addr, mux)
}
