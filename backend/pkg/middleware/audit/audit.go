// Package audit is a Kratos middleware that records every mutating
// admin-API call into an append-only audit log.
//
// What it captures:
//   - Operation name (e.g., /identity.service.v1.UserService/Create)
//   - Caller identity (user_id, username) extracted from the auth context
//   - Client IP, user-agent, request ID
//   - Status code + message
//   - Round-trip duration
//   - Redacted request and response payloads
//
// What it deliberately does NOT do:
//   - Decide *whether* an operation is mutating — that is determined by the
//     caller-supplied IsMutating predicate. By convention, gRPC method names
//     beginning with Create/Update/Delete/Apply/Suspend/Resume/Bounce/
//     Rotate/Import/Reorder/Login/Logout/RefreshToken/ChangePassword
//     count as mutating.
//   - Block on disk: writes are off the request path via WriteFunc, which
//     the application is expected to back with a buffered async writer.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
)

// Entry is the audit record handed to WriteFunc.
type Entry struct {
	At            time.Time
	Operation     string
	ResourceType  string
	ResourceID    string
	ActorUserID   uint32
	ActorUsername string
	ClientIP      string
	UserAgent     string
	RequestID     string
	StatusCode    int32
	StatusMessage string
	RequestJSON   string
	ResponseJSON  string
	DurationMS    int64
}

// WriteFunc writes one Entry. Implementations should not block the request
// path — buffer or hand off to a worker.
type WriteFunc func(ctx context.Context, e *Entry) error

// IdentityFunc extracts the caller identity from the request context. Return
// (0, "") for unauthenticated calls (e.g., the Login endpoint).
type IdentityFunc func(ctx context.Context) (uid uint32, username string)

// MutatingFunc decides whether to write an audit row for an operation. The
// default predicate uses gRPC method-name conventions.
type MutatingFunc func(operation string) bool

// ResourceFunc extracts (resource_type, resource_id) from the typed request.
// Returning empty strings is fine — the row will record only the operation.
type ResourceFunc func(req any) (resourceType, resourceID string)

// RedactFunc returns the JSON-encoded payload with sensitive fields removed.
// The default redactor JSON-encodes the value as-is, replacing string fields
// named "password", "old_password", "new_password", "secret", "token",
// "access_token", "refresh_token", "key" with "[REDACTED]".
type RedactFunc func(v any) (string, error)

// Options control middleware behavior.
type Options struct {
	Write      WriteFunc
	Identity   IdentityFunc
	IsMutating MutatingFunc
	Resource   ResourceFunc
	Redact     RedactFunc
	// MaxJSONBytes caps redacted JSON length per side; defaults to 16 KiB.
	MaxJSONBytes int
}

// Server returns a kratos middleware for auditing.
func Server(opts Options) middleware.Middleware {
	if opts.IsMutating == nil {
		opts.IsMutating = DefaultMutatingPredicate
	}
	if opts.Redact == nil {
		opts.Redact = DefaultRedactor
	}
	if opts.MaxJSONBytes <= 0 {
		opts.MaxJSONBytes = 16 * 1024
	}
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return handler(ctx, req)
			}
			op := tr.Operation()
			if !opts.IsMutating(op) {
				return handler(ctx, req)
			}
			start := time.Now()
			resp, err := handler(ctx, req)
			dur := time.Since(start)
			if opts.Write == nil {
				return resp, err
			}

			entry := &Entry{
				At:        start.UTC(),
				Operation: op,
				DurationMS: dur.Milliseconds(),
			}
			if opts.Identity != nil {
				entry.ActorUserID, entry.ActorUsername = opts.Identity(ctx)
			}
			if opts.Resource != nil {
				entry.ResourceType, entry.ResourceID = opts.Resource(req)
			}
			entry.ClientIP = clientIP(tr)
			entry.UserAgent = headerValue(tr, "user-agent", "User-Agent")
			entry.RequestID = headerValue(tr, "x-request-id", "X-Request-Id", "request-id")

			if err != nil {
				entry.StatusCode = 13 // INTERNAL by default
				entry.StatusMessage = err.Error()
				if c, ok := errCoder(err); ok {
					entry.StatusCode = c
				}
			}
			if reqJSON, jErr := opts.Redact(req); jErr == nil {
				entry.RequestJSON = clip(reqJSON, opts.MaxJSONBytes)
			}
			if resp != nil {
				if rJSON, jErr := opts.Redact(resp); jErr == nil {
					entry.ResponseJSON = clip(rJSON, opts.MaxJSONBytes)
				}
			}
			// Best-effort write.
			_ = opts.Write(ctx, entry)
			return resp, err
		}
	}
}

// DefaultMutatingPredicate matches gRPC method names whose final element
// implies state mutation.
func DefaultMutatingPredicate(operation string) bool {
	if operation == "" {
		return false
	}
	last := operation
	if idx := strings.LastIndex(operation, "/"); idx >= 0 {
		last = operation[idx+1:]
	}
	switch last {
	case "Login", "Logout", "RefreshToken", "ChangePassword":
		return true
	}
	for _, p := range []string{"Create", "Update", "Delete", "Apply", "Suspend",
		"Resume", "Bounce", "Rotate", "Import", "Reorder", "Set", "Patch"} {
		if strings.HasPrefix(last, p) {
			return true
		}
	}
	return false
}

// DefaultRedactor JSON-encodes v and replaces sensitive string keys.
func DefaultRedactor(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	buf, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("audit: marshal: %w", err)
	}
	// Decode-modify-reencode for fields by name. We avoid a heavy reflect
	// walk; this is a hot path under high-traffic services.
	var generic any
	if err := json.Unmarshal(buf, &generic); err != nil {
		return string(buf), nil
	}
	redactWalk(generic)
	out, err := json.Marshal(generic)
	if err != nil {
		return string(buf), nil
	}
	return string(out), nil
}

var sensitiveKeys = map[string]struct{}{
	"password":      {},
	"old_password":  {},
	"new_password":  {},
	"secret":        {},
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"key":           {},
	"private_key":   {},
}

func redactWalk(v any) {
	switch m := v.(type) {
	case map[string]any:
		for k, val := range m {
			lk := strings.ToLower(k)
			if _, sensitive := sensitiveKeys[lk]; sensitive {
				if _, isString := val.(string); isString {
					m[k] = "[REDACTED]"
					continue
				}
			}
			redactWalk(val)
		}
	case []any:
		for _, item := range m {
			redactWalk(item)
		}
	}
}

// errCoder pulls a numeric error code if the error implements one. We avoid a
// hard dep on kratos errors here so this package can be reused by tests.
type coder interface{ Code() int32 }

func errCoder(err error) (int32, bool) {
	var c coder
	if errors.As(err, &c) {
		return c.Code(), true
	}
	return 0, false
}

func clientIP(tr transport.Transporter) string {
	if h := headerValue(tr, "x-forwarded-for"); h != "" {
		if i := strings.IndexByte(h, ','); i > 0 {
			return strings.TrimSpace(h[:i])
		}
		return strings.TrimSpace(h)
	}
	if h := headerValue(tr, "x-real-ip"); h != "" {
		return h
	}
	if endpoint := tr.Endpoint(); endpoint != "" {
		host, _, err := net.SplitHostPort(endpoint)
		if err == nil {
			return host
		}
	}
	return ""
}

func headerValue(tr transport.Transporter, names ...string) string {
	rh := tr.RequestHeader()
	if rh == nil {
		return ""
	}
	for _, n := range names {
		if v := rh.Get(n); v != "" {
			return v
		}
	}
	return ""
}

func clip(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}
