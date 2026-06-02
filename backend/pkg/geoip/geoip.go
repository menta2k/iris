// Package geoip wraps the MaxMind GeoLite2 country database for the login
// firewall's REGION rules. It is built to fail open: a missing or unreadable
// database, or an IP that can't be resolved to a country (private, loopback,
// unparseable), yields an empty country code with no error so callers treat
// REGION rules as unenforceable rather than blocking every login.
package geoip

import (
	"net"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

// Resolver looks up the ISO-3166-1 alpha-2 country code for an IP. A nil
// *Resolver (or one with no loaded database) is valid and reports every IP
// as indeterminate, so the firewall fails open when geo is disabled.
type Resolver struct {
	db *geoip2.Reader
}

// Open loads the GeoLite2-Country .mmdb at path. A missing or unreadable
// file is NOT an error: Open returns (nil, nil) so boot proceeds with geo
// disabled. A genuine open failure on an existing file is returned so the
// operator sees a misconfiguration, but callers may still choose to proceed.
func Open(path string) (*Resolver, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	db, err := geoip2.Open(path)
	if err != nil {
		// Treat "not found" as disabled rather than fatal; the firewall
		// logs a warning when it sees REGION rules with no resolver.
		if isNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &Resolver{db: db}, nil
}

// CountryISO returns the uppercase ISO-3166-1 alpha-2 country code for ip.
// It returns ("", nil) — indeterminate, fail-open — when the resolver is
// disabled, the IP is empty/unparseable, or the IP is private/loopback (no
// meaningful country). A real lookup error is returned as (", err).
func (r *Resolver) CountryISO(ip string) (string, error) {
	if r == nil || r.db == nil {
		return "", nil
	}
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsUnspecified() {
		return "", nil
	}
	rec, err := r.db.Country(parsed)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(rec.Country.IsoCode), nil
}

// Close releases the underlying database handle. Safe on a nil/disabled
// resolver.
func (r *Resolver) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func isNotExist(err error) bool {
	if err == nil {
		return false
	}
	// geoip2.Open wraps os errors; a substring check keeps us off the
	// platform-specific syscall error types.
	msg := err.Error()
	return strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "cannot find the file") ||
		strings.Contains(msg, "system cannot find")
}
