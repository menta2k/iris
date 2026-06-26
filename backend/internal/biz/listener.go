package biz

import (
	"net"
	"regexp"
	"strings"
)

// safePathRe matches an absolute path with no shell metacharacters (used for
// TLS cert/key paths that are interpolated into the generated policy).
var safePathRe = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

// dnsNameRe validates an EHLO/DNS-compatible hostname (labels of letters,
// digits, and hyphens, separated by dots). Shared by VMTA/DKIM/listener/global
// settings validation.
var dnsNameRe = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

// Listener status values.
const (
	ListenerStatusActive   = "active"
	ListenerStatusDisabled = "disabled"
)

// Listener is an ESMTP listener: an IP + port where mail is received, with an
// EHLO/banner hostname and TLS/relay settings. A VMTA attaches to a listener
// and uses its IP as the outbound egress source and its hostname as the EHLO.
type Listener struct {
	ID             string
	Name           string
	IPAddress      string
	Port           int
	Hostname       string
	TLSEnabled     bool
	TLSCertPath    string
	TLSKeyPath     string
	RequireAuth    bool
	MaxMessageSize int64
	// RelayHosts is the CIDR/IP allowlist permitted to relay (submit outbound)
	// through this listener. As of 3.0.0 the list is authoritative — there is no
	// RFC-1918 fallback. Loopback (127.0.0.1/32) is ALWAYS permitted on every
	// listener so on-box local injection/submission works regardless of config;
	// everything else must be listed explicitly. An EMPTY list therefore means
	// loopback-only: the listener relays only for localhost and otherwise accepts
	// mail only for local/hosted domains (an inbound-only / MX listener). Populate
	// it (e.g. on a :587 submission listener) to authorize additional senders.
	RelayHosts []string
	Status     string
}

// ListenAddr returns the "ip:port" bind string for the listener.
func (l *Listener) ListenAddr() string {
	return net.JoinHostPort(l.IPAddress, itoa(l.Port))
}

// Validate checks listener invariants before persistence.
func (l *Listener) Validate() error {
	l.Name = strings.TrimSpace(l.Name)
	l.IPAddress = strings.TrimSpace(l.IPAddress)
	l.Hostname = strings.TrimSpace(l.Hostname)
	l.TLSCertPath = strings.TrimSpace(l.TLSCertPath)
	l.TLSKeyPath = strings.TrimSpace(l.TLSKeyPath)
	if l.Status == "" {
		l.Status = ListenerStatusActive
	}
	if l.Port == 0 {
		l.Port = 25
	}

	if l.Name == "" {
		return Invalid("LISTENER_NAME_REQUIRED", "listener name is required")
	}
	if len(l.Name) > 64 {
		return Invalid("LISTENER_NAME_TOO_LONG", "listener name must be at most 64 characters")
	}
	if ip := net.ParseIP(l.IPAddress); ip == nil {
		return Invalid("LISTENER_IP_INVALID", "ip_address %q is not a valid IP address", l.IPAddress)
	}
	if l.IPAddress == "0.0.0.0" || l.IPAddress == "::" {
		return Invalid("LISTENER_IP_WILDCARD", "ip_address must be a concrete IP (it is reused as the egress source)")
	}
	if l.Port < 1 || l.Port > 65535 {
		return Invalid("LISTENER_PORT_RANGE", "port must be between 1 and 65535")
	}
	if l.Hostname == "" {
		return Invalid("LISTENER_HOSTNAME_REQUIRED", "hostname (EHLO) is required")
	}
	if len(l.Hostname) > 253 || !dnsNameRe.MatchString(l.Hostname) {
		return Invalid("LISTENER_HOSTNAME_INVALID", "hostname %q is not a valid DNS name", l.Hostname)
	}
	if l.TLSEnabled {
		if l.TLSCertPath == "" || l.TLSKeyPath == "" {
			return Invalid("LISTENER_TLS_PATHS_REQUIRED", "tls_cert_path and tls_key_path are required when TLS is enabled")
		}
		if !safePathRe.MatchString(l.TLSCertPath) || !safePathRe.MatchString(l.TLSKeyPath) {
			return Invalid("LISTENER_TLS_PATH_INVALID", "TLS paths must be absolute and free of shell metacharacters")
		}
	}
	if l.MaxMessageSize < 0 {
		return Invalid("LISTENER_MAX_SIZE_INVALID", "max_message_size must not be negative")
	}
	for _, h := range l.RelayHosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(h); err != nil && net.ParseIP(h) == nil {
			return Invalid("LISTENER_RELAY_HOST_INVALID", "relay host %q is not a valid IP or CIDR", h)
		}
	}
	if l.Status != ListenerStatusActive && l.Status != ListenerStatusDisabled {
		return Invalid("LISTENER_STATUS_INVALID", "status %q is not valid", l.Status)
	}
	return nil
}

// itoa is a tiny dependency-free int-to-string for ports.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
