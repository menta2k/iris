package biz

import (
	"strings"
	"testing"
)

func TestResolveTemplateSubstitutesWindow(t *testing.T) {
	def, ok := lookupWidget("kumo_messages_received_rate")
	if !ok {
		t.Fatal("expected catalog widget to exist")
	}
	q := def.resolveTemplate("5m", "")
	if !strings.Contains(q, "[5m]") {
		t.Fatalf("expected window substituted, got %q", q)
	}
	if strings.Contains(q, "$window") || strings.Contains(q, "$groupBy") {
		t.Fatalf("unresolved placeholder in %q", q)
	}
}

func TestResolveTemplateGroupByAllowed(t *testing.T) {
	def, ok := lookupWidget("kumo_messages_delivered_rate")
	if !ok {
		t.Fatal("expected catalog widget to exist")
	}
	q := def.resolveTemplate("5m", "provider")
	if !strings.Contains(q, "by (provider)") {
		t.Fatalf("expected group-by clause, got %q", q)
	}
}

func TestResolveTemplateGroupByRejectsUnknownLabel(t *testing.T) {
	def, ok := lookupWidget("kumo_messages_delivered_rate")
	if !ok {
		t.Fatal("expected catalog widget to exist")
	}
	// A label not in the allow-list must be dropped (no injection).
	q := def.resolveTemplate("5m", "instance) or vector(1) (")
	if strings.Contains(q, "instance") || strings.Contains(q, "vector(1)") {
		t.Fatalf("injection leaked into query: %q", q)
	}
	if strings.Contains(q, "by (") {
		t.Fatalf("expected no group-by for disallowed label, got %q", q)
	}
}

func TestResolveTemplateGroupByIgnoredWhenUnsupported(t *testing.T) {
	def, ok := lookupWidget("kumo_messages_received_rate") // SupportsGroupBy=false
	if !ok {
		t.Fatal("expected catalog widget to exist")
	}
	q := def.resolveTemplate("5m", "provider")
	if strings.Contains(q, "by (") {
		t.Fatalf("expected no group-by on unsupported widget, got %q", q)
	}
}

func TestWidgetCatalogReturnsCopy(t *testing.T) {
	a := WidgetCatalog()
	if len(a) == 0 {
		t.Fatal("expected a non-empty catalog")
	}
	a[0].Title = "mutated"
	b := WidgetCatalog()
	if b[0].Title == "mutated" {
		t.Fatal("WidgetCatalog must return a copy, not the shared slice")
	}
}

func TestLookupWidgetUnknown(t *testing.T) {
	if _, ok := lookupWidget("does_not_exist"); ok {
		t.Fatal("expected unknown key to miss")
	}
}
