package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/menta2k/iris/backend/internal/biz"
)

// kumod admin queue control. The KumoMTA HTTP listener (k.cfg.BaseURL) exposes
// Prometheus metrics for live queue depths and /api/admin/{suspend,bounce}/v1 for
// control. Callers must be a trusted host on that listener (iris is co-located).

// scheduledByDomainRe matches a `scheduled_by_domain{...domain="X"...} N`
// Prometheus line (kumod's per-destination scheduled depth), capturing the label
// set and value. Using only the by-domain series avoids double-counting against
// the per-queue scheduled_count series.
var scheduledByDomainRe = regexp.MustCompile(`^scheduled_by_domain\{([^}]*)\}\s+([0-9.eE+-]+)`)

var domainLabelRe = regexp.MustCompile(`domain="([^"]*)"`)

// QueueSummary returns live per-domain scheduled-queue depths from kumod's
// metrics, annotated with any active suspensions. Empty (not an error) when no
// admin base URL is configured.
func (k *FileKumoMTA) QueueSummary(ctx context.Context) ([]*biz.QueueState, error) {
	if strings.TrimSpace(k.cfg.BaseURL) == "" {
		return nil, nil
	}
	body, err := k.adminGET(ctx, "/metrics")
	if err != nil {
		return nil, err
	}
	byDomain := map[string]int64{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		m := scheduledByDomainRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		dm := domainLabelRe.FindStringSubmatch(m[1])
		if dm == nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[2], 64)
		byDomain[strings.ToLower(dm[1])] += int64(v)
	}

	suspended := k.suspensionsByDomain(ctx)

	out := make([]*biz.QueueState, 0, len(byDomain))
	for d, depth := range byDomain {
		qs := &biz.QueueState{Domain: d, Depth: depth}
		if s, ok := suspended[d]; ok {
			qs.Suspended = true
			qs.SuspendID = s.ID
			qs.SuspendReason = s.Reason
		}
		out = append(out, qs)
	}
	// Include suspended domains that currently have no scheduled depth.
	for d, s := range suspended {
		if _, ok := byDomain[d]; ok {
			continue
		}
		out = append(out, &biz.QueueState{Domain: d, Suspended: true, SuspendID: s.ID, SuspendReason: s.Reason})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth > out[j].Depth
		}
		return out[i].Domain < out[j].Domain
	})
	return out, nil
}

type kumoSuspension struct {
	ID     string
	Domain string
	Reason string
}

// suspensionsByDomain fetches active scheduled-queue suspensions, keyed by
// domain. Best-effort: a failure yields an empty map so depths still render.
func (k *FileKumoMTA) suspensionsByDomain(ctx context.Context) map[string]kumoSuspension {
	out := map[string]kumoSuspension{}
	body, err := k.adminGET(ctx, "/api/admin/suspend/v1")
	if err != nil {
		return out
	}
	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return out
	}
	for _, e := range raw {
		s := kumoSuspension{
			ID:     stringField(e, "id"),
			Domain: strings.ToLower(stringField(e, "domain")),
			Reason: stringField(e, "reason"),
		}
		if s.Domain != "" {
			out[s.Domain] = s
		}
	}
	return out
}

// SuspendQueue suspends the scheduled queue for a destination domain.
func (k *FileKumoMTA) SuspendQueue(ctx context.Context, domain, reason string) (string, error) {
	if reason == "" {
		reason = "suspended via iris"
	}
	if err := k.adminJSON(ctx, http.MethodPost, "/api/admin/suspend/v1", map[string]any{
		"domain": domain, "reason": reason,
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("suspended queue for %s", domain), nil
}

// ResumeQueue clears the suspension(s) for a destination domain.
func (k *FileKumoMTA) ResumeQueue(ctx context.Context, domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	found := false
	for d, s := range k.suspensionsByDomain(ctx) {
		if d != domain || s.ID == "" {
			continue
		}
		found = true
		if err := k.adminJSON(ctx, http.MethodDelete, "/api/admin/suspend/v1/"+s.ID, nil); err != nil {
			return "", err
		}
	}
	if !found {
		return "", biz.NotFound("QUEUE_NOT_SUSPENDED", "no active suspension for %s", domain)
	}
	return fmt.Sprintf("resumed queue for %s", domain), nil
}

// BounceQueue administratively bounces (purges) queued messages for a domain.
func (k *FileKumoMTA) BounceQueue(ctx context.Context, domain, reason string) (string, error) {
	if reason == "" {
		reason = "bounced via iris"
	}
	if err := k.adminJSON(ctx, http.MethodPost, "/api/admin/bounce/v1", map[string]any{
		"domain": domain, "reason": reason,
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("bounced queued messages for %s", domain), nil
}

func (k *FileKumoMTA) adminGET(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.adminURL(path), nil)
	if err != nil {
		return nil, biz.Internal(err, "build kumod request")
	}
	resp, err := k.client.Do(req)
	if err != nil {
		return nil, biz.Unavailable("KUMO_ADMIN_UNREACHABLE", "kumod admin endpoint unreachable: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, biz.Unavailable("KUMO_ADMIN_FAILED", "kumod %s returned %d", path, resp.StatusCode)
	}
	return body, nil
}

func (k *FileKumoMTA) adminJSON(ctx context.Context, method, path string, payload any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return biz.Internal(err, "marshal kumod request")
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, k.adminURL(path), body)
	if err != nil {
		return biz.Internal(err, "build kumod request")
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := k.client.Do(req)
	if err != nil {
		return biz.Unavailable("KUMO_ADMIN_UNREACHABLE", "kumod admin endpoint unreachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return biz.Unavailable("KUMO_ADMIN_FAILED", "kumod %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

func (k *FileKumoMTA) adminURL(path string) string {
	return strings.TrimRight(k.cfg.BaseURL, "/") + path
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
