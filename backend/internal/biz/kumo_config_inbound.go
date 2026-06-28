package biz

import (
	"fmt"
	"sort"
	"strings"
)

// DefaultMaildirBase is the deployment-wide Maildir root used when no base is
// configured. A maildir route with an empty MaildirPath lands under
// DefaultMaildirBase/<domain>/<local-part>.
const DefaultMaildirBase = "/var/spool/iris/maildirs"

// webhookTrackerQueue is the synthetic queue name for the in-policy webhook
// poster (make.webhook_post).
const webhookTrackerQueue = "iris_webhook"

// activeRoute reports whether a route is renderable (active and complete for its
// action).
func activeRoute(r *InboundRoute) bool {
	if r == nil || r.Status != InboundRouteActive {
		return false
	}
	switch r.Action {
	case InboundActionWebhook:
		return strings.TrimSpace(r.DestinationURL) != ""
	case InboundActionForward:
		return strings.TrimSpace(r.ForwardHost) != ""
	case InboundActionMaildir:
		return true
	default:
		return false
	}
}

// rspamdMachineryEnabled reports whether the rspamd scan function should be
// rendered: the global mode is tag/enforce, or some active route opts into
// scanning. Requires a configured rspamd URL either way.
func rspamdMachineryEnabled(snap ConfigSnapshot) bool {
	if strings.TrimSpace(snap.RspamdURL) == "" {
		return false
	}
	if snap.rspamdEnabled() {
		return true
	}
	for _, r := range snap.InboundRoutes {
		if activeRoute(r) && (r.SpamScan == ScanTag || r.SpamScan == ScanEnforce) {
			return true
		}
	}
	return false
}

// effectiveScan resolves a route's scan mode to off/tag/enforce for rendering.
// "default" follows the deployment-wide rspamd mode; an explicit tag/enforce is
// honored only when the rspamd machinery is available (a URL is set).
func effectiveScan(r *InboundRoute, snap ConfigSnapshot) string {
	if !rspamdMachineryEnabled(snap) {
		return ScanOff
	}
	switch r.SpamScan {
	case ScanOff:
		return ScanOff
	case ScanTag:
		return ScanTag
	case ScanEnforce:
		return ScanEnforce
	default: // default / empty -> follow the global mode
		if !snap.rspamdEnabled() {
			return ScanOff
		}
		if snap.rspamdEnforce() {
			return ScanEnforce
		}
		return ScanTag
	}
}

// inboundRoutesEnabled reports whether any inbound route is rendered.
func inboundRoutesEnabled(snap ConfigSnapshot) bool {
	for _, r := range snap.InboundRoutes {
		if activeRoute(r) {
			return true
		}
	}
	return false
}

// webhookEnabled reports whether any active webhook route is configured (and so
// whether the in-policy webhook poster is rendered).
func webhookEnabled(snap ConfigSnapshot) bool {
	return anyRouteAction(snap, InboundActionWebhook)
}

func maildirEnabled(snap ConfigSnapshot) bool { return anyRouteAction(snap, InboundActionMaildir) }
func forwardEnabled(snap ConfigSnapshot) bool { return anyRouteAction(snap, InboundActionForward) }

func anyRouteAction(snap ConfigSnapshot, action string) bool {
	for _, r := range snap.InboundRoutes {
		if activeRoute(r) && r.Action == action {
			return true
		}
	}
	return false
}

// sortedActiveRoutes returns the renderable routes in deterministic, priority
// order: higher Priority first, then by name, then match value. Recipient
// uniqueness (one action per match) is enforced by a partial unique index, but
// rendering also dedupes defensively, keeping the first (highest-priority) route
// for a given (match_type, match_value).
func sortedActiveRoutes(snap ConfigSnapshot) []*InboundRoute {
	var out []*InboundRoute
	for _, r := range snap.InboundRoutes {
		if activeRoute(r) {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].MatchValue < out[j].MatchValue
	})
	seen := map[string]bool{}
	deduped := out[:0]
	for _, r := range out {
		key := r.MatchType + "\x00" + strings.ToLower(strings.TrimSpace(r.MatchValue))
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, r)
	}
	return deduped
}

// resolvedMaildirPath returns the maildir path for a route: its explicit
// MaildirPath, or the deployment base plus a per-user template.
func resolvedMaildirPath(r *InboundRoute, base string) string {
	if p := strings.TrimSpace(r.MaildirPath); p != "" {
		return p
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = DefaultMaildirBase
	}
	return base + "/{{ domain_part }}/{{ local_part }}"
}

// forwardMX returns the smarthost "host:port" delivery target for a forward route.
func forwardMX(r *InboundRoute) string {
	port := r.ForwardPort
	if port == 0 {
		port = DefaultForwardPort
	}
	return fmt.Sprintf("%s:%d", strings.ToLower(strings.TrimSpace(r.ForwardHost)), port)
}

// forwardTarget is a unique forwarding smarthost addressed by a synthetic queue
// key. Routes sharing the same mx+tls collapse onto one target.
type forwardTarget struct {
	key string
	mx  string
	tls string
}

// maildirTarget is a unique maildir destination addressed by a synthetic queue
// key. Routes resolving to the same path collapse onto one target.
type maildirTarget struct {
	key  string
	path string
}

// forwardTargets returns the deduped forwarding smarthosts (deterministic order)
// and a map from each renderable forward route's MX to its synthetic queue key.
func forwardTargets(snap ConfigSnapshot) ([]forwardTarget, map[string]string) {
	keyByMX := map[string]string{}
	var targets []forwardTarget
	for _, r := range sortedActiveRoutes(snap) {
		if r.Action != InboundActionForward {
			continue
		}
		mx := forwardMX(r)
		if _, ok := keyByMX[mx]; ok {
			continue
		}
		key := fmt.Sprintf("iris_forward_%d", len(targets))
		keyByMX[mx] = key
		targets = append(targets, forwardTarget{key: key, mx: mx, tls: r.ForwardTLS})
	}
	return targets, keyByMX
}

// maildirTargets returns the deduped maildir destinations (deterministic order)
// and a map from each resolved path to its synthetic queue key.
func maildirTargets(snap ConfigSnapshot) ([]maildirTarget, map[string]string) {
	base := snap.InboundMaildirBase
	keyByPath := map[string]string{}
	var targets []maildirTarget
	for _, r := range sortedActiveRoutes(snap) {
		if r.Action != InboundActionMaildir {
			continue
		}
		path := resolvedMaildirPath(r, base)
		if _, ok := keyByPath[path]; ok {
			continue
		}
		key := fmt.Sprintf("iris_maildir_%d", len(targets))
		keyByPath[path] = key
		targets = append(targets, maildirTarget{key: key, path: path})
	}
	return targets, keyByPath
}

// writeInboundRoutes emits the inbound-route dispatch tables (consumed by the
// reception hook, get_listener_domain and get_queue_config) plus the
// make.webhook_post constructor. Each recipient maps to a single action queue:
// the webhook poster, a maildir destination, or a forwarding smarthost. No-op
// when no route is configured.
func writeInboundRoutes(b *strings.Builder, snap ConfigSnapshot) {
	if !inboundRoutesEnabled(snap) {
		return
	}
	fwdTargets, fwdKeyByMX := forwardTargets(snap)
	mdTargets, mdKeyByPath := maildirTargets(snap)

	b.WriteString(`-- ===== inbound routes (maildir / forward / webhook) =====
local WEBHOOK_TRACKER = "iris_webhook"
-- recipient -> { queue = <queue name>, class = <mailclass> }
local ROUTE_BY_EMAIL = {}
local ROUTE_BY_DOMAIN = {}
-- recipient domains relayed at the listener so the reception hook can dispatch
local ROUTE_DOMAINS = {}
-- webhook poster lookups (raw RFC822 POST), used by make.webhook_post
local WEBHOOK_BY_EMAIL = {}
local WEBHOOK_BY_DOMAIN = {}
-- maildir destination path, keyed by synthetic queue name
local MAILDIR_PATHS = {}
-- forward smarthost { mx = 'host:port' }, keyed by synthetic queue name
local FORWARD_SMARTHOSTS = {}
`)

	for _, t := range mdTargets {
		fmt.Fprintf(b, "MAILDIR_PATHS[%s] = %s\n", MustLuaString(t.key), MustLuaString(t.path))
	}
	for _, t := range fwdTargets {
		fmt.Fprintf(b, "FORWARD_SMARTHOSTS[%s] = { mx = %s }\n", MustLuaString(t.key), MustLuaString(t.mx))
	}

	for _, r := range sortedActiveRoutes(snap) {
		value := strings.ToLower(strings.TrimSpace(r.MatchValue))
		var queue, class string
		switch r.Action {
		case InboundActionWebhook:
			queue, class = webhookTrackerQueue, "webhook"
			entry := fmt.Sprintf("{ url = %s, secret = %s }", MustLuaString(r.DestinationURL), MustLuaString(r.SecretRef))
			if r.MatchType == MatchRecipientEmail {
				fmt.Fprintf(b, "WEBHOOK_BY_EMAIL[%s] = %s\n", MustLuaString(value), entry)
			} else {
				fmt.Fprintf(b, "WEBHOOK_BY_DOMAIN[%s] = %s\n", MustLuaString(value), entry)
			}
		case InboundActionMaildir:
			queue, class = mdKeyByPath[resolvedMaildirPath(r, snap.InboundMaildirBase)], "maildir"
		case InboundActionForward:
			queue, class = fwdKeyByMX[forwardMX(r)], "forward"
		default:
			continue
		}
		routeEntry := fmt.Sprintf("{ queue = %s, class = %s, scan = %s }",
			MustLuaString(queue), MustLuaString(class), MustLuaString(effectiveScan(r, snap)))
		if r.MatchType == MatchRecipientEmail {
			fmt.Fprintf(b, "ROUTE_BY_EMAIL[%s] = %s\n", MustLuaString(value), routeEntry)
		} else {
			fmt.Fprintf(b, "ROUTE_BY_DOMAIN[%s] = %s\n", MustLuaString(value), routeEntry)
		}
		fmt.Fprintf(b, "ROUTE_DOMAINS[%s] = true\n", MustLuaString(r.RouteDomain()))
	}

	if webhookEnabled(snap) {
		b.WriteString(`
kumo.on('make.webhook_post', function(_domain, _tenant, _campaign)
  local connection = {}
  function connection:send(message)
    local rcpt = message:recipient()
    local email = (rcpt and rcpt.email or ''):lower()
    local dom = (rcpt and rcpt.domain or ''):lower()
    local route = WEBHOOK_BY_EMAIL[email] or WEBHOOK_BY_DOMAIN[dom]
    if not route then
      return '250 no webhook route (dropped)'
    end
    local body = message:get_data()
    local client = kumo.http.build_client {}
    local req = client:post(route.url)
    req:header('Content-Type', 'message/rfc822')
    req:header('X-Iris-Recipient', email)
    req:header('X-Iris-Message-Id', tostring(message:id()))
    if route.secret and route.secret ~= '' then
      local sig = kumo.digest.hmac_sha256({ key_data = route.secret }, body)
      req:header('X-Iris-Signature', tostring(sig))
    end
    req:body(body)
    local ok, resp = pcall(function() return req:send() end)
    if not ok then
      kumo.log_error('webhook: post failed err=' .. tostring(resp))
      return string.format('451 4.4.1 webhook post error: %s', tostring(resp))
    end
    local code = resp:status_code()
    if code >= 200 and code < 300 then
      return string.format('250 forwarded to webhook (%d)', code)
    end
    return string.format('451 4.4.1 webhook returned %d', code)
  end
  return connection
end)
`)
	}
	b.WriteString("\n")
}
