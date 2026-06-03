package geoip

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

// DefaultURLTemplate is the DB-IP free "IP to Country Lite" download. The
// single %s is the YYYY-MM month; DB-IP publishes a fresh file each month.
// The download is gzip-compressed. Override via IRIS_GEOIP_DOWNLOAD_URL for
// a mirror or the paid DB-IP/MaxMind editions (any URL whose %s is the
// month and whose body is a gzipped .mmdb).
const DefaultURLTemplate = "https://download.db-ip.com/free/dbip-country-lite-%s.mmdb.gz"

// maxDecompressedBytes bounds the gunzip output as a decompression-bomb
// guard. The real database is ~tens of MB; 512 MiB is far above that and
// far below anything that would exhaust disk on a sane host.
const maxDecompressedBytes = 512 << 20

// EnsureCurrent downloads the current month's database to path when the
// existing file is missing or not from the current month, writing it
// atomically. It returns true when a new file was written. Network/HTTP
// failures return an error and leave any existing file untouched, so callers
// can fail open on the previous (or no) database.
func EnsureCurrent(ctx context.Context, path, urlTemplate string, client *http.Client, now time.Time) (bool, error) {
	if path == "" {
		return false, nil
	}
	if fileIsCurrent(path, now) {
		return false, nil
	}
	month := now.UTC().Format("2006-01")
	url := fmt.Sprintf(urlTemplate, month)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("geoip: download %s: HTTP %d", url, resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return false, fmt.Errorf("geoip: gunzip %s: %w", url, err)
	}
	defer gz.Close()

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".dbip-*.mmdb.tmp")
	if err != nil {
		return false, fmt.Errorf("geoip: temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Removed on any early return; a no-op after a successful rename.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmp, io.LimitReader(gz, maxDecompressedBytes)); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("geoip: write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("geoip: close %s: %w", tmpName, err)
	}

	// Validate it's a usable mmdb before swapping it into place — a
	// truncated download or an HTML error page must never replace a good DB.
	if db, err := maxminddb.Open(tmpName); err == nil {
		_ = db.Close()
	} else {
		return false, fmt.Errorf("geoip: downloaded file is not a valid mmdb: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return false, fmt.Errorf("geoip: install %s: %w", path, err)
	}
	return true, nil
}

// fileIsCurrent reports whether path exists and was last modified in the
// current (now) month — our proxy for "already holds this month's data",
// since a fresh download stamps the file with the current mtime.
func fileIsCurrent(path string, now time.Time) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	mt := fi.ModTime().UTC()
	n := now.UTC()
	return mt.Year() == n.Year() && mt.Month() == n.Month()
}
