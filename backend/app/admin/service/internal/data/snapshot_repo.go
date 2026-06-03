// SnapshotRepo aggregates the on-disk policy snapshot from every config table
// (listeners, dkim, vmtas, mail classes, routing rules, suppressions). It
// implements service.SnapshotProvider.
package data

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/dkimidentity"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/listenerconfig"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/mailclass"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/routingrule"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/virtualmta"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/virtualmtagroup"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	"github.com/menta2k/iris/backend/pkg/kumopolicy"
)

// SnapshotRepo loads a kumopolicy.Snapshot from ent.
type SnapshotRepo struct{ client *ent.Client }

// NewSnapshotRepo wires the ent client.
func NewSnapshotRepo(c *ent.Client) *SnapshotRepo { return &SnapshotRepo{client: c} }

// CurrentSnapshot reads every config table and returns a flat snapshot.
// Each query is independent — caller's context cancels them all.
func (r *SnapshotRepo) CurrentSnapshot(ctx context.Context) (*kumopolicy.Snapshot, error) {
	listeners, err := r.client.ListenerConfig.Query().
		WithDomains().
		Order(ent.Asc(listenerconfig.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot: listeners: %w", err)
	}
	dkim, err := r.client.DkimIdentity.Query().
		Where(dkimidentity.ActiveEQ(true)).
		Order(ent.Asc(dkimidentity.FieldDomain)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot: dkim: %w", err)
	}
	vmtas, err := r.client.VirtualMta.Query().
		Order(ent.Asc(virtualmta.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot: vmtas: %w", err)
	}
	classes, err := r.client.MailClass.Query().
		Order(ent.Asc(mailclass.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot: classes: %w", err)
	}
	rules, err := r.client.RoutingRule.Query().
		WithConditions().WithTarget().
		Order(ent.Asc(routingrule.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot: rules: %w", err)
	}
	suppressions, err := r.client.SuppressionEntry.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot: suppressions: %w", err)
	}
	groups, err := r.client.VirtualMtaGroup.Query().
		WithMembers(func(q *ent.VirtualMtaGroupMemberQuery) { q.WithVmta() }).
		Order(ent.Asc(virtualmtagroup.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot: vmta groups: %w", err)
	}

	snap := &kumopolicy.Snapshot{
		Listeners:        make([]kumopolicy.Listener, 0, len(listeners)),
		DkimIdentities:   make([]kumopolicy.DkimIdentity, 0, len(dkim)),
		VirtualMtas:      make([]kumopolicy.VirtualMta, 0, len(vmtas)),
		VirtualMtaGroups: make([]kumopolicy.VirtualMtaGroup, 0, len(groups)),
		MailClasses:      make([]kumopolicy.MailClass, 0, len(classes)),
		RoutingRules:     make([]kumopolicy.RoutingRule, 0, len(rules)),
		Suppressions:     make([]kumopolicy.Suppression, 0, len(suppressions)),
	}
	for _, g := range groups {
		members := make([]kumopolicy.VirtualMtaGroupMember, 0, len(g.Edges.Members))
		for _, m := range g.Edges.Members {
			vmtaName := ""
			if v := m.Edges.Vmta; v != nil {
				vmtaName = v.Name
			}
			members = append(members, kumopolicy.VirtualMtaGroupMember{
				VmtaName: vmtaName,
				Weight:   m.Weight,
				Priority: m.Priority,
				Enabled:  m.Enabled,
			})
		}
		snap.VirtualMtaGroups = append(snap.VirtualMtaGroups, kumopolicy.VirtualMtaGroup{
			Name:    g.Name,
			Enabled: g.Enabled,
			Members: members,
		})
	}
	for _, l := range listeners {
		domains := make([]kumopolicy.ListenerDomain, 0, len(l.Edges.Domains))
		for _, d := range l.Edges.Domains {
			domains = append(domains, kumopolicy.ListenerDomain{
				Domain:       d.Domain,
				RelayAllowed: d.RelayAllowed,
				RequireTLS:   d.RequireTLS,
			})
		}
		snap.Listeners = append(snap.Listeners, kumopolicy.Listener{
			Name:           l.Name,
			ListenAddr:     l.ListenAddr,
			Hostname:       l.Hostname,
			TLSEnabled:     l.TLSEnabled,
			TLSCertPath:    l.TLSCertPemPath,
			TLSKeyPath:     l.TLSKeyPemPath,
			RequireAuth:    l.RequireAuth,
			MaxMessageSize: l.MaxMessageSize,
			Domains:        domains,
		})
	}
	for _, d := range dkim {
		snap.DkimIdentities = append(snap.DkimIdentities, kumopolicy.DkimIdentity{
			Domain: d.Domain, Selector: d.Selector,
			Algorithm: d.Algorithm, KeyPath: d.KeyPath,
		})
	}
	for _, v := range vmtas {
		ips := []string{}
		if v.SourceIps != "" {
			for _, p := range strings.Split(v.SourceIps, ",") {
				if t := strings.TrimSpace(p); t != "" {
					ips = append(ips, t)
				}
			}
		}
		snap.VirtualMtas = append(snap.VirtualMtas, kumopolicy.VirtualMta{
			Name: v.Name, SourceIPs: ips, HeloName: v.HeloName,
			MaxConnections:           v.MaxConnections,
			MaxMessagesPerConnection: v.MaxMessagesPerConnection,
			ConnectTimeout:           v.ConnectTimeout,
			ProviderProfile:          v.ProviderProfile,
		})
	}
	for _, c := range classes {
		snap.MailClasses = append(snap.MailClasses, kumopolicy.MailClass{
			Name:       c.Name,
			Enabled:    c.Enabled,
			TargetKind: c.TargetKind,
			TargetRef:  c.TargetRef,
		})
	}
	for _, ru := range rules {
		conds := make([]kumopolicy.RuleCondition, 0, len(ru.Edges.Conditions))
		for _, c := range ru.Edges.Conditions {
			conds = append(conds, kumopolicy.RuleCondition{Field: c.Field, Op: c.Op, Value: c.Value})
		}
		var target kumopolicy.RuleTarget
		if ru.Edges.Target != nil {
			target = kumopolicy.RuleTarget{
				Kind:       ru.Edges.Target.Kind,
				Ref:        ru.Edges.Target.Ref,
				RejectCode: ru.Edges.Target.RejectCode,
				RejectText: ru.Edges.Target.RejectText,
			}
		}
		snap.RoutingRules = append(snap.RoutingRules, kumopolicy.RoutingRule{
			Name: ru.Name, Priority: ru.Priority, Enabled: ru.Enabled,
			Conditions: conds, Target: target,
		})
	}
	for _, s := range suppressions {
		snap.Suppressions = append(snap.Suppressions, kumopolicy.Suppression{
			Address: s.Address, Scope: s.Scope,
		})
	}
	// GlobalSettings come from env so the renderer can emit kumomta-side
	// hooks (Redis log stream, mail-class header override, etc.) without
	// requiring a UI for each knob.
	snap.GlobalSettings = kumopolicy.GlobalSettings{
		LogStreamRedisURL: strings.TrimSpace(os.Getenv("IRIS_LOGSTREAM_REDIS_URL")),
		LogStreamName:     strings.TrimSpace(os.Getenv("IRIS_LOGSTREAM_NAME")),
		LogStreamMaxLen:   strings.TrimSpace(os.Getenv("IRIS_LOGSTREAM_MAXLEN")),
		MailClassHeader:   strings.TrimSpace(os.Getenv("IRIS_MAIL_CLASS_HEADER")),
		// KumoHTTPListen is the bind spec emitted into kumo.start_http_listener.
		// In docker-compose admin-service reaches kumomta on the iris
		// network so '0.0.0.0:8000' is fine. In a host-native install both
		// processes share the loopback interface and would collide on :8000;
		// set to '127.0.0.1:8025' (or similar) and align IRIS_KUMO_API_ENDPOINT.
		KumoHTTPListen:   strings.TrimSpace(os.Getenv("IRIS_KUMO_HTTP_LISTEN")),
		EsmtpListenAddr:  strings.TrimSpace(os.Getenv("IRIS_ESMTP_LISTEN")),
		TestDomainRoutes: parseTestDomainRoutes(),
		QueuePerVmta:     parseBoolEnv("IRIS_QUEUE_PER_VMTA"),
		// Async DSN ingestion. Empty BounceDomain disables the whole
		// pipeline (no VERP rewrite, no inbound catcher). Operators must
		// also publish DNS MX + SPF for the bounce domain — see deploy
		// docs.
		BounceDomain:        strings.TrimSpace(os.Getenv("IRIS_BOUNCE_DOMAIN")),
		BounceSenderDomains: parseDomainList(os.Getenv("IRIS_BOUNCE_SENDER_DOMAINS")),
		BouncePrefix:        strings.TrimSpace(os.Getenv("IRIS_BOUNCE_DOMAIN_PREFIX")),
		VerpSecret:          strings.TrimSpace(os.Getenv("IRIS_VERP_SECRET")),
		DsnStreamName:       strings.TrimSpace(os.Getenv("IRIS_DSNSTREAM_NAME")),
		BounceTokenTTL:      strings.TrimSpace(os.Getenv("IRIS_BOUNCE_TOKEN_TTL")),
	}
	// Merge UI-managed Global Settings on top of the env-derived shell.
	// The DB row wins for any field the operator has explicitly set; an
	// untouched (zero-value) field falls back to the env value above.
	// Failure to read the row is not fatal — we log and proceed with the
	// env-only view so a corrupt singleton can never brick policy renders.
	r.applyGlobalSettings(ctx, snap)
	return snap, nil
}

// applyGlobalSettings overlays the singleton settings row on top of the
// env-derived GlobalSettings. List fields completely replace the env
// list when non-empty (operator intent: "this is the full set"); string
// fields replace the env value when non-empty.
func (r *SnapshotRepo) applyGlobalSettings(ctx context.Context, snap *kumopolicy.Snapshot) {
	row, err := r.client.GlobalSettings.Get(ctx, 1)
	if err != nil {
		// On miss / corrupt row, log and keep the env-only view. The DB
		// migration seeds id=1 so a real miss is rare; tests that don't
		// run the SQL phase will hit this path and that's fine.
		if !ent.IsNotFound(err) {
			log.Printf("snapshot: global_settings read failed (%v) — using env defaults", err)
		}
		return
	}
	if v := strings.TrimSpace(row.KumoHTTPListen); v != "" {
		snap.GlobalSettings.KumoHTTPListen = v
	}
	if v := strings.TrimSpace(row.EsmtpListenAddr); v != "" {
		snap.GlobalSettings.EsmtpListenAddr = v
	}
	if len(row.EsmtpRelayHosts) > 0 {
		snap.GlobalSettings.EsmtpRelayHosts = append([]string(nil), row.EsmtpRelayHosts...)
	}
	if len(row.HTTPTrustedHosts) > 0 {
		snap.GlobalSettings.HTTPTrustedHosts = append([]string(nil), row.HTTPTrustedHosts...)
	}
	if v := strings.TrimSpace(row.BounceDomain); v != "" {
		snap.GlobalSettings.BounceDomain = v
	}
	if len(row.BounceSenderDomains) > 0 {
		snap.GlobalSettings.BounceSenderDomains = append([]string(nil), row.BounceSenderDomains...)
	}
	if v := strings.TrimSpace(row.BouncePrefix); v != "" {
		snap.GlobalSettings.BouncePrefix = v
	}
	if v := strings.TrimSpace(row.MailClassHeader); v != "" {
		snap.GlobalSettings.MailClassHeader = v
	}
	if v := strings.TrimSpace(row.EgressEhloDomain); v != "" {
		snap.GlobalSettings.EgressEhloDomain = v
	}
}

// parseBoolEnv reads a truthy/falsy env var. Accepts 1/true/yes/on
// (case-insensitive) as true; everything else (including unset) is false.
func parseBoolEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// parseDomainList splits a comma-separated env value into a normalised
// list of lowercased, trimmed, non-empty domains. Suitable for
// IRIS_BOUNCE_SENDER_DOMAINS — operators write things like
// "Test-1.com, test2.com," and expect us to clean it up.
func parseDomainList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		d := strings.TrimSpace(strings.ToLower(p))
		// Strip stray leading/trailing dots — operators sometimes paste a
		// FQDN with the trailing dot and we don't want it leaking into
		// the rendered Lua.
		d = strings.Trim(d, ".")
		if d == "" {
			continue
		}
		if _, dup := seen[d]; dup {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// parseTestDomainRoutes reads IRIS_TEST_DOMAIN_ROUTES — a JSON object
// mapping recipient domain → "host:port". A malformed value is logged and
// dropped (test mode is opt-in; we should never break a prod boot on a
// typo'd test env).
func parseTestDomainRoutes() map[string]string {
	v := strings.TrimSpace(os.Getenv("IRIS_TEST_DOMAIN_ROUTES"))
	if v == "" {
		return nil
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		log.Printf("snapshot: IRIS_TEST_DOMAIN_ROUTES is not a valid JSON object: %v (ignored)", err)
		return nil
	}
	return out
}

var _ service.SnapshotProvider = (*SnapshotRepo)(nil)
