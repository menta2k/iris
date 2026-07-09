package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

// greenarrowDriver POSTs events in GreenArrow's Event-Notification wire format: a
// bare JSON array of event objects (never wrapped), Content-Type application/json,
// success on any 2xx. A single iris event may expand to several GreenArrow events
// (a bad-address bounce becomes both bounce_all and bounce_bad_address), and the
// array is chunked to maxBatch objects per POST to mirror event_delivery_max_batch_size.
// Implements biz.EventDriver.
type greenarrowDriver struct {
	url      string
	maxBatch int
	headers  map[string]string
	client   *http.Client
}

// NewGreenArrowDriverFactory returns the factory registered under the
// "greenarrow" driver name. Config keys: url (required); max_batch_size (optional,
// default 20); timeout (optional duration); headers (optional, "Key: Value" per line).
func NewGreenArrowDriverFactory() biz.EventDriverFactory {
	return func(p *biz.EventProcessor) (biz.EventDriver, error) {
		cfg := p.DriverConfig
		url := strings.TrimSpace(cfg["url"])
		if url == "" {
			return nil, fmt.Errorf("greenarrow driver: url is required")
		}
		maxBatch := 20
		if n, err := strconv.Atoi(strings.TrimSpace(cfg["max_batch_size"])); err == nil && n > 0 {
			maxBatch = n
		}
		timeout := 10 * time.Second
		if d, ok := biz.ParseFlexDuration(cfg["timeout"]); ok && d > 0 {
			timeout = d
		}
		return &greenarrowDriver{
			url:      url,
			maxBatch: maxBatch,
			headers:  parseHeaderLines(cfg["headers"]),
			client:   &http.Client{Timeout: timeout},
		}, nil
	}
}

func (g *greenarrowDriver) Deliver(ctx context.Context, events []biz.DispatchEvent) error {
	objs := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		objs = append(objs, biz.GreenArrowEvents(ev)...)
	}
	for start := 0; start < len(objs); start += g.maxBatch {
		end := start + g.maxBatch
		if end > len(objs) {
			end = len(objs)
		}
		if err := g.post(ctx, objs[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (g *greenarrowDriver) post(ctx context.Context, batch []map[string]any) error {
	body, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal greenarrow events: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build greenarrow request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range g.headers {
		req.Header.Set(k, v)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("greenarrow post: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("greenarrow endpoint returned %d", resp.StatusCode)
	}
	return nil
}
