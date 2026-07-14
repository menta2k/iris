// Package netutil provides small host-networking helpers shared by iris and
// the iris-agent.
package netutil

import (
	"net"
	"sort"
)

// LocalIPs returns the host's assignable global-unicast IP addresses (IPv4 and
// IPv6), sorted and de-duplicated. Loopback, link-local, multicast, and
// unspecified addresses are excluded — the result is the set of addresses a
// listener or egress source can realistically bind on this host. IPv4 sorts
// before IPv6, then lexically.
func LocalIPs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // skip down interfaces
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || !ip.IsGlobalUnicast() {
				continue
			}
			s := ip.String()
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		i4, j4 := net.ParseIP(out[i]).To4() != nil, net.ParseIP(out[j]).To4() != nil
		if i4 != j4 {
			return i4 // IPv4 first
		}
		return out[i] < out[j]
	})
	return out, nil
}
