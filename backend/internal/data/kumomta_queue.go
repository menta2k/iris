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

// kumod admin queue control. Each node's kumod HTTP listener exposes
// Prometheus metrics for live queue depths and /api/admin/{suspend,bounce}/v1
// for control. Local nodes are reached directly (trusted, co-located); remote
// nodes through their agent's mTLS /v1/kumod reverse proxy. Queues exist
// independently on every node, so summaries aggregate and actions fan out.

// scheduledByDomainRe matches a `scheduled_by_domain{...domain="X"...} N`
// Prometheus line (kumod's per-destination scheduled depth), capturing the label
// set and value. Using only the by-domain series avoids double-counting against
// the per-queue scheduled_count series.
var scheduledByDomainRe = regexp.MustCompile(`^scheduled_by_domain\{([^}]*)\}\s+([0-9.eE+-]+)`)

var domainLabelRe = regexp.MustCompile(`domain="([^"]*)"`)

// adminTargets returns the nodes whose kumod admin channel is reachable.
func (k *FileKumoMTA) adminTargets(ctx context.Context) ([]applyTarget, error) {
	targets, err := k.applyTargets(ctx)
	if err != nil {
		return nil, err
	}
	out := targets[:0:0]
	for _, t := range targets {
		if t.transport.adminAvailable() {
			out = append(out, t)
		}
	}
	return out, nil
}

// QueueSummary returns live per-domain scheduled-queue depths aggregated
// across every participating node, annotated with active suspensions (a domain
// counts as suspended when it is suspended on ANY node). Empty (not an error)
// when no admin channel is configured. Unreachable nodes are skipped
// best-effort; the error is returned only when every node fails.
func (k *FileKumoMTA) QueueSummary(ctx context.Context) ([]*biz.QueueState, error) {
	targets, err := k.adminTargets(ctx)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}

	byDomain := map[string]int64{}
	suspended := map[string]kumoSuspension{}
	var scraped int
	var firstErr error
	for _, t := range targets {
		body, err := t.transport.adminGET(ctx, "/metrics")
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("node %s: %w", t.name, err)
			}
			biz.LoggerFrom(ctx).Warn("queue summary: node scrape failed", "node", t.name, "error", err.Error())
			continue
		}
		scraped++
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
		for d, s := range suspensionsByDomain(ctx, t.transport) {
			if _, ok := suspended[d]; !ok {
				suspended[d] = s
			}
		}
	}
	if scraped == 0 {
		return nil, firstErr
	}

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

// suspensionsByDomain fetches one node's active scheduled-queue suspensions,
// keyed by domain. Best-effort: a failure yields an empty map so depths still
// render.
func suspensionsByDomain(ctx context.Context, t nodeTransport) map[string]kumoSuspension {
	out := map[string]kumoSuspension{}
	body, err := t.adminGET(ctx, "/api/admin/suspend/v1")
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

// fanOutQueueAction runs a per-node action on every admin-reachable node and
// composes a summary. Partial failure is an error naming the failed nodes so
// the operator knows some nodes still hold the previous state.
func (k *FileKumoMTA) fanOutQueueAction(ctx context.Context, verb string, action func(t applyTarget) error) (string, error) {
	targets, err := k.adminTargets(ctx)
	if err != nil {
		return "", err
	}
	if len(targets) == 0 {
		return "", biz.Unavailable("KUMO_ADMIN_UNCONFIGURED", "no kumod admin endpoint configured")
	}
	var okNodes, failed []string
	var firstErr error
	for _, t := range targets {
		if err := action(t); err != nil {
			failed = append(failed, t.name)
			if firstErr == nil {
				firstErr = fmt.Errorf("node %s: %w", t.name, err)
			}
			continue
		}
		okNodes = append(okNodes, t.name)
	}
	if len(failed) > 0 {
		if len(okNodes) == 0 {
			return "", firstErr
		}
		return "", fmt.Errorf("%s succeeded on %s but FAILED on %s: %w",
			verb, strings.Join(okNodes, ", "), strings.Join(failed, ", "), firstErr)
	}
	if len(targets) == 1 {
		return verb, nil
	}
	return fmt.Sprintf("%s on %s", verb, strings.Join(okNodes, ", ")), nil
}

// SuspendQueue suspends the scheduled queue for a destination domain on every
// participating node.
func (k *FileKumoMTA) SuspendQueue(ctx context.Context, domain, reason string) (string, error) {
	if reason == "" {
		reason = "suspended via iris"
	}
	return k.fanOutQueueAction(ctx, fmt.Sprintf("suspended queue for %s", domain), func(t applyTarget) error {
		return t.transport.adminJSON(ctx, http.MethodPost, "/api/admin/suspend/v1", map[string]any{
			"domain": domain, "reason": reason,
		})
	})
}

// ResumeQueue clears the suspension(s) for a destination domain on every node
// that holds one (suspension ids are per node).
func (k *FileKumoMTA) ResumeQueue(ctx context.Context, domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	found := false
	summary, err := k.fanOutQueueAction(ctx, fmt.Sprintf("resumed queue for %s", domain), func(t applyTarget) error {
		for d, s := range suspensionsByDomain(ctx, t.transport) {
			if d != domain || s.ID == "" {
				continue
			}
			found = true
			if err := t.transport.adminJSON(ctx, http.MethodDelete, "/api/admin/suspend/v1/"+s.ID, nil); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if !found {
		return "", biz.NotFound("QUEUE_NOT_SUSPENDED", "no active suspension for %s", domain)
	}
	return summary, nil
}

// BounceQueue administratively bounces (purges) queued messages for a domain
// on every participating node.
func (k *FileKumoMTA) BounceQueue(ctx context.Context, domain, reason string) (string, error) {
	if reason == "" {
		reason = "bounced via iris"
	}
	return k.fanOutQueueAction(ctx, fmt.Sprintf("bounced queued messages for %s", domain), func(t applyTarget) error {
		return t.transport.adminJSON(ctx, http.MethodPost, "/api/admin/bounce/v1", map[string]any{
			"domain": domain, "reason": reason,
		})
	})
}

// ---- shared kumod HTTP helpers (used by both transports) -------------------

// kumodGET fetches a kumod admin/metrics URL. path is used in error messages.
func kumodGET(ctx context.Context, client *http.Client, url, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, biz.Internal(err, "build kumod request")
	}
	resp, err := client.Do(req)
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

// kumodJSON sends a JSON admin request to a kumod URL.
func kumodJSON(ctx context.Context, client *http.Client, method, url, path string, payload any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return biz.Internal(err, "marshal kumod request")
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return biz.Internal(err, "build kumod request")
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
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

// kumodInject posts a built message to a kumod /api/inject/v1 URL.
func kumodInject(ctx context.Context, client *http.Client, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return biz.Internal(err, "build kumo inject request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return biz.Unavailable("KUMO_INJECT_UNREACHABLE", "kumod injection endpoint unreachable: %v", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return biz.Unavailable("KUMO_INJECT_FAILED", "kumod /api/inject/v1 returned %d: %s",
			resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
