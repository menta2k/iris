package biz

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file is the single point at which database-backed configuration is
// turned into executable Lua for the KumoMTA policy. Every user-controlled
// string MUST flow through LuaString / LuaIdent / LuaNumber before reaching the
// rendered policy. The approach is adapted from the production Iris policy
// generator: be strict, never rely on template auto-escaping.

// luaReserved is the set of Lua 5.4 reserved words; we refuse to emit any of
// these as identifiers derived from user input.
var luaReserved = map[string]struct{}{
	"and": {}, "break": {}, "do": {}, "else": {}, "elseif": {}, "end": {},
	"false": {}, "for": {}, "function": {}, "goto": {}, "if": {}, "in": {},
	"local": {}, "nil": {}, "not": {}, "or": {}, "repeat": {}, "return": {},
	"then": {}, "true": {}, "until": {}, "while": {},
}

// LuaString returns a quoted, escaped Lua string literal safe for inclusion in
// any Lua position that accepts a string literal. It rejects NUL bytes and
// invalid UTF-8, and numerically escapes control characters.
func LuaString(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", Invalid("LUA_STRING_UNSAFE", "value is not valid UTF-8")
	}
	if strings.ContainsRune(s, 0) {
		return "", Invalid("LUA_STRING_UNSAFE", "value contains a NUL byte")
	}
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
				fmt.Fprintf(&b, `\%03d`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

// MustLuaString panics on error. Use only for values already validated by the
// caller (constants, allow-listed enums, or model-validated fields).
func MustLuaString(s string) string {
	out, err := LuaString(s)
	if err != nil {
		panic(err)
	}
	return out
}

// LuaIdent validates that s is a Lua identifier and not a reserved word. Use
// for table keys / names that derive from user input.
func LuaIdent(s string) (string, error) {
	if s == "" {
		return "", Invalid("LUA_IDENT_INVALID", "identifier is empty")
	}
	if _, reserved := luaReserved[s]; reserved {
		return "", Invalid("LUA_IDENT_RESERVED", "identifier %q is a Lua reserved word", s)
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return "", Invalid("LUA_IDENT_INVALID", "identifier %q contains an illegal character", s)
		}
	}
	return s, nil
}

// LuaNumber returns a Lua numeric literal for f, rejecting NaN/Inf which Lua
// cannot represent as a literal.
func LuaNumber(f float64) (string, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", Invalid("LUA_NUMBER_INVALID", "number is not finite")
	}
	return strconv.FormatFloat(f, 'g', -1, 64), nil
}

// sanitizeComment strips characters that could break out of a Lua line comment.
func sanitizeComment(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "\x00", "").Replace(s)
}
