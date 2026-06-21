package biz

import (
	"net/mail"
	"strings"
)

// Iris user status values.
const (
	UserInvited  = "invited"
	UserActive   = "active"
	UserDisabled = "disabled"
	UserLocked   = "locked"
)

// IrisUser is an administrative user of Iris.
type IrisUser struct {
	ID          string
	Email       string
	DisplayName string
	Status      string
	MFARequired bool
	Roles       []string
	// PasswordHash is the bcrypt digest used for login. It is internal-only and
	// never copied into API responses (see userToProto).
	PasswordHash string
}

// CanAuthenticate reports whether the user is permitted to authenticate.
func (u *IrisUser) CanAuthenticate() bool {
	return u.Status == UserActive
}

// Validate checks user invariants before persistence.
func (u *IrisUser) Validate() error {
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	u.DisplayName = strings.TrimSpace(u.DisplayName)
	if u.Status == "" {
		u.Status = UserInvited
	}
	if u.Email == "" {
		return Invalid("USER_EMAIL_REQUIRED", "email is required")
	}
	if _, err := mail.ParseAddress(u.Email); err != nil {
		return Invalid("USER_EMAIL_INVALID", "email %q is not valid", u.Email)
	}
	if !validUserStatus(u.Status) {
		return Invalid("USER_STATUS_INVALID", "status %q is not valid", u.Status)
	}
	return nil
}

func validUserStatus(s string) bool {
	switch s {
	case UserInvited, UserActive, UserDisabled, UserLocked:
		return true
	default:
		return false
	}
}

// Role is a named set of permissions.
type Role struct {
	ID          string
	Name        string
	Description string
	Permissions []string
	Builtin     bool
}

// Validate checks role invariants.
func (r *Role) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return Invalid("ROLE_NAME_REQUIRED", "role name is required")
	}
	for _, p := range r.Permissions {
		if strings.TrimSpace(p) == "" {
			return Invalid("ROLE_PERMISSION_INVALID", "role permissions must be non-empty")
		}
	}
	return nil
}

// ResolvePermissions merges the permissions of the given role names using the
// built-in role definitions plus any explicit role permission sets.
func ResolvePermissions(roleNames []string, custom map[string][]string) PermissionSet {
	set := PermissionSet{}
	for _, name := range roleNames {
		if perms, ok := BuiltinRolePermissions[name]; ok {
			for _, p := range perms {
				set[p] = struct{}{}
			}
		}
		if custom != nil {
			for _, p := range custom[name] {
				set[Permission(p)] = struct{}{}
			}
		}
	}
	return set
}
