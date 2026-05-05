// runner.go — drives one TrafficBlock against the kumomta SMTP listener.
//
// Each scenario runs in its own goroutine. A worker pool sized at
// max(rate*2, 8) bounds concurrent SMTP connections so we don't overload
// kumomta with file descriptors while still saturating it at requested
// throughput.
package main

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RunResult is what a scenario reports back so main() can surface a
// per-scenario summary even if the assertions later fail.
type RunResult struct {
	Name      string
	Sent      int64
	Failed    int64
	StartedAt time.Time
	FinishedAt time.Time
}

// runScenario blocks until the scenario's submissions are all dispatched
// (synchronous SMTP — the goroutine returns once the destination has acked).
// kumomta accepts on receipt; downstream delivery happens out of band.
func runScenario(ctx context.Context, smtpAddr string, t TrafficBlock) RunResult {
	// Initialise FinishedAt to StartedAt so the defaulted duration is 0
	// rather than a wrap-around negative when the caller logs before the
	// deferred update fires.
	now := time.Now()
	res := RunResult{Name: t.Name, StartedAt: now, FinishedAt: now}
	defer func() { res.FinishedAt = time.Now() }()

	bodyBytes := t.BodyBytes
	if bodyBytes <= 0 {
		bodyBytes = 256
	}
	body := buildBody(t, bodyBytes)

	// Worker pool: enough headroom that we can saturate kumomta at the
	// configured rate without serialising on a single goroutine.
	workers := t.RatePerSec * 2
	if workers < 8 {
		workers = 8
	}
	if workers > 256 {
		workers = 256
	}
	jobs := make(chan int, workers*2)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				if err := sendOne(smtpAddr, t, body); err != nil {
					atomic.AddInt64(&res.Failed, 1)
					log.Printf("loadgen[%s]: send failed: %v", t.Name, err)
					continue
				}
				atomic.AddInt64(&res.Sent, 1)
			}
		}()
	}

	if t.Total > 0 {
		// Burst mode: just fire `total` and let the workers chew through.
		for i := 0; i < t.Total; i++ {
			select {
			case jobs <- i:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return res
			}
		}
	} else {
		// Rate-limited mode: a ticker paces submissions; workers do the I/O.
		interval := time.Second / time.Duration(t.RatePerSec)
		end := time.Now().Add(time.Duration(t.DurationSec) * time.Second)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		i := 0
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case <-ticker.C:
				if time.Now().After(end) {
					break loop
				}
				select {
				case jobs <- i:
					i++
				default:
					// Workers are saturated — drop the tick rather than queueing
					// unbounded, so we get a realistic "kumomta back-pressure"
					// signal instead of memory growth on the loadgen side.
					atomic.AddInt64(&res.Failed, 1)
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	res.FinishedAt = time.Now() // explicit set; the deferred update can't
	return res                  // mutate a non-named return value.
}

func sendOne(smtpAddr string, t TrafficBlock, body []byte) error {
	c, err := smtp.Dial(smtpAddr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer c.Close()

	if err := c.Hello("loadgen.test"); err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	if err := c.Mail(t.From); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := c.Rcpt(t.To); err != nil {
		return fmt.Errorf("rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("data close: %w", err)
	}
	return c.Quit()
}

// buildBody assembles a minimal RFC822 message. Headers from the scenario
// take precedence over our defaults so a scenario can override From/To
// (envelope vs header) for tests that exercise that mismatch.
func buildBody(t TrafficBlock, padBytes int) []byte {
	var b strings.Builder
	headers := map[string]string{
		"From":      t.From,
		"To":        t.To,
		"Subject":   "loadgen " + t.Name,
		"Date":      time.Now().UTC().Format(time.RFC1123Z),
		"Message-ID": fmt.Sprintf("<loadgen-%s-%d@kumo-ui.test>", t.Name, time.Now().UnixNano()),
	}
	for k, v := range t.Headers {
		headers[k] = v
	}
	for k, v := range headers {
		fmt.Fprintf(&b, "%s: %s\r\n", k, v)
	}
	b.WriteString("\r\n")
	// Padding so we don't generate microscopic messages — kumomta's
	// processing per-byte is more interesting at realistic sizes.
	pad := strings.Repeat("x", padBytes)
	b.WriteString("loadgen body: ")
	b.WriteString(t.Name)
	b.WriteString("\r\n")
	b.WriteString(pad)
	b.WriteString("\r\n")
	return []byte(b.String())
}
