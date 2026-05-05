package audit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/transport"
	"github.com/stretchr/testify/require"
)

// fakeTransport implements transport.Transporter for unit tests.
type fakeTransport struct {
	op       string
	endpoint string
	headers  fakeHeaders
}

func (f *fakeTransport) Kind() transport.Kind            { return transport.KindGRPC }
func (f *fakeTransport) Endpoint() string                { return f.endpoint }
func (f *fakeTransport) Operation() string               { return f.op }
func (f *fakeTransport) RequestHeader() transport.Header { return f.headers }
func (f *fakeTransport) ReplyHeader() transport.Header   { return f.headers }

type fakeHeaders map[string]string

func (h fakeHeaders) Get(key string) string         { return h[strings.ToLower(key)] }
func (h fakeHeaders) Set(key, value string)         { h[strings.ToLower(key)] = value }
func (h fakeHeaders) Add(key, value string)         { h[strings.ToLower(key)] = value }
func (h fakeHeaders) Keys() []string {
	out := make([]string, 0, len(h))
	for k := range h {
		out = append(out, k)
	}
	return out
}
func (h fakeHeaders) Values(key string) []string {
	if v, ok := h[strings.ToLower(key)]; ok {
		return []string{v}
	}
	return nil
}

func TestDefaultMutatingPredicate(t *testing.T) {
	cases := map[string]bool{
		"/identity.service.v1.UserService/Create":    true,
		"/identity.service.v1.UserService/Update":    true,
		"/identity.service.v1.UserService/Delete":    true,
		"/identity.service.v1.UserService/Get":       false,
		"/identity.service.v1.UserService/List":      false,
		"/audit.service.v1.AuditService/List":        false,
		"/auth.v1.AuthenticationService/Login":       true,
		"/auth.v1.AuthenticationService/Logout":      true,
		"/kumo.v1.PolicyService/Apply":               true,
		"/kumo.v1.PolicyService/Render":              false,
		"/kumo.v1.PolicyService/Validate":            false,
		"/kumo.v1.QueueService/Suspend":              true,
		"/kumo.v1.QueueService/Bounce":               true,
		"/kumo.v1.SuppressionService/Import":         true,
		"/kumo.v1.RoutingService/Reorder":            true,
		"/identity.service.v1.UserService/ChangePassword": true,
	}
	for op, expected := range cases {
		require.Equal(t, expected, DefaultMutatingPredicate(op), op)
	}
}

func TestRedactorMasksSensitiveFields(t *testing.T) {
	type loginReq struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	out, err := DefaultRedactor(loginReq{Username: "alice", Password: "hunter2"})
	require.NoError(t, err)
	require.Contains(t, out, `"username":"alice"`)
	require.Contains(t, out, `"password":"[REDACTED]"`)
	require.NotContains(t, out, "hunter2")
}

func TestRedactorHandlesNested(t *testing.T) {
	v := map[string]any{
		"user": map[string]any{
			"name":     "alice",
			"password": "hunter2",
		},
		"tokens": []any{
			map[string]any{"access_token": "secret"},
		},
	}
	out, err := DefaultRedactor(v)
	require.NoError(t, err)
	require.NotContains(t, out, "hunter2")
	require.NotContains(t, out, "secret")
	require.Contains(t, out, "[REDACTED]")
}

func TestServerSkipsNonMutating(t *testing.T) {
	called := false
	mw := Server(Options{
		Write: func(ctx context.Context, e *Entry) error { called = true; return nil },
	})
	tr := &fakeTransport{op: "/svc/List", headers: fakeHeaders{}}
	ctx := transport.NewServerContext(context.Background(), tr)
	_, err := mw(func(ctx context.Context, req any) (any, error) { return "ok", nil })(ctx, "req")
	require.NoError(t, err)
	require.False(t, called, "audit must skip read-only operations")
}

func TestServerWritesEntryOnMutation(t *testing.T) {
	var entry *Entry
	mw := Server(Options{
		Write: func(ctx context.Context, e *Entry) error { entry = e; return nil },
		Identity: func(ctx context.Context) (uint32, string) {
			return 42, "alice"
		},
		Resource: func(req any) (string, string) { return "user", "1" },
	})
	tr := &fakeTransport{
		op:      "/svc/CreateUser",
		endpoint: "127.0.0.1:1234",
		headers: fakeHeaders{
			"x-forwarded-for": "203.0.113.4, 10.0.0.1",
			"user-agent":      "test",
			"x-request-id":    "req-1",
		},
	}
	ctx := transport.NewServerContext(context.Background(), tr)
	_, err := mw(func(ctx context.Context, req any) (any, error) {
		return map[string]any{"ok": true}, nil
	})(ctx, map[string]any{"username": "alice", "password": "p"})
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.Equal(t, "/svc/CreateUser", entry.Operation)
	require.Equal(t, uint32(42), entry.ActorUserID)
	require.Equal(t, "alice", entry.ActorUsername)
	require.Equal(t, "203.0.113.4", entry.ClientIP)
	require.Equal(t, "test", entry.UserAgent)
	require.Equal(t, "req-1", entry.RequestID)
	require.Equal(t, "user", entry.ResourceType)
	require.Equal(t, "1", entry.ResourceID)
	require.Contains(t, entry.RequestJSON, "[REDACTED]")
}

func TestServerCapturesError(t *testing.T) {
	var entry *Entry
	mw := Server(Options{
		Write: func(ctx context.Context, e *Entry) error { entry = e; return nil },
	})
	tr := &fakeTransport{op: "/svc/DeleteUser", headers: fakeHeaders{}}
	ctx := transport.NewServerContext(context.Background(), tr)
	_, _ = mw(func(ctx context.Context, req any) (any, error) {
		return nil, errors.New("boom")
	})(ctx, struct{}{})
	require.NotNil(t, entry)
	require.Contains(t, entry.StatusMessage, "boom")
}

func TestClipTruncates(t *testing.T) {
	require.Equal(t, "abc", clip("abc", 10))
	require.Equal(t, "ab...[truncated]", clip("abcdef", 2))
}
