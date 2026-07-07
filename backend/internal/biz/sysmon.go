package biz

import (
	"fmt"
	"strings"
	"time"
)

// Self-monitoring: iris samples host CPU / memory / disk, compares them against
// operator thresholds, and emails an alert when a resource is over the line
// (and a recovery notice when it drops back). This file holds the pure domain
// types + threshold evaluation; the sampler, repo, notifier, and worker live
// alongside.

// Monitored resources.
const (
	ResourceCPU  = "cpu"
	ResourceMem  = "memory"
	ResourceDisk = "disk"
)

// Alert levels (the transition being reported).
const (
	AlertBreached  = "breached"
	AlertRecovered = "recovered"
)

// Defaults applied by NormalizeMonitorSettings.
const (
	defaultSMTPHost   = "localhost:25"
	defaultCooldown   = 30 // minutes
	defaultSample     = 30 // seconds
	defaultDiskPath   = "/"
	minSampleSeconds  = 5
	maxSampleSeconds  = 3600
	maxCooldownMinute = 24 * 60
)

// DiskUsage is one monitored filesystem path.
type DiskUsage struct {
	Path        string  `json:"path"`
	UsedPercent float64 `json:"used_percent"`
	UsedBytes   uint64  `json:"used_bytes"`
	TotalBytes  uint64  `json:"total_bytes"`
}

// SystemSnapshot is a point-in-time host resource sample. Available is false
// until the worker has taken its first successful sample.
type SystemSnapshot struct {
	CollectedAt   time.Time   `json:"collected_at"`
	CPUPercent    float64     `json:"cpu_percent"`
	MemPercent    float64     `json:"mem_percent"`
	MemUsedBytes  uint64      `json:"mem_used_bytes"`
	MemTotalBytes uint64      `json:"mem_total_bytes"`
	Disks         []DiskUsage `json:"disks"`
	Available     bool        `json:"available"`
}

// MonitorSettings configures thresholds and alert delivery. A threshold of 0
// disables that resource's check. Percent thresholds are 0..100.
type MonitorSettings struct {
	Enabled         bool     `json:"enabled"`
	CPUThreshold    int      `json:"cpu_threshold"`
	MemThreshold    int      `json:"mem_threshold"`
	DiskThreshold   int      `json:"disk_threshold"`
	DiskPaths       []string `json:"disk_paths"`
	NotifyEmails    []string `json:"notify_emails"`
	FromEmail       string   `json:"from_email"`
	SMTPHost        string   `json:"smtp_host"`
	CooldownMinutes int      `json:"cooldown_minutes"`
	SampleSeconds   int      `json:"sample_seconds"`
}

// MonitorAlert is a recorded threshold transition (breach or recovery).
type MonitorAlert struct {
	ID        string
	Resource  string  // cpu | memory | disk
	Detail    string  // disk path (empty for cpu/memory)
	Level     string  // breached | recovered
	Value     float64 // measured percent at the time
	Threshold int
	Message   string
	CreatedAt time.Time
	Notified  bool
}

// Key uniquely identifies the monitored thing an alert is about, so the worker
// can track per-resource state (a disk path is distinct per mount).
func (a MonitorAlert) Key() string {
	if a.Detail != "" {
		return a.Resource + ":" + a.Detail
	}
	return a.Resource
}

// Cooldown is the minimum gap between repeat breach notifications.
func (s MonitorSettings) Cooldown() time.Duration {
	return time.Duration(s.CooldownMinutes) * time.Minute
}

// SampleInterval is how often the worker samples the host.
func (s MonitorSettings) SampleInterval() time.Duration {
	return time.Duration(s.SampleSeconds) * time.Second
}

// NormalizeMonitorSettings clamps ranges and applies defaults in place, then
// validates. Returns an error only for operator-facing invalidity.
func NormalizeMonitorSettings(s *MonitorSettings) error {
	s.CPUThreshold = clampPercent(s.CPUThreshold)
	s.MemThreshold = clampPercent(s.MemThreshold)
	s.DiskThreshold = clampPercent(s.DiskThreshold)

	s.DiskPaths = cleanList(s.DiskPaths)
	if len(s.DiskPaths) == 0 {
		s.DiskPaths = []string{defaultDiskPath}
	}
	s.NotifyEmails = cleanList(s.NotifyEmails)
	s.FromEmail = strings.TrimSpace(s.FromEmail)
	s.SMTPHost = strings.TrimSpace(s.SMTPHost)
	if s.SMTPHost == "" {
		s.SMTPHost = defaultSMTPHost
	}
	if s.CooldownMinutes <= 0 {
		s.CooldownMinutes = defaultCooldown
	}
	if s.CooldownMinutes > maxCooldownMinute {
		s.CooldownMinutes = maxCooldownMinute
	}
	if s.SampleSeconds < minSampleSeconds {
		s.SampleSeconds = defaultSample
	}
	if s.SampleSeconds > maxSampleSeconds {
		s.SampleSeconds = maxSampleSeconds
	}

	if s.Enabled {
		if len(s.NotifyEmails) == 0 {
			return Invalid("MONITOR_NOTIFY_REQUIRED", "at least one notification email is required when monitoring is enabled")
		}
		for _, e := range s.NotifyEmails {
			if !strings.Contains(e, "@") {
				return Invalid("MONITOR_NOTIFY_INVALID", "notification email %q is not valid", e)
			}
		}
		if s.FromEmail == "" {
			return Invalid("MONITOR_FROM_REQUIRED", "a From address is required when monitoring is enabled")
		}
		if !strings.Contains(s.FromEmail, "@") {
			return Invalid("MONITOR_FROM_INVALID", "From address %q is not valid", s.FromEmail)
		}
	}
	return nil
}

func clampPercent(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// EvaluateBreaches returns an alert (level=breached) for every resource whose
// measured usage is at or above its enabled threshold. Pure: the worker maps
// these to notifications, applying cooldown and recovery state.
func EvaluateBreaches(s SystemSnapshot, cfg MonitorSettings) []MonitorAlert {
	if !s.Available {
		return nil
	}
	var out []MonitorAlert
	add := func(resource, detail string, value float64, threshold int) {
		out = append(out, MonitorAlert{
			Resource:  resource,
			Detail:    detail,
			Level:     AlertBreached,
			Value:     value,
			Threshold: threshold,
			Message:   breachMessage(resource, detail, value, threshold),
		})
	}
	if cfg.CPUThreshold > 0 && s.CPUPercent >= float64(cfg.CPUThreshold) {
		add(ResourceCPU, "", s.CPUPercent, cfg.CPUThreshold)
	}
	if cfg.MemThreshold > 0 && s.MemPercent >= float64(cfg.MemThreshold) {
		add(ResourceMem, "", s.MemPercent, cfg.MemThreshold)
	}
	if cfg.DiskThreshold > 0 {
		for _, d := range s.Disks {
			if d.UsedPercent >= float64(cfg.DiskThreshold) {
				add(ResourceDisk, d.Path, d.UsedPercent, cfg.DiskThreshold)
			}
		}
	}
	return out
}

func breachMessage(resource, detail string, value float64, threshold int) string {
	label := resourceLabel(resource, detail)
	return fmt.Sprintf("%s at %.1f%% (threshold %d%%)", label, value, threshold)
}

// RecoveryMessage renders the human text for a resolved alert.
func RecoveryMessage(resource, detail string, value float64, threshold int) string {
	return fmt.Sprintf("%s recovered to %.1f%% (threshold %d%%)", resourceLabel(resource, detail), value, threshold)
}

func resourceLabel(resource, detail string) string {
	switch resource {
	case ResourceCPU:
		return "CPU"
	case ResourceMem:
		return "Memory"
	case ResourceDisk:
		return "Disk " + detail
	default:
		return resource
	}
}
