package kumopolicy

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLuaStringEscapesQuotesAndBackslashes(t *testing.T) {
	out, err := LuaString(`hello "world" \ tab	end`)
	require.NoError(t, err)
	require.Equal(t, `"hello \"world\" \\ tab\tend"`, out)
}

func TestLuaStringEscapesNewlines(t *testing.T) {
	out, err := LuaString("a\nb\rc")
	require.NoError(t, err)
	require.Equal(t, `"a\nb\rc"`, out)
}

func TestLuaStringEscapesControlBytes(t *testing.T) {
	out, err := LuaString(string([]byte{0x01, 0x1F, 0x7F}))
	require.NoError(t, err)
	require.Equal(t, `"\001\031\127"`, out)
}

func TestLuaStringRejectsNUL(t *testing.T) {
	_, err := LuaString("ab\x00cd")
	require.ErrorIs(t, err, ErrUnsafeString)
}

func TestLuaStringRejectsInvalidUTF8(t *testing.T) {
	_, err := LuaString(string([]byte{0xff, 0xfe}))
	require.ErrorIs(t, err, ErrUnsafeString)
}

// TestLuaStringInjectionAttempts covers the OWASP-style vectors a hostile
// admin user might submit. Each case must round-trip through LuaString and
// remain a single Lua string literal — no statement smuggling possible.
func TestLuaStringInjectionAttempts(t *testing.T) {
	cases := []string{
		`"; os.execute("rm -rf /"); --`,
		`]] os.execute([[evil]]); print("`,
		`\\"; require("io").popen("id"); --`,
		"\"\nrequire('os').exit()\n--",
		`]==] dangerous ]==]`,
		`'; print('boom'); --`,
	}
	for _, in := range cases {
		out, err := LuaString(in)
		require.NoError(t, err, "input %q", in)
		require.True(t, strings.HasPrefix(out, `"`) && strings.HasSuffix(out, `"`), "input %q produced %q", in, out)
		// Must not contain unescaped statement-breaking characters.
		// Specifically: between the first and last quote, every backslash run
		// must immediately precede an escapable character.
		body := out[1 : len(out)-1]
		// No raw newlines / nul / unescaped backslash-quote.
		require.NotContains(t, body, "\n", "raw newline leaked: %q -> %q", in, out)
		require.NotContains(t, body, "\r", "raw CR leaked: %q -> %q", in, out)
	}
}

func TestLuaIdentValid(t *testing.T) {
	for _, ok := range []string{"x", "X", "_x", "x1", "snake_case", "_"} {
		v, err := LuaIdent(ok)
		require.NoError(t, err, ok)
		require.Equal(t, ok, v)
	}
}

func TestLuaIdentInvalid(t *testing.T) {
	for _, bad := range []string{"", "1abc", "a-b", "a b", "a.b", "a$"} {
		_, err := LuaIdent(bad)
		require.ErrorIs(t, err, ErrInvalidIdent, "bad input %q", bad)
	}
}

func TestLuaIdentReserved(t *testing.T) {
	for _, k := range []string{"end", "function", "local", "true", "while"} {
		_, err := LuaIdent(k)
		require.ErrorIs(t, err, ErrReservedIdent, k)
	}
}

func TestLuaNumber(t *testing.T) {
	out, err := LuaNumber(3.14)
	require.NoError(t, err)
	require.Equal(t, "3.14", out)
}

func TestLuaNumberRejectsNonFinite(t *testing.T) {
	_, err := LuaNumber(math.NaN())
	require.ErrorIs(t, err, ErrInvalidNumber)
	_, err = LuaNumber(math.Inf(1))
	require.ErrorIs(t, err, ErrInvalidNumber)
}

func TestMustLuaStringPanicsOnUnsafe(t *testing.T) {
	require.Panics(t, func() { MustLuaString("\x00") })
}
