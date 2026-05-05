package authorizer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseValid(t *testing.T) {
	p, err := Parse("kumo.policy:write")
	require.NoError(t, err)
	require.Equal(t, "kumo.policy", p.resource)
	require.Equal(t, "write", p.action)
	require.False(t, p.wild)
}

func TestParseWildcardResource(t *testing.T) {
	p, err := Parse("kumo.*:read")
	require.NoError(t, err)
	require.True(t, p.wild)
	require.Equal(t, "kumo.", p.prefix)
}

func TestParseGlobalWildcard(t *testing.T) {
	p, err := Parse("*:*")
	require.NoError(t, err)
	require.True(t, p.wild)
	require.Equal(t, "*", p.resource)
}

func TestParseInvalid(t *testing.T) {
	for _, bad := range []string{"", "noop", ":", "x:", ":y"} {
		_, err := Parse(bad)
		require.ErrorIs(t, err, ErrInvalidPermission, bad)
	}
}

func TestAllowExactMatch(t *testing.T) {
	a := New([]string{"audit.log:read", "user:write"})
	require.True(t, a.Allow("audit.log", "read"))
	require.True(t, a.Allow("user", "write"))
	require.False(t, a.Allow("audit.log", "write"))
	require.False(t, a.Allow("user", "delete"))
}

func TestAllowResourceWildcard(t *testing.T) {
	a := New([]string{"kumo.*:read"})
	require.True(t, a.Allow("kumo.policy", "read"))
	require.True(t, a.Allow("kumo.queue", "read"))
	require.False(t, a.Allow("audit.log", "read"))
	require.False(t, a.Allow("kumo.policy", "write"))
}

func TestAllowActionWildcard(t *testing.T) {
	a := New([]string{"kumo.policy:*"})
	require.True(t, a.Allow("kumo.policy", "read"))
	require.True(t, a.Allow("kumo.policy", "write"))
	require.False(t, a.Allow("kumo.queue", "write"))
}

func TestAllowGlobal(t *testing.T) {
	a := New([]string{"*:*"})
	require.True(t, a.Allow("anything", "everything"))
}

func TestNewIgnoresMalformed(t *testing.T) {
	a := New([]string{"good:read", "::bad", "alsobad"})
	require.True(t, a.Allow("good", "read"))
	require.False(t, a.Allow("alsobad", "read"))
}

func TestAllowCaseInsensitive(t *testing.T) {
	a := New([]string{"KUMO.Policy:WRITE"})
	require.True(t, a.Allow("kumo.policy", "write"))
}
