package biz

import (
	"context"
	"net"
	"sort"
	"strings"
)

// DiagnoseRequest asks how mail from FromEmail would be handled. Recipient and
// Mailclass are optional and only refine the routing simulation (mailclass and
// routing are header/recipient driven, not determined by the sender alone).
type DiagnoseRequest struct {
	FromEmail string
	Recipient string
	Mailclass string
}

// RoutingOutcome is the simulated routing decision for a message.
type RoutingOutcome struct {
	MatchedRule string   // routing rule name, or "" when none matched (default pool)
	EgressPool  string   // the egress pool that would be used
	VMTAs       []string // VMTA name(s) in that pool
	EgressIPs   []string // egress IP(s) those VMTAs send from
	Listeners   []string // listener(s) those VMTAs are bound to
	Note        string   // explanation, esp. when no rule matched
}

// DiagnoseResult bundles the sending-readiness checks and routing simulation.
type DiagnoseResult struct {
	FromEmail string
	Domain    string
	Items     []CheckItem
	Routing   RoutingOutcome
}

// DiagnoseUsecase reports how mail from a given address is handled by this
// deployment and whether the sending domain is set up correctly. It reuses the
// domain-check DNS helpers and the config snapshot.
type DiagnoseUsecase struct {
	loader   ConfigSnapshotLoader
	dns      DNSResolver
	settings EffectiveSettingsProvider // resolves deployment settings (e.g. DMARC report addr)
}

// NewDiagnoseUsecase constructs the use case. A nil resolver uses the system
// default; a nil settings provider falls back to the raw snapshot.
func NewDiagnoseUsecase(loader ConfigSnapshotLoader, dns DNSResolver, settings EffectiveSettingsProvider) *DiagnoseUsecase {
	if dns == nil {
		dns = net.DefaultResolver
	}
	return &DiagnoseUsecase{loader: loader, dns: dns, settings: settings}
}

// Diagnose runs the DNS sending-readiness checks for the from-address domain and
// simulates routing for an (optional) recipient/mailclass.
func (uc *DiagnoseUsecase) Diagnose(ctx context.Context, req DiagnoseRequest) (*DiagnoseResult, error) {
	if _, err := RequirePermission(ctx, PermServiceControl); err != nil {
		return nil, err
	}
	from := strings.ToLower(strings.TrimSpace(req.FromEmail))
	if !isValidEmail(from) {
		return nil, Invalid("DIAGNOSE_FROM_INVALID", "from_email %q is not a valid email address", req.FromEmail)
	}
	domain := RecipientDomain(from)
	recipient := strings.ToLower(strings.TrimSpace(req.Recipient))
	mailclass := strings.TrimSpace(req.Mailclass)

	snap, err := uc.loader.Snapshot(ctx)
	if err != nil {
		return nil, err
	}

	listenerIPs := map[string]struct{}{}
	for _, l := range snap.Listeners {
		if ip := strings.TrimSpace(l.IPAddress); ip != "" {
			listenerIPs[ip] = struct{}{}
		}
	}
	var egressIPs []string
	for _, v := range snap.VMTAs {
		if ip := strings.TrimSpace(v.IPAddress); ip != "" {
			egressIPs = append(egressIPs, ip)
		}
	}

	out := &DiagnoseResult{FromEmail: from, Domain: domain}
	out.Items = append(out.Items, diagnoseDKIMSigner(domain, snap.DKIM))
	out.Items = append(out.Items, checkDKIM(ctx, uc.dns, domain, snap.DKIM)...)
	out.Items = append(out.Items, checkSPF(ctx, uc.dns, domain, egressIPs))
	// The configured DMARC report address is a deployment setting (merged via the
	// settings provider), not part of the raw snapshot — resolve it explicitly.
	reportAddr := strings.TrimSpace(snap.DMARCReportAddr)
	if uc.settings != nil {
		if eff, serr := uc.settings.Effective(ctx); serr == nil && eff.DMARCReportAddr != "" {
			reportAddr = eff.DMARCReportAddr
		}
	}
	out.Items = append(out.Items, checkDMARC(ctx, uc.dns, domain))
	out.Items = append(out.Items, diagnoseDMARCReporting(ctx, uc.dns, domain, reportAddr))
	out.Items = append(out.Items, checkMX(ctx, uc.dns, domain, listenerIPs))
	out.Items = append(out.Items, diagnoseFBL(domain, snap.FBLEndpoints))
	out.Items = append(out.Items, diagnoseHosted(domain, snap.hostedDomains()))
	out.Routing = simulateRouting(snap, mailclass, recipient)
	return out, nil
}

// diagnoseDKIMSigner reports whether Iris has a DKIM signer configured (and ready)
// for the domain, i.e. whether outbound mail from it will be DKIM-signed.
func diagnoseDKIMSigner(domain string, dkims []*DKIMDomain) CheckItem {
	item := CheckItem{Name: "DKIM signing"}
	var ready, pending []string
	for _, d := range dkims {
		if !strings.EqualFold(strings.TrimSpace(d.Domain), domain) {
			continue
		}
		if d.Status == DKIMReady {
			ready = append(ready, d.Selector)
		} else {
			pending = append(pending, d.Selector)
		}
	}
	switch {
	case len(ready) > 0:
		item.Status = CheckPass
		item.Detail = "Iris will DKIM-sign mail from this domain (selector " + strings.Join(ready, ", ") + ")."
		item.Records = ready
	case len(pending) > 0:
		item.Status = CheckWarn
		item.Detail = "A DKIM signer exists but is not ready (selector " + strings.Join(pending, ", ") + ") — mail will not be signed until it is."
	default:
		item.Status = CheckWarn
		item.Detail = "No DKIM signer is configured in Iris for this domain — mail from it will not be DKIM-signed."
	}
	return item
}

// diagnoseFBL reports whether a feedback loop is configured for the domain.
func diagnoseFBL(domain string, endpoints []*FBLEndpoint) CheckItem {
	item := CheckItem{Name: "Feedback loop"}
	var approved, awaiting []string
	for _, e := range endpoints {
		if e == nil || !strings.EqualFold(strings.TrimSpace(e.Domain), domain) {
			continue
		}
		if e.Status == FBLApproved {
			approved = append(approved, e.FeedbackAddress)
		} else {
			awaiting = append(awaiting, e.FeedbackAddress)
		}
	}
	switch {
	case len(approved) > 0:
		item.Status = CheckPass
		item.Detail = "Feedback loop approved (ARF parsing active): " + strings.Join(approved, ", ") + "."
		item.Records = approved
	case len(awaiting) > 0:
		item.Status = CheckWarn
		item.Detail = "Feedback loop awaiting approval (mail forwarded, not parsed yet): " + strings.Join(awaiting, ", ") + "."
	default:
		item.Status = CheckWarn
		item.Detail = "No feedback loop configured for this domain — complaints won't auto-suppress."
	}
	return item
}

// diagnoseHosted reports whether the domain is a hosted (locally-handled) domain.
func diagnoseHosted(domain string, hosted []string) CheckItem {
	item := CheckItem{Name: "Hosted domain"}
	for _, h := range hosted {
		if strings.EqualFold(strings.TrimSpace(h), domain) {
			item.Status = CheckPass
			item.Detail = "Domain is configured as a hosted (locally-handled) domain."
			return item
		}
	}
	item.Status = CheckWarn
	item.Detail = "Domain is not in the hosted-domains set (treated as a relay/outbound-only domain)."
	return item
}

// diagnoseDMARCReporting checks whether aggregate reports for the domain will
// reach Iris: a report address must be configured (Global Settings) AND the
// domain's published DMARC rua= must include it.
func diagnoseDMARCReporting(ctx context.Context, dns DNSResolver, domain, configuredAddr string) CheckItem {
	item := CheckItem{Name: "DMARC reporting (rua)"}
	configuredAddr = strings.ToLower(strings.TrimSpace(configuredAddr))

	txts, _ := dns.LookupTXT(ctx, "_dmarc."+domain)
	var record string
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
			record = strings.TrimSpace(t)
			break
		}
	}
	rua := parseDMARCRua(record)

	switch {
	case configuredAddr == "":
		item.Status = CheckWarn
		item.Detail = "No DMARC report address is set in Iris (Global Settings → DMARC report address); aggregate reports can't be collected."
	case record == "":
		item.Status = CheckWarn
		item.Detail = "No DMARC record at _dmarc." + domain + " to carry a rua= reporting address."
	case len(rua) == 0:
		item.Status = CheckWarn
		item.Detail = "DMARC record has no rua= — add rua=mailto:" + configuredAddr + " to receive aggregate reports."
	case containsFold(rua, configuredAddr):
		item.Status = CheckPass
		item.Detail = "Aggregate reports for this domain are directed to the configured address (" + configuredAddr + ")."
		item.Records = rua
	default:
		item.Status = CheckWarn
		item.Detail = "rua= points elsewhere (" + strings.Join(rua, ", ") + "); add mailto:" + configuredAddr + " so reports reach Iris."
		item.Records = rua
	}
	return item
}

// parseDMARCRua extracts the rua= mailbox addresses from a DMARC record (the
// "mailto:" scheme and any "!size" suffix stripped), lowercased.
func parseDMARCRua(record string) []string {
	for _, tok := range strings.Split(record, ";") {
		tok = strings.TrimSpace(tok)
		if !strings.HasPrefix(strings.ToLower(tok), "rua=") {
			continue
		}
		var out []string
		for _, uri := range strings.Split(tok[len("rua="):], ",") {
			uri = strings.ToLower(strings.TrimSpace(uri))
			uri = strings.TrimPrefix(uri, "mailto:")
			if i := strings.IndexByte(uri, '!'); i >= 0 {
				uri = uri[:i]
			}
			if uri != "" {
				out = append(out, uri)
			}
		}
		return out
	}
	return nil
}

func containsFold(list []string, want string) bool {
	for _, v := range list {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

// simulateRouting mirrors the rendered select_pool: it walks active routing rules
// in descending priority and returns the egress pool/VMTAs the first matching
// rule targets. mailclass matches the X-Mail-Class value; recipient rules match
// the envelope recipient/domain. With no inputs or no match, the default pool is
// reported.
func simulateRouting(snap ConfigSnapshot, mailclass, recipient string) RoutingOutcome {
	vmtaByID := map[string]*VMTA{}
	for _, v := range snap.VMTAs {
		vmtaByID[v.ID] = v
	}
	groupByID := map[string]*VMTAGroup{}
	for _, g := range snap.Groups {
		groupByID[g.ID] = g
	}
	recipientDomain := RecipientDomain(recipient)

	routes := append([]*RoutingRule(nil), snap.Routes...)
	sort.SliceStable(routes, func(i, j int) bool { return routes[i].Priority > routes[j].Priority })

	for _, r := range routes {
		if r.Status != RoutingStatusActive {
			continue
		}
		matched := false
		switch r.MatchType {
		case MatchMailclass:
			matched = mailclass != "" && mailclass == r.MatchValue
		case MatchRecipientEmail:
			matched = recipient != "" && recipient == strings.ToLower(r.MatchValue)
		case MatchRecipientDomain:
			matched = recipientDomain != "" && recipientDomain == strings.ToLower(r.MatchValue)
		}
		if !matched {
			continue
		}
		out := RoutingOutcome{MatchedRule: r.Name}
		switch r.TargetType {
		case TargetVMTA:
			if v := vmtaByID[r.TargetID]; v != nil {
				out.EgressPool = v.Name
				addVMTA(&out, v)
			}
		case TargetVMTAGroup:
			if g := groupByID[r.TargetID]; g != nil {
				out.EgressPool = g.Name
				for _, m := range g.Members {
					addVMTA(&out, vmtaByID[m.VMTAID])
				}
			}
		}
		return out
	}

	// No rule matched: KumoMTA falls back to the default egress pool. Surface the
	// active VMTAs as the possible egress so the operator knows the sending IPs.
	out := RoutingOutcome{
		EgressPool: "default",
		Note:       "No routing rule matched — KumoMTA uses the default egress pool. Provide a recipient and/or mailclass to simulate a specific rule.",
	}
	for _, v := range snap.VMTAs {
		if v.Status == VMTAStatusActive {
			addVMTA(&out, v)
		}
	}
	return out
}

func addVMTA(out *RoutingOutcome, v *VMTA) {
	if v == nil {
		return
	}
	if v.Name != "" {
		out.VMTAs = append(out.VMTAs, v.Name)
	}
	if ip := strings.TrimSpace(v.IPAddress); ip != "" {
		out.EgressIPs = append(out.EgressIPs, ip)
	}
	if ln := strings.TrimSpace(v.ListenerName); ln != "" {
		out.Listeners = append(out.Listeners, ln)
	}
}
