package service

import "context"

// clientIPKey carries the request's client IP through context so the auth
// adapter can forward it to Login. The hand-rolled HTTP login handler (which
// has direct access to *http.Request, and extracts X-Forwarded-For behind
// the HTTPS reverse proxy) sets it; the gRPC path can set it from peer info
// in a future enhancement.
type clientIPKey struct{}

// WithClientIP returns a context carrying ip. A blank ip is a no-op so the
// firewall simply fails open on IP/REGION attributes downstream.
func WithClientIP(ctx context.Context, ip string) context.Context {
	if ip == "" {
		return ctx
	}
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// clientIPFromCtx reads the client IP set by WithClientIP, or "" if unset.
func clientIPFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(clientIPKey{}).(string); ok {
		return v
	}
	return ""
}
