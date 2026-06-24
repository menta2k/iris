package biz

import (
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// rblZones is the curated set of DNS blocklists checked. Results are only
// reliable from a non-public DNS resolver (Spamhaus/others return errors or
// blanket listings to public resolvers like 8.8.8.8).
var rblZones = []string{
	"zen.spamhaus.org",
	"b.barracudacentral.org",
	"bl.spamcop.net",
	"dnsbl.sorbs.net",
}

// rblConcurrency bounds simultaneous DNSBL lookups.
const rblConcurrency = 8

// RBLListing is the result of checking one IP against one blocklist zone.
type RBLListing struct {
	Zone   string
	Listed bool
	Reason string // the zone's TXT explanation when listed
}

// RBLIPResult is the per-IP rollup across all zones.
type RBLIPResult struct {
	IP       string
	Source   string // "listener", "egress", or "listener, egress"
	Listed   bool   // true if listed on any zone
	Listings []RBLListing
}

// RBLReport is the full RBL check across the deployment's IPs.
type RBLReport struct {
	Results   []RBLIPResult
	Zones     []string
	CheckedAt time.Time
	// Skipped holds non-IPv4 addresses that were not checked (DNSBLs are IPv4).
	Skipped []string
}

// RBLUsecase checks the deployment's listener and VMTA egress IPs against DNS
// blocklists.
type RBLUsecase struct {
	loader ConfigSnapshotLoader
	dns    DNSResolver
}

// NewRBLUsecase constructs the use case. A nil resolver uses the system default.
func NewRBLUsecase(loader ConfigSnapshotLoader, dns DNSResolver) *RBLUsecase {
	if dns == nil {
		dns = net.DefaultResolver
	}
	return &RBLUsecase{loader: loader, dns: dns}
}

// Check resolves every distinct IPv4 listener/egress IP against rblZones and
// returns the per-IP results. Lookups are concurrent and fail-safe: an IP is
// only reported listed on a positive (resolving) answer, never on an error.
func (uc *RBLUsecase) Check(ctx context.Context) (*RBLReport, error) {
	if _, err := RequirePermission(ctx, PermServiceControl); err != nil {
		return nil, err
	}
	snap, err := uc.loader.Snapshot(ctx)
	if err != nil {
		return nil, err
	}

	sources := map[string]map[string]struct{}{} // ip -> set of sources
	var skipped []string
	add := func(ip, src string) {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return
		}
		if net.ParseIP(ip).To4() == nil {
			skipped = append(skipped, ip)
			return
		}
		if sources[ip] == nil {
			sources[ip] = map[string]struct{}{}
		}
		sources[ip][src] = struct{}{}
	}
	for _, l := range snap.Listeners {
		add(l.IPAddress, "listener")
	}
	for _, v := range snap.VMTAs {
		add(v.IPAddress, "egress")
	}

	ips := make([]string, 0, len(sources))
	for ip := range sources {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	sort.Strings(skipped)

	// Concurrent (ip, zone) lookups.
	type job struct{ ip, zone string }
	var jobs []job
	for _, ip := range ips {
		for _, z := range rblZones {
			jobs = append(jobs, job{ip, z})
		}
	}
	listings := map[string][]RBLListing{}
	var mu sync.Mutex
	sem := make(chan struct{}, rblConcurrency)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			l := uc.lookup(ctx, j.ip, j.zone)
			mu.Lock()
			listings[j.ip] = append(listings[j.ip], l)
			mu.Unlock()
		}(j)
	}
	wg.Wait()

	out := &RBLReport{Zones: rblZones, CheckedAt: time.Now().UTC(), Skipped: skipped}
	for _, ip := range ips {
		ls := listings[ip]
		sort.Slice(ls, func(i, j int) bool { return ls[i].Zone < ls[j].Zone })
		res := RBLIPResult{IP: ip, Source: joinSources(sources[ip]), Listings: ls}
		for _, l := range ls {
			if l.Listed {
				res.Listed = true
			}
		}
		out.Results = append(out.Results, res)
	}
	return out, nil
}

// lookup queries one DNSBL. DNSBL semantics: a resolving A record means listed
// (and the TXT carries the reason); NXDOMAIN/any error means not listed.
func (uc *RBLUsecase) lookup(ctx context.Context, ip, zone string) RBLListing {
	query := reverseIPv4(ip) + "." + zone
	addrs, err := uc.dns.LookupHost(ctx, query)
	if err != nil || len(addrs) == 0 {
		return RBLListing{Zone: zone, Listed: false}
	}
	reason := ""
	if txts, terr := uc.dns.LookupTXT(ctx, query); terr == nil {
		reason = strings.TrimSpace(strings.Join(txts, " "))
	}
	return RBLListing{Zone: zone, Listed: true, Reason: reason}
}

// reverseIPv4 turns "1.2.3.4" into "4.3.2.1" for DNSBL queries.
func reverseIPv4(ip string) string {
	p := strings.Split(ip, ".")
	if len(p) != 4 {
		return ip
	}
	return p[3] + "." + p[2] + "." + p[1] + "." + p[0]
}

func joinSources(set map[string]struct{}) string {
	var s []string
	// stable order: listener before egress
	for _, name := range []string{"listener", "egress"} {
		if _, ok := set[name]; ok {
			s = append(s, name)
		}
	}
	return strings.Join(s, ", ")
}
