package biz

import (
	"regexp"
	"strings"
	"time"
)

// DeliveryBlueprint is a base traffic-shaping rule for a receiving-domain MX
// pattern: the default egress-path limits a new/unknown IP starts from and falls
// back to. Blueprints render to the base shaping file (iris-base.toml); the
// warmup engine and TSA layer per-IP overrides on top. Grouped in the UI by
// Provider (a display label / rollup group).
type DeliveryBlueprint struct {
	ID                string
	Provider          string // group label, e.g. "Gmail", "Microsoft", "Yahoo"
	MXPattern         string // receiving domain, e.g. "google.com" (mx_rollup site key)
	ConnRate          string // max_connection_rate, e.g. "5/min"
	DeliveriesPerConn int    // max_deliveries_per_connection
	ConnLimit         int    // connection_limit (default for a new IP)
	DailyCap          int    // base max_message_rate, messages/day
	Status            string // active | disabled
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Blueprint status values.
const (
	BlueprintActive   = "active"
	BlueprintDisabled = "disabled"
)

// connRateRe matches a KumoMTA connection-rate spec: <number>/<unit>, where unit
// is a (sec/min/hour/day) form. Used for max_connection_rate.
var connRateRe = regexp.MustCompile(`^[1-9][0-9]*/(s|sec|second|m|min|minute|h|hr|hour|d|day)$`)

// Validate normalizes and checks a blueprint before persistence.
func (b *DeliveryBlueprint) Validate() error {
	b.Provider = strings.TrimSpace(b.Provider)
	b.MXPattern = strings.ToLower(strings.TrimSpace(b.MXPattern))
	b.ConnRate = strings.ToLower(strings.TrimSpace(b.ConnRate))
	if b.Status == "" {
		b.Status = BlueprintActive
	}
	if b.Provider == "" {
		return Invalid("BLUEPRINT_PROVIDER_REQUIRED", "provider is required")
	}
	if b.MXPattern == "" || len(b.MXPattern) > 253 || !dnsNameRe.MatchString(b.MXPattern) {
		return Invalid("BLUEPRINT_MX_INVALID", "mx_pattern %q is not a valid domain", b.MXPattern)
	}
	if b.ConnRate != "" && !connRateRe.MatchString(b.ConnRate) {
		return Invalid("BLUEPRINT_CONN_RATE_INVALID", "conn_rate %q must be like 5/min", b.ConnRate)
	}
	if b.DeliveriesPerConn < 0 || b.DeliveriesPerConn > 1_000_000 {
		return Invalid("BLUEPRINT_DELIVERIES_RANGE", "deliveries_per_conn out of range")
	}
	if b.ConnLimit < 0 || b.ConnLimit > 100_000 {
		return Invalid("BLUEPRINT_CONN_LIMIT_RANGE", "conn_limit out of range")
	}
	if b.DailyCap < 0 || b.DailyCap > 1_000_000_000 {
		return Invalid("BLUEPRINT_DAILY_CAP_RANGE", "daily_cap out of range")
	}
	if b.Status != BlueprintActive && b.Status != BlueprintDisabled {
		return Invalid("BLUEPRINT_STATUS_INVALID", "status %q is not valid", b.Status)
	}
	return nil
}

// DefaultBlueprints is the built-in provider registry imported by "Seed
// Defaults": the major mailbox providers with conservative starting limits
// (Microsoft ramps more cautiously). Extend as KumoMTA's community shaping set
// evolves.
func DefaultBlueprints() []DeliveryBlueprint {
	bp := func(provider, mx, rate string, deliveries, connLimit, daily int) DeliveryBlueprint {
		return DeliveryBlueprint{
			Provider: provider, MXPattern: mx, ConnRate: rate,
			DeliveriesPerConn: deliveries, ConnLimit: connLimit, DailyCap: daily,
			Status: BlueprintActive,
		}
	}
	return []DeliveryBlueprint{
		// gmail.com is the primary Gmail recipient domain; google.com/googlemail.com
		// are corporate/legacy. With mx_rollup=false the shaping block matches the
		// literal recipient domain, so without gmail.com real Gmail mail is unshaped.
		bp("Gmail", "gmail.com", "5/min", 10, 3, 150),
		bp("Gmail", "google.com", "5/min", 10, 3, 150),
		bp("Gmail", "googlemail.com", "5/min", 10, 3, 150),
		bp("Microsoft", "outlook.com", "3/min", 10, 2, 150),
		bp("Microsoft", "hotmail.com", "3/min", 10, 2, 150),
		bp("Microsoft", "live.com", "3/min", 10, 2, 150),
		bp("Microsoft", "office365.com", "3/min", 10, 2, 150),
		bp("Microsoft", "msn.com", "3/min", 10, 2, 150),
		bp("Yahoo", "yahoodns.net", "5/min", 10, 3, 150),
		bp("Yahoo", "yahoo.com", "5/min", 10, 3, 150),
		bp("Yahoo", "ymail.com", "5/min", 10, 3, 150),
		bp("Yahoo", "aol.com", "5/min", 10, 3, 150),
	}
}
