package biz

import "testing"

func TestValidateAcmeDomain(t *testing.T) {
	valid := []string{
		"example.com",
		"mail.example.com",
		"*.example.com",
		"*.kmx.jobs.bg",
		"a.b.c.d.example.com",
	}
	for _, d := range valid {
		if err := validateAcmeDomain(d); err != nil {
			t.Errorf("expected %q valid, got %v", d, err)
		}
	}

	invalid := []string{
		"",
		"*",
		"*.",
		"*.*.example.com", // only a single leading wildcard label is allowed
		"exa mple.com",
		"-bad.example.com",
		"example..com",
	}
	for _, d := range invalid {
		if err := validateAcmeDomain(d); err == nil {
			t.Errorf("expected %q invalid, got nil error", d)
		}
	}
}

func TestIsWildcardDomain(t *testing.T) {
	if !isWildcardDomain("*.example.com") {
		t.Fatal("*.example.com should be a wildcard")
	}
	if isWildcardDomain("example.com") {
		t.Fatal("example.com is not a wildcard")
	}
}
