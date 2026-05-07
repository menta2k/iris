package data

import (
	"strings"
	"testing"
)

func TestShouldOverride(t *testing.T) {
	cases := []struct {
		existing, incoming string
		want               bool
	}{
		// Same reason → refresh.
		{"hard_bounce", "hard_bounce", true},
		// Severity ladder.
		{"soft_bounce", "hard_bounce", true},
		{"hard_bounce", "expired", true},
		{"soft_bounce", "expired", true},
		// Higher reason wins; lower never overrides.
		{"hard_bounce", "soft_bounce", false},
		{"expired", "hard_bounce", false},
		// Operator-set reasons are sticky.
		{"manual", "hard_bounce", false},
		{"complaint", "soft_bounce", false},
		{"complaint", "expired", false},
	}
	for _, c := range cases {
		if got := shouldOverride(c.existing, c.incoming); got != c.want {
			t.Errorf("shouldOverride(%q, %q) = %v want %v",
				c.existing, c.incoming, got, c.want)
		}
	}
}

func TestBuildSuppressionNote(t *testing.T) {
	got := buildSuppressionNote("smtp; 550 unknown user", "5.1.1", "unknown_user")
	if !strings.Contains(got, "status=5.1.1") {
		t.Errorf("missing status: %q", got)
	}
	if !strings.Contains(got, "category=unknown_user") {
		t.Errorf("missing category: %q", got)
	}
	if !strings.Contains(got, "diag=smtp; 550 unknown user") {
		t.Errorf("missing diag: %q", got)
	}
	// Empty fields are skipped, no double pipes.
	got2 := buildSuppressionNote("", "5.1.1", "")
	if strings.Contains(got2, "||") {
		t.Errorf("doubled separator: %q", got2)
	}
	if !strings.HasPrefix(got2, "status=") {
		t.Errorf("expected status leading, got %q", got2)
	}
}

func TestParseAutoSuppressFlag(t *testing.T) {
	cases := map[string]bool{
		"":      true, // default-on
		"true":  true,
		"yes":   true,
		"1":     true,
		"on":    true,
		"FALSE": false,
		"0":     false,
		"no":    false,
		"off":   false,
	}
	for in, want := range cases {
		t.Setenv("IRIS_BOUNCE_AUTO_SUPPRESS", in)
		got := parseAutoSuppressFlag()
		if got != want {
			t.Errorf("parseAutoSuppressFlag(%q) = %v want %v", in, got, want)
		}
	}
}

func TestParseDomainList(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		// Lowercases, trims whitespace, strips trailing dots, de-dupes.
		{"Test-1.com, test2.com", []string{"test-1.com", "test2.com"}},
		{"  test-1.com  ,, ,Test-1.COM, test2.com.  ", []string{"test-1.com", "test2.com"}},
		{",,,", nil},
		// Order-preserving (env author's order is operator intent).
		{"b.com,a.com", []string{"b.com", "a.com"}},
	}
	for _, c := range cases {
		got := parseDomainList(c.in)
		if len(got) != len(c.want) {
			t.Errorf("parseDomainList(%q) = %v want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseDomainList(%q)[%d] = %q want %q",
					c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestParseIntEnv(t *testing.T) {
	t.Setenv("IRIS_TEST_INT", "")
	if got := parseIntEnv("IRIS_TEST_INT", 7); got != 7 {
		t.Errorf("default fallback: %d", got)
	}
	t.Setenv("IRIS_TEST_INT", "42")
	if got := parseIntEnv("IRIS_TEST_INT", 7); got != 42 {
		t.Errorf("parse: %d", got)
	}
	t.Setenv("IRIS_TEST_INT", "junk")
	if got := parseIntEnv("IRIS_TEST_INT", 7); got != 7 {
		t.Errorf("malformed fallback: %d", got)
	}
	t.Setenv("IRIS_TEST_INT", "-1")
	if got := parseIntEnv("IRIS_TEST_INT", 7); got != 7 {
		t.Errorf("negative fallback: %d", got)
	}
}
