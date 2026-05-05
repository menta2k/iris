// Package kumopolicy renders the kumomta Lua policy from a config snapshot.
//
// SECURITY MODEL
//
// This package is the single point at which database-backed configuration is
// transformed into executable Lua. Every user-controlled string MUST flow
// through one of the helpers here before reaching the rendered policy:
//
//   - LuaString  – escapes a string for embedding inside a Lua double-quoted
//                  literal. Rejects NUL bytes (Lua source files cannot embed
//                  NUL safely).
//   - LuaIdent   – validates an identifier against [A-Za-z_][A-Za-z0-9_]*,
//                  rejecting Lua reserved words. Use for table keys / variable
//                  names that a user can influence (mail class names, etc.).
//   - LuaNumber  – emits a finite numeric literal. Rejects NaN / Inf.
//
// String construction never relies on text/template autoescaping; the
// template only receives values that have already been hardened.
package kumopolicy

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	// ErrUnsafeString is returned for strings that cannot be safely embedded.
	ErrUnsafeString = errors.New("kumopolicy: string contains unsafe bytes")
	// ErrInvalidIdent is returned for identifiers that fail the allow-list.
	ErrInvalidIdent = errors.New("kumopolicy: invalid Lua identifier")
	// ErrReservedIdent is returned for identifiers that match Lua keywords.
	ErrReservedIdent = errors.New("kumopolicy: identifier is a Lua reserved word")
	// ErrInvalidNumber is returned for non-finite numbers.
	ErrInvalidNumber = errors.New("kumopolicy: number is not finite")
)

// luaReserved is the set of Lua 5.4 reserved words. We refuse to emit any of
// these as identifiers from user input.
var luaReserved = map[string]struct{}{
	"and": {}, "break": {}, "do": {}, "else": {}, "elseif": {}, "end": {},
	"false": {}, "for": {}, "function": {}, "goto": {}, "if": {}, "in": {},
	"local": {}, "nil": {}, "not": {}, "or": {}, "repeat": {}, "return": {},
	"then": {}, "true": {}, "until": {}, "while": {},
}

// LuaString returns a quoted, escaped Lua string literal safe for inclusion
// in any Lua source position that accepts a string literal.
//
// Encoding strategy: emit a long-bracket literal ([==[...]==]) when the
// string contains common dangerous characters; otherwise use a double-quoted
// literal with a fixed escape table. Long brackets cannot be terminated
// without the matching ]==], which we verify is absent.
func LuaString(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", fmt.Errorf("%w: not valid UTF-8", ErrUnsafeString)
	}
	if strings.ContainsRune(s, 0) {
		return "", fmt.Errorf("%w: contains NUL", ErrUnsafeString)
	}

	// Always use double-quoted with explicit escape — predictable, simple,
	// resistant to operator confusion. Long-bracket forms invite nesting bugs.
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case 0x0B:
			b.WriteString(`\v`)
		case 0x07:
			b.WriteString(`\a`)
		default:
			if r < 0x20 || r == 0x7F {
				// Non-printable controls — emit \ddd numeric escape.
				fmt.Fprintf(&b, `\%03d`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

// MustLuaString panics on error. Use only with values that have already been
// validated by callers (constants / allow-listed enums).
func MustLuaString(s string) string {
	out, err := LuaString(s)
	if err != nil {
		panic(err)
	}
	return out
}

// LuaIdent validates that s is a Lua identifier and is not reserved.
func LuaIdent(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidIdent)
	}
	if _, isReserved := luaReserved[s]; isReserved {
		return "", fmt.Errorf("%w: %q", ErrReservedIdent, s)
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return "", fmt.Errorf("%w: %q", ErrInvalidIdent, s)
		}
	}
	return s, nil
}

// LuaNumber returns a Lua numeric literal for f, or ErrInvalidNumber for
// NaN/Inf which Lua cannot represent as a literal.
func LuaNumber(f float64) (string, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", ErrInvalidNumber
	}
	return strconv.FormatFloat(f, 'g', -1, 64), nil
}
