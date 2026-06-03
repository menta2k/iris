// Package geoip wraps an MMDB country database for the login firewall's
// REGION rules. It is format-agnostic: any database in the MaxMind DB
// (.mmdb) format that exposes a `country.iso_code` field works — including
// the free DB-IP "IP to Country Lite" database and MaxMind's GeoLite2-Country.
//
// The resolver is hot-swappable: the background updater (see download.go +
// the geoip-updater server) downloads a fresh monthly database and calls
// Reload, which swaps the open handle under a lock without a restart.
//
// It is built to fail open: a missing or unreadable database, or an IP that
// can't be resolved to a country (private, loopback, unparseable), yields an
// empty country code with no error so callers treat REGION rules as
// unenforceable rather than blocking every login.
package geoip

import (
	"errors"
	"io/fs"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// Resolver looks up the ISO-3166-1 alpha-2 country code for an IP. A nil
// *Resolver, or one whose database hasn't loaded yet, reports every IP as
// indeterminate so the firewall fails open while geo is disabled.
type Resolver struct {
	path string

	mu sync.RWMutex
	db *maxminddb.Reader
}

// countryRecord is the minimal subset of an mmdb record we decode. Both
// DB-IP IP-to-Country Lite and MaxMind GeoLite2-Country expose
// `country.iso_code`, so this struct reads either database unchanged.
type countryRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

// New constructs a resolver bound to path and attempts an initial load. A
// missing file is not fatal — the resolver stays disabled (fail-open) until
// the updater downloads one and calls Reload. New never returns an error so
// the admin service always boots.
func New(path string) *Resolver {
	r := &Resolver{path: strings.TrimSpace(path)}
	if err := r.Reload(); err != nil {
		log.Printf("geoip: initial load %q failed: %v — REGION rules disabled until a DB is present", r.path, err)
	}
	return r
}

// Path returns the on-disk database path (used by the updater).
func (r *Resolver) Path() string {
	if r == nil {
		return ""
	}
	return r.path
}

// Reload (re)opens the database from Path and atomically swaps it in,
// closing the previous handle. A missing file disables the resolver and is
// not an error; a corrupt/unreadable existing file returns the error and
// leaves the current handle untouched.
func (r *Resolver) Reload() error {
	if r == nil || r.path == "" {
		return nil
	}
	db, err := maxminddb.Open(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			r.swap(nil)
			return nil
		}
		return err
	}
	r.swap(db)
	return nil
}

// swap replaces the active reader and closes the old one.
func (r *Resolver) swap(db *maxminddb.Reader) {
	r.mu.Lock()
	old := r.db
	r.db = db
	r.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

// CountryISO returns the uppercase ISO-3166-1 alpha-2 country code for ip.
// It returns ("", nil) — indeterminate, fail-open — when the resolver is
// disabled, the IP is empty/unparseable, or the IP is private/loopback (no
// meaningful country). A real lookup error is returned as ("", err).
func (r *Resolver) CountryISO(ip string) (string, error) {
	if r == nil {
		return "", nil
	}
	r.mu.RLock()
	db := r.db
	r.mu.RUnlock()
	if db == nil {
		return "", nil
	}
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsUnspecified() {
		return "", nil
	}
	var rec countryRecord
	if err := db.Lookup(parsed, &rec); err != nil {
		return "", err
	}
	return strings.ToUpper(rec.Country.ISOCode), nil
}

// Close releases the underlying database handle. Safe on a nil/disabled
// resolver.
func (r *Resolver) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db == nil {
		return nil
	}
	err := r.db.Close()
	r.db = nil
	return err
}
