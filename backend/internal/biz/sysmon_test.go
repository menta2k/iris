package biz

import "testing"

func TestNormalizeMonitorSettingsDefaults(t *testing.T) {
	s := &MonitorSettings{}
	if err := NormalizeMonitorSettings(s); err != nil {
		t.Fatalf("disabled settings should validate: %v", err)
	}
	if len(s.DiskPaths) != 2 || s.DiskPaths[0] != "/" || s.DiskPaths[1] != KumoSpoolPath {
		t.Errorf("disk paths default = %v, want [/ %s]", s.DiskPaths, KumoSpoolPath)
	}
	if s.SMTPHost != "localhost:25" || s.CooldownMinutes != 30 || s.SampleSeconds != 30 {
		t.Errorf("defaults wrong: %+v", s)
	}
}

func TestNormalizeMonitorSettingsRequiresNotify(t *testing.T) {
	s := &MonitorSettings{Enabled: true}
	if err := NormalizeMonitorSettings(s); err == nil {
		t.Fatal("enabled without notify email should fail")
	}
	s = &MonitorSettings{Enabled: true, NotifyEmails: []string{"ops@example.com"}, FromEmail: "iris@example.com"}
	if err := NormalizeMonitorSettings(s); err != nil {
		t.Fatalf("enabled with valid email should pass: %v", err)
	}
}

func TestNormalizeClampsPercent(t *testing.T) {
	s := &MonitorSettings{CPUThreshold: 250, MemThreshold: -5, DiskThreshold: 80}
	_ = NormalizeMonitorSettings(s)
	if s.CPUThreshold != 100 || s.MemThreshold != 0 || s.DiskThreshold != 80 {
		t.Errorf("clamp wrong: %+v", s)
	}
}

func TestEvaluateBreaches(t *testing.T) {
	snap := SystemSnapshot{
		Available:  true,
		CPUPercent: 95, MemPercent: 40,
		Disks: []DiskUsage{{Path: "/", UsedPercent: 92}, {Path: "/data", UsedPercent: 10}},
	}
	cfg := MonitorSettings{CPUThreshold: 90, MemThreshold: 90, DiskThreshold: 85}
	got := EvaluateBreaches(snap, cfg)
	if len(got) != 2 {
		t.Fatalf("want 2 breaches (cpu + disk /), got %d: %+v", len(got), got)
	}
	keys := map[string]bool{}
	for _, a := range got {
		keys[a.Key()] = true
		if a.Level != AlertBreached {
			t.Errorf("level = %q", a.Level)
		}
	}
	if !keys["cpu"] || !keys["disk:/"] {
		t.Errorf("missing expected breaches: %v", keys)
	}
}

func TestEvaluateBreachesDisabledThreshold(t *testing.T) {
	snap := SystemSnapshot{Available: true, CPUPercent: 99, Disks: []DiskUsage{{Path: "/", UsedPercent: 99}}}
	// 0 thresholds disable each check.
	if got := EvaluateBreaches(snap, MonitorSettings{}); len(got) != 0 {
		t.Fatalf("zero thresholds should yield no breaches, got %+v", got)
	}
}

func TestEvaluateBreachesUnavailable(t *testing.T) {
	if got := EvaluateBreaches(SystemSnapshot{Available: false, CPUPercent: 100}, MonitorSettings{CPUThreshold: 50}); got != nil {
		t.Fatalf("no breaches before first sample, got %+v", got)
	}
}
