package acmedns

import "net/url"

// mustParseURL parses an operator-supplied URL string. Returns nil on empty
// input (callers gate on the field being required); non-empty invalid URLs
// propagate as nil and the lego provider's New*Config surfaces the resulting
// "missing endpoint" error.
func mustParseURL(s string) *url.URL {
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil
	}
	return u
}
