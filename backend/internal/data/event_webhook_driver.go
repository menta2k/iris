package data

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// webhookDriver POSTs a JSON body {"events": [...]} to a configured URL, with an
// optional HMAC-SHA256 signature and custom headers. Implements biz.EventDriver.
type webhookDriver struct {
	url     string
	secret  string
	format  string
	headers map[string]string
	client  *http.Client
}

// NewWebhookDriverFactory returns the factory registered under the "webhook"
// driver name. Config keys: url (required), secret (optional HMAC), timeout
// (optional duration), headers (optional, "Key: Value" per line).
func NewWebhookDriverFactory() biz.EventDriverFactory {
	return func(p *biz.EventProcessor) (biz.EventDriver, error) {
		cfg := p.DriverConfig
		timeout := 10 * time.Second
		if d, ok := biz.ParseFlexDuration(cfg["timeout"]); ok && d > 0 {
			timeout = d
		}
		return &webhookDriver{
			url:     strings.TrimSpace(cfg["url"]),
			secret:  cfg["secret"],
			format:  cfg["format"],
			headers: parseHeaderLines(cfg["headers"]),
			client:  &http.Client{Timeout: timeout},
		}, nil
	}
}

func (w *webhookDriver) Deliver(ctx context.Context, events []biz.DispatchEvent) error {
	formatted := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		formatted = append(formatted, biz.FormatEvent(w.format, ev))
	}
	body, err := json.Marshal(map[string]any{"events": formatted})
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "iris-event-processor")
	if w.secret != "" {
		mac := hmac.New(sha256.New, []byte(w.secret))
		mac.Write(body)
		req.Header.Set("X-Iris-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook post: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

// parseHeaderLines parses "Key: Value" lines into a header map.
func parseHeaderLines(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}
