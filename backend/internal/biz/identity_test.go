package biz

import "testing"

func TestIrisUserValidate(t *testing.T) {
	assertReason(t, (&IrisUser{Email: "a@b.com"}).Validate(), "")
	assertReason(t, (&IrisUser{}).Validate(), "USER_EMAIL_REQUIRED")
	assertReason(t, (&IrisUser{Email: "not-an-email"}).Validate(), "USER_EMAIL_INVALID")
	assertReason(t, (&IrisUser{Email: "a@b.com", Status: "bogus"}).Validate(), "USER_STATUS_INVALID")
}

func TestUserCanAuthenticate(t *testing.T) {
	if !(&IrisUser{Status: UserActive}).CanAuthenticate() {
		t.Fatal("active user should authenticate")
	}
	for _, st := range []string{UserInvited, UserDisabled, UserLocked} {
		if (&IrisUser{Status: st}).CanAuthenticate() {
			t.Fatalf("status %q should not authenticate", st)
		}
	}
}

func TestResolvePermissions(t *testing.T) {
	// Owner wildcard grants everything.
	owner := ResolvePermissions([]string{RoleOwner}, nil)
	if !owner.Has(PermServiceControl) || !owner.Has(PermUserWrite) {
		t.Fatal("owner should have all permissions")
	}
	// Viewer is read-only: must not have write/control permissions.
	viewer := ResolvePermissions([]string{RoleViewer}, nil)
	if viewer.Has(PermServiceControl) || viewer.Has(PermVMTAWrite) {
		t.Fatal("viewer must not have write/control permissions")
	}
	if !viewer.Has(PermMailRead) {
		t.Fatal("viewer should have mail read")
	}
	// Custom role permissions merge in.
	custom := ResolvePermissions([]string{"custom"}, map[string][]string{"custom": {string(PermAuditRead)}})
	if !custom.Has(PermAuditRead) {
		t.Fatal("custom role permission should resolve")
	}
}

func TestRoleValidate(t *testing.T) {
	assertReason(t, (&Role{Name: "r", Permissions: []string{"a:b"}}).Validate(), "")
	assertReason(t, (&Role{}).Validate(), "ROLE_NAME_REQUIRED")
	assertReason(t, (&Role{Name: "r", Permissions: []string{" "}}).Validate(), "ROLE_PERMISSION_INVALID")
}
