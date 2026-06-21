package biz

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// Pagination bounds for list queries. Bounded queries protect the database from
// unbounded scans and align with the performance constraints in plan.md.
const (
	DefaultPageSize = 50
	MaxPageSize     = 500
)

// Page is a validated, bounded pagination request resolved to an offset.
type Page struct {
	Size   int
	Offset int
}

// NormalizePage validates and bounds a page size, decoding an opaque offset
// token. An invalid token resolves to offset 0 rather than erroring, so callers
// never expose token internals.
func NormalizePage(size int, token string) Page {
	if size <= 0 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	return Page{Size: size, Offset: decodeOffset(token)}
}

// NextToken returns an opaque token for the next page, or "" if this page was
// not full (no further results).
func (p Page) NextToken(returned int) string {
	if returned < p.Size {
		return ""
	}
	return encodeOffset(p.Offset + p.Size)
}

func encodeOffset(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("o:" + strconv.Itoa(offset)))
}

func decodeOffset(token string) int {
	if token == "" {
		return 0
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0
	}
	s := string(raw)
	if !strings.HasPrefix(s, "o:") {
		return 0
	}
	offset, err := strconv.Atoi(strings.TrimPrefix(s, "o:"))
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

// SanitizeFilter trims a free-text filter value and bounds its length to guard
// against abusive inputs. It does not perform SQL escaping; callers must always
// use parameterized queries.
func SanitizeFilter(v string) string {
	v = strings.TrimSpace(v)
	const maxFilterLen = 320 // RFC 5321 max email path length
	if len(v) > maxFilterLen {
		return v[:maxFilterLen]
	}
	return v
}
