// Package authorizer evaluates permission strings against a caller's
// granted permissions.
//
// Permission grammar:
//
//   <resource>:<action>
//
//   resource: dot-separated identifier ("kumo.policy", "audit.log", "user")
//   action:   read | write | delete | apply | * (any)
//
// Wildcards:
//
//   "*:*"               grants everything (admin)
//   "kumo.*:*"          grants all actions on any kumo.* resource
//   "kumo.policy:*"     grants any action on kumo.policy
//   "kumo.policy:read"  grants exactly one action
//
// The resource component supports a single trailing ".*" segment.
//
// Performance: matches are O(N) where N = grants. Each grant is parsed once
// at load time; the per-request hot path does only string comparisons.
package authorizer

import (
	"errors"
	"strings"
)

// Permission is a parsed grant.
type Permission struct {
	resource string // canonical, lowercased
	wild     bool   // true iff the resource ends in ".*"
	prefix   string // for wildcard, the prefix without the ".*" — including dot
	action   string // canonical, lowercased; "*" for any
}

// ErrInvalidPermission is returned for malformed permission strings.
var ErrInvalidPermission = errors.New("authorizer: invalid permission string")

// Parse turns "kumo.policy:write" into a Permission.
func Parse(s string) (Permission, error) {
	res, action, ok := strings.Cut(s, ":")
	if !ok || res == "" || action == "" {
		return Permission{}, ErrInvalidPermission
	}
	res = strings.ToLower(res)
	action = strings.ToLower(action)
	p := Permission{resource: res, action: action}
	if strings.HasSuffix(res, ".*") {
		p.wild = true
		p.prefix = strings.TrimSuffix(res, "*") // keep trailing dot
	}
	if res == "*" {
		p.wild = true
		p.prefix = ""
		p.resource = "*"
	}
	return p, nil
}

// Matches returns true iff p grants action on resource.
func (p Permission) Matches(resource, action string) bool {
	resource = strings.ToLower(resource)
	action = strings.ToLower(action)
	if p.action != "*" && p.action != action {
		return false
	}
	switch {
	case p.resource == "*":
		return true
	case p.wild:
		return strings.HasPrefix(resource, p.prefix) || resource+"." == p.prefix
	default:
		return p.resource == resource
	}
}

// Authorizer holds the parsed grants and applies them to access decisions.
type Authorizer struct {
	grants []Permission
}

// New builds an Authorizer from a slice of permission strings. Invalid
// entries are silently dropped — callers should validate at write time.
func New(perms []string) *Authorizer {
	out := make([]Permission, 0, len(perms))
	for _, raw := range perms {
		p, err := Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return &Authorizer{grants: out}
}

// Allow returns true iff the caller can perform `action` on `resource`.
func (a *Authorizer) Allow(resource, action string) bool {
	for _, g := range a.grants {
		if g.Matches(resource, action) {
			return true
		}
	}
	return false
}
