package geoip

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileIsCurrent(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "db.mmdb")

	if fileIsCurrent(path, now) {
		t.Fatal("missing file must not be current")
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Just-written file (this month) is current.
	if !fileIsCurrent(path, now) {
		t.Fatal("file written this month should be current")
	}
	// Backdate to last month → stale.
	lastMonth := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, lastMonth, lastMonth); err != nil {
		t.Fatal(err)
	}
	if fileIsCurrent(path, now) {
		t.Fatal("file from last month must be stale")
	}
}

func TestEnsureCurrentSkipsWhenFresh(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "db.mmdb")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	downloaded, err := EnsureCurrent(context.Background(), path, srv.URL+"/db-%s.mmdb.gz", srv.Client(), now)
	if err != nil || downloaded {
		t.Fatalf("expected skip, got downloaded=%v err=%v", downloaded, err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatal("a fresh file must not trigger a network fetch")
	}
}

func TestEnsureCurrentHTTPError(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "db.mmdb") // missing => stale => fetch

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	downloaded, err := EnsureCurrent(context.Background(), path, srv.URL+"/db-%s.mmdb.gz", srv.Client(), now)
	if err == nil || downloaded {
		t.Fatalf("expected HTTP error, got downloaded=%v err=%v", downloaded, err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("no file must be written on HTTP error")
	}
}

func TestEnsureCurrentRejectsInvalidMmdb(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "db.mmdb")

	// Serve a valid gzip stream whose payload is NOT a valid mmdb.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte("this is not an mmdb database"))
	_ = gz.Close()
	body := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	downloaded, err := EnsureCurrent(context.Background(), path, srv.URL+"/db-%s.mmdb.gz", srv.Client(), now)
	if err == nil || downloaded {
		t.Fatalf("expected invalid-mmdb rejection, got downloaded=%v err=%v", downloaded, err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("an invalid download must not replace/create the target file")
	}
	// The temp file must have been cleaned up.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}
