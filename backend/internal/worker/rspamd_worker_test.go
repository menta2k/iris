package worker

import (
	"context"
	"log/slog"
	"reflect"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

type fakeRspamdStore struct {
	got []*biz.RspamdFilterResult
}

func (f *fakeRspamdStore) IngestRspamdResult(_ context.Context, res *biz.RspamdFilterResult) error {
	f.got = append(f.got, res)
	return nil
}

// TestRspamdHandleMapsStreamFields verifies the worker maps the policy's XADD
// fields onto a filter result, decoding the JSON symbols array and the score.
func TestRspamdHandleMapsStreamFields(t *testing.T) {
	store := &fakeRspamdStore{}
	w := NewRspamdWorker(nil, store, slog.New(slog.NewTextHandler(discard{}, nil)))

	w.handle(context.Background(), data.StreamMessage{Values: map[string]any{
		"action":  "add header",
		"score":   "7.25",
		"symbols": `["BAYES_SPAM","MIME_HTML_ONLY"]`,
		"reason":  "spam detected",
	}})

	if len(store.got) != 1 {
		t.Fatalf("expected 1 ingested result, got %d", len(store.got))
	}
	r := store.got[0]
	if r.Action != "add header" || r.Score != 7.25 || r.Reason != "spam detected" {
		t.Fatalf("unexpected mapping: %+v", r)
	}
	if !reflect.DeepEqual(r.Symbols, []string{"BAYES_SPAM", "MIME_HTML_ONLY"}) {
		t.Fatalf("symbols not decoded: %#v", r.Symbols)
	}
}

// TestParseSymbols covers the JSON-array decode and its fail-soft cases.
func TestParseSymbols(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{"array", `["A","B"]`, []string{"A", "B"}},
		{"empty string", "", nil},
		{"blank", "   ", nil},
		{"missing", nil, nil},
		{"malformed", `["A"`, nil},
		{"empty array", `[]`, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseSymbols(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseSymbols(%v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
