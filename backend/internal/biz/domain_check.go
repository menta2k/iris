package biz

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// Domain-check statuses.
const (
	CheckPass = "pass"
	CheckWarn = "warn"
	CheckFail = "fail"
)

// DNSResolver is the subset of net.Resolver the domain checker needs (injectable
// for tests). net.DefaultResolver satisfies it.
type DNSResolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// CheckItem is one verification result.
type CheckItem struct {
	Name    string   // "MX", "SPF", or "DKIM (selector)"
	Status  string   // pass | warn | fail
	Detail  string   // human-readable explanation
	Records []string // the DNS records found, for display
}

// DomainBounceCheck bundles the MX/SPF/DKIM results for a domain.
type DomainBounceCheck struct {
	Domain string
	Items  []CheckItem
}

// DomainCheckUsecase verifies a domain's DNS is set up to send and to accept
// bounces for this deployment. It reads the deployment's own listener/egress IPs
// and DKIM selectors from the config snapshot.
type DomainCheckUsecase struct {
	loader ConfigSnapshotLoader
	dns    DNSResolver
}

// NewDomainCheckUsecase constructs the use case. A nil resolver uses the system
// default.
func NewDomainCheckUsecase(loader ConfigSnapshotLoader, dns DNSResolver) *DomainCheckUsecase {
	if dns == nil {
		dns = net.DefaultResolver
	}
	return &DomainCheckUsecase{loader: loader, dns: dns}
}

// Check verifies MX (accepts bounces here), SPF (authorizes our sending IPs),
// and DKIM (selector records published) for a domain.
func (uc *DomainCheckUsecase) Check(ctx context.Context, domain string) (*DomainBounceCheck, error) {
	if _, err := RequirePermission(ctx, PermServiceControl); err != nil {
		return nil, err
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	if err := validateAcmeDomain(domain); err != nil || strings.HasPrefix(domain, "*.") {
		return nil, Invalid("DOMAIN_INVALID", "domain %q is not a valid DNS name", domain)
	}

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

	out := &DomainBounceCheck{Domain: domain}
	out.Items = append(out.Items, checkMX(ctx, uc.dns, domain, listenerIPs))
	out.Items = append(out.Items, checkSPF(ctx, uc.dns, domain, egressIPs))
	out.Items = append(out.Items, checkDKIM(ctx, uc.dns, domain, snap.DKIM)...)
	return out, nil
}

// checkMX verifies the domain has MX records and that at least one resolves to a
// listener IP of this deployment (so bounces land here).
func checkMX(ctx context.Context, dns DNSResolver, domain string, listenerIPs map[string]struct{}) CheckItem {
	item := CheckItem{Name: "MX"}
	mxs, err := dns.LookupMX(ctx, domain)
	if err != nil || len(mxs) == 0 {
		item.Status = CheckFail
		item.Detail = "No MX record found — the domain can't receive bounce/DSN mail."
		return item
	}
	pointsToUs := false
	for _, mx := range mxs {
		host := strings.TrimSuffix(mx.Host, ".")
		item.Records = append(item.Records, host)
		ips, _ := dns.LookupHost(ctx, host)
		for _, ip := range ips {
			if _, ok := listenerIPs[ip]; ok {
				pointsToUs = true
			}
		}
	}
	switch {
	case pointsToUs:
		item.Status = CheckPass
		item.Detail = "MX resolves to a listener IP of this deployment."
	case len(listenerIPs) == 0:
		item.Status = CheckWarn
		item.Detail = "MX records exist, but no listeners are configured to compare against."
	default:
		item.Status = CheckWarn
		item.Detail = "MX records exist but none resolve to this deployment's listener IPs — bounces may not arrive here."
	}
	return item
}

// checkSPF verifies an SPF record exists and explicitly authorizes the egress
// (VMTA) IPs via ip4 mechanisms. a/mx/include are reported as needing manual
// verification (they can't be fully resolved here).
func checkSPF(ctx context.Context, dns DNSResolver, domain string, egressIPs []string) CheckItem {
	item := CheckItem{Name: "SPF"}
	txts, err := dns.LookupTXT(ctx, domain)
	if err != nil {
		item.Status = CheckFail
		item.Detail = "TXT lookup failed: " + err.Error()
		return item
	}
	var spf string
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=spf1") {
			spf = strings.TrimSpace(t)
			break
		}
	}
	if spf == "" {
		item.Status = CheckFail
		item.Detail = "No v=spf1 record — receivers can't verify mail sent from this domain."
		return item
	}
	item.Records = []string{spf}

	var nets []*net.IPNet
	var ips []net.IP
	hasIndirect := false
	for _, tok := range strings.Fields(spf) {
		low := strings.ToLower(tok)
		switch {
		case strings.HasPrefix(low, "ip4:") || strings.HasPrefix(low, "ip6:"):
			v := tok[4:]
			if strings.Contains(v, "/") {
				if _, n, err := net.ParseCIDR(v); err == nil {
					nets = append(nets, n)
				}
			} else if ip := net.ParseIP(v); ip != nil {
				ips = append(ips, ip)
			}
		case low == "a", strings.HasPrefix(low, "a:"), low == "mx", strings.HasPrefix(low, "mx:"),
			strings.HasPrefix(low, "include:"), strings.HasPrefix(low, "redirect="):
			hasIndirect = true
		}
	}
	covered, missing := 0, []string{}
	for _, e := range egressIPs {
		ip := net.ParseIP(e)
		ok := false
		for _, x := range ips {
			if x.Equal(ip) {
				ok = true
			}
		}
		for _, n := range nets {
			if ip != nil && n.Contains(ip) {
				ok = true
			}
		}
		if ok {
			covered++
		} else {
			missing = append(missing, e)
		}
	}
	switch {
	case len(egressIPs) == 0:
		item.Status = CheckWarn
		item.Detail = "SPF record present, but no VMTAs are configured to verify against."
	case len(missing) == 0:
		item.Status = CheckPass
		item.Detail = "SPF authorizes every VMTA egress IP."
	case covered > 0 || hasIndirect:
		item.Status = CheckWarn
		item.Detail = "SPF present; not all egress IPs are explicitly listed (" + strings.Join(missing, ", ") + "). a/mx/include mechanisms need manual verification."
	default:
		item.Status = CheckFail
		item.Detail = "SPF present but does not authorize any VMTA egress IP (" + strings.Join(missing, ", ") + ")."
	}
	return item
}

// checkDKIM verifies the published DKIM TXT record for each selector Iris has
// configured for the domain.
func checkDKIM(ctx context.Context, dns DNSResolver, domain string, dkims []*DKIMDomain) []CheckItem {
	var selectors []string
	for _, d := range dkims {
		if strings.EqualFold(strings.TrimSpace(d.Domain), domain) && strings.TrimSpace(d.Selector) != "" {
			selectors = append(selectors, d.Selector)
		}
	}
	if len(selectors) == 0 {
		return []CheckItem{{
			Name:   "DKIM",
			Status: CheckWarn,
			Detail: "No DKIM selector is configured in Iris for this domain.",
		}}
	}
	var items []CheckItem
	for _, sel := range selectors {
		item := CheckItem{Name: fmt.Sprintf("DKIM (%s)", sel)}
		host := sel + "._domainkey." + domain
		txts, err := dns.LookupTXT(ctx, host)
		joined := strings.Join(txts, "")
		switch {
		case err != nil || joined == "":
			item.Status = CheckFail
			item.Detail = "No DKIM record published at " + host + "."
		case !strings.Contains(joined, "p="):
			item.Status = CheckWarn
			item.Detail = "A TXT record exists at " + host + " but has no public key (p=)."
		default:
			item.Status = CheckPass
			item.Detail = "DKIM key published at " + host + "."
			item.Records = []string{host}
		}
		items = append(items, item)
	}
	return items
}

// checkDMARC verifies a DMARC policy is published at _dmarc.<domain>. A policy of
// p=none is reported as a warning (monitor-only, no enforcement).
func checkDMARC(ctx context.Context, dns DNSResolver, domain string) CheckItem {
	item := CheckItem{Name: "DMARC"}
	host := "_dmarc." + domain
	txts, err := dns.LookupTXT(ctx, host)
	var dmarc string
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), "v=dmarc1") {
			dmarc = strings.TrimSpace(t)
			break
		}
	}
	switch {
	case err != nil && dmarc == "":
		item.Status = CheckFail
		item.Detail = "No DMARC record at " + host + " — receivers have no alignment policy for this domain."
	case dmarc == "":
		item.Status = CheckFail
		item.Detail = "No v=DMARC1 record at " + host + "."
	default:
		item.Records = []string{dmarc}
		policy := ""
		for _, tok := range strings.Split(dmarc, ";") {
			tok = strings.TrimSpace(strings.ToLower(tok))
			if strings.HasPrefix(tok, "p=") {
				policy = strings.TrimSpace(tok[2:])
			}
		}
		if policy == "" || policy == "none" {
			item.Status = CheckWarn
			item.Detail = "DMARC present but p=" + orValue(policy, "none") + " (monitor-only, not enforced)."
		} else {
			item.Status = CheckPass
			item.Detail = "DMARC policy published (p=" + policy + ")."
		}
	}
	return item
}

func orValue(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
