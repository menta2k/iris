package biz

import "testing"

func TestNormalizeSuppressionFilterSortWhitelist(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty defaults to value", "", "value"},
		{"known column kept", "created_at", "created_at"},
		{"uppercase normalized", "CreatedAt", "value"}, // not a key; "createdat" != "created_at"
		{"valid uppercase key", "STATUS", "status"},
		{"unknown rejected", "id", "value"},
		{"injection rejected", "value; drop table", "value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeSuppressionFilter(SuppressionFilter{Sort: c.in})
			if got.Sort != c.want {
				t.Fatalf("sort %q → %q, want %q", c.in, got.Sort, c.want)
			}
		})
	}
}

func TestNormalizeSuppressionFilterSortDescPreserved(t *testing.T) {
	f := NormalizeSuppressionFilter(SuppressionFilter{Sort: "status", Desc: true})
	if f.Sort != "status" || !f.Desc {
		t.Fatalf("expected status/desc preserved, got %q/%v", f.Sort, f.Desc)
	}
}
