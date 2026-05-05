package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/transport"
	"github.com/stretchr/testify/require"

	appjwt "github.com/menta2k/iris/backend/pkg/jwt"
)

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
func (h fakeHeaders) Keys() []string                { return nil }
func (h fakeHeaders) Values(key string) []string {
	if v, ok := h[strings.ToLower(key)]; ok {
		return []string{v}
	}
	return nil
}

type fakeVerifier struct{ ok bool }

func (f *fakeVerifier) VerifyAccess(tok string) (*appjwt.Claims, error) {
	if !f.ok {
		return nil, errors.New("nope")
	}
	c := &appjwt.Claims{}
	c.UserID = 7
	c.Username = "alice"
	c.Roles = []string{"admin"}
	return c, nil
}

func TestServerExtractsBearer(t *testing.T) {
	mw := Server(&fakeVerifier{ok: true})
	tr := &fakeTransport{op: "/svc/Get", headers: fakeHeaders{"authorization": "Bearer abc"}}
	ctx := transport.NewServerContext(context.Background(), tr)
	var seen Identity
	_, err := mw(func(ctx context.Context, req any) (any, error) {
		seen, _ = FromContext(ctx)
		return nil, nil
	})(ctx, struct{}{})
	require.NoError(t, err)
	require.Equal(t, uint32(7), seen.UserID)
	require.Equal(t, "alice", seen.Username)
}

func TestServerRejectsMissingBearer(t *testing.T) {
	mw := Server(&fakeVerifier{ok: true})
	tr := &fakeTransport{op: "/svc/Get", headers: fakeHeaders{}}
	ctx := transport.NewServerContext(context.Background(), tr)
	_, err := mw(func(ctx context.Context, req any) (any, error) { return nil, nil })(ctx, nil)
	require.ErrorIs(t, err, ErrMissingBearer)
}

func TestServerRejectsInvalidToken(t *testing.T) {
	mw := Server(&fakeVerifier{ok: false})
	tr := &fakeTransport{op: "/svc/Get", headers: fakeHeaders{"authorization": "Bearer xyz"}}
	ctx := transport.NewServerContext(context.Background(), tr)
	_, err := mw(func(ctx context.Context, req any) (any, error) { return nil, nil })(ctx, nil)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestBearerFromHeader(t *testing.T) {
	require.Equal(t, "abc", bearerFromHeader("Bearer abc"))
	require.Equal(t, "abc", bearerFromHeader("bearer abc"))
	require.Equal(t, "abc", bearerFromHeader("Bearer  abc "))
	require.Equal(t, "", bearerFromHeader("Basic abc"))
	require.Equal(t, "", bearerFromHeader(""))
	require.Equal(t, "", bearerFromHeader("Bear"))
}

func TestIdentityFunc(t *testing.T) {
	uid, name := IdentityFunc(context.Background())
	require.Equal(t, uint32(0), uid)
	require.Equal(t, "", name)

	ctx := WithIdentity(context.Background(), Identity{UserID: 1, Username: "bob"})
	uid, name = IdentityFunc(ctx)
	require.Equal(t, uint32(1), uid)
	require.Equal(t, "bob", name)
}
