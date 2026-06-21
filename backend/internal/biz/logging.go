package biz

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// sensitiveKeys are log/audit attribute keys whose values must never be
// rendered in clear text. Matching is case-insensitive and substring-based so
// that keys like "private_key_ref" or "session_token" are caught.
var sensitiveKeys = []string{
	"password", "secret", "token", "private_key", "privatekey",
	"authorization", "cookie", "mfa_code", "otp", "dkim_private",
	"raw_payload", "preview", "diagnostic_raw",
}

const redacted = "[REDACTED]"

// NewLogger builds a structured slog logger that redacts sensitive attributes.
func NewLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       lvl,
		ReplaceAttr: redactAttr,
	})
	return slog.New(handler)
}

// redactAttr replaces the value of any sensitive attribute with a placeholder.
func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if IsSensitiveKey(a.Key) {
		return slog.String(a.Key, redacted)
	}
	return a
}

// IsSensitiveKey reports whether a field key holds sensitive data and must be
// redacted before logging, auditing, or returning over the API.
func IsSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// RedactMap returns a copy of m with sensitive values replaced. It is used to
// build safe change summaries for audit entries.
func RedactMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if IsSensitiveKey(k) {
			out[k] = redacted
			continue
		}
		out[k] = v
	}
	return out
}

type loggerKey struct{}

// WithLogger stores a logger on the context for request-scoped logging.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// LoggerFrom returns the request logger, or the default logger if none is set.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
