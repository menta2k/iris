package biz

// Permission is a fine-grained capability checked before protected operations.
type Permission string

const (
	// Outbound configuration.
	PermVMTARead     Permission = "vmta:read"
	PermVMTAWrite    Permission = "vmta:write"
	PermRoutingRead  Permission = "routing:read"
	PermRoutingWrite Permission = "routing:write"

	// Mail operations.
	PermMailRead       Permission = "mail:read"
	PermQueueRead      Permission = "queue:read"
	PermQueueControl   Permission = "queue:control"
	PermServiceControl Permission = "service:control"

	// Domain & recipient safety.
	PermDKIMRead         Permission = "dkim:read"
	PermDKIMWrite        Permission = "dkim:write"
	PermSuppressionRead  Permission = "suppression:read"
	PermSuppressionWrite Permission = "suppression:write"

	// Inbound automation.
	PermWebhookRead  Permission = "webhook:read"
	PermWebhookWrite Permission = "webhook:write"
	PermRspamdRead   Permission = "rspamd:read"

	// Security administration.
	PermUserRead  Permission = "user:read"
	PermUserWrite Permission = "user:write"
	PermAuditRead Permission = "audit:read"

	// Dashboard.
	PermDashboardRead Permission = "dashboard:read"

	// Global settings (deployment-level policy knobs).
	PermSettingsRead  Permission = "settings:read"
	PermSettingsWrite Permission = "settings:write"

	// Wildcard granting all permissions; reserved for the owner role.
	PermAll Permission = "*"
)

// Built-in role names.
const (
	RoleOwner    = "owner"
	RoleOperator = "operator"
	RoleSecurity = "security_admin"
	RoleViewer   = "viewer"
)

// BuiltinRolePermissions maps built-in roles to their granted permissions.
var BuiltinRolePermissions = map[string][]Permission{
	RoleOwner: {PermAll},
	RoleOperator: {
		PermVMTARead, PermVMTAWrite, PermRoutingRead, PermRoutingWrite,
		PermMailRead, PermQueueRead, PermQueueControl, PermServiceControl,
		PermDKIMRead, PermDKIMWrite, PermSuppressionRead, PermSuppressionWrite,
		PermWebhookRead, PermWebhookWrite, PermRspamdRead, PermDashboardRead,
		PermSettingsRead, PermSettingsWrite,
	},
	RoleSecurity: {
		PermUserRead, PermUserWrite, PermAuditRead, PermDashboardRead,
		PermMailRead, PermQueueRead,
	},
	RoleViewer: {
		PermVMTARead, PermRoutingRead, PermMailRead, PermQueueRead,
		PermDKIMRead, PermSuppressionRead, PermWebhookRead, PermRspamdRead,
		PermDashboardRead, PermUserRead, PermAuditRead, PermSettingsRead,
	},
}

// PermissionSet is an efficient lookup set of granted permissions.
type PermissionSet map[Permission]struct{}

// NewPermissionSet builds a set from a list of permission strings.
func NewPermissionSet(perms []string) PermissionSet {
	set := make(PermissionSet, len(perms))
	for _, p := range perms {
		set[Permission(p)] = struct{}{}
	}
	return set
}

// Has reports whether the set grants the required permission, honoring the
// wildcard permission.
func (s PermissionSet) Has(required Permission) bool {
	if _, ok := s[PermAll]; ok {
		return true
	}
	_, ok := s[required]
	return ok
}

// Authorize returns a forbidden DomainError if the set lacks the permission.
func (s PermissionSet) Authorize(required Permission) error {
	if s.Has(required) {
		return nil
	}
	return Forbidden("PERMISSION_DENIED", "missing required permission %q", required)
}
