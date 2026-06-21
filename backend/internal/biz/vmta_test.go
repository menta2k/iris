package biz

import "testing"

func TestVMTAValidate(t *testing.T) {
	tests := []struct {
		name    string
		vmta    VMTA
		wantErr string // empty means no error
	}{
		{"valid", VMTA{Name: "v1", ListenerID: "lst-1"}, ""},
		{"valid with max conns", VMTA{Name: "v1", ListenerID: "lst-1", MaxConnections: 50}, ""},
		{"missing name", VMTA{ListenerID: "lst-1"}, "VMTA_NAME_REQUIRED"},
		{"missing listener", VMTA{Name: "v1"}, "VMTA_LISTENER_REQUIRED"},
		{"negative max conns", VMTA{Name: "v1", ListenerID: "lst-1", MaxConnections: -1}, "VMTA_MAX_CONNECTIONS_RANGE"},
		{"bad status", VMTA{Name: "v1", ListenerID: "lst-1", Status: "nope"}, "VMTA_STATUS_INVALID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.vmta
			err := v.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if v.Status != VMTAStatusActive {
					t.Fatalf("expected default status active, got %q", v.Status)
				}
				return
			}
			de, ok := AsDomainError(err)
			if !ok {
				t.Fatalf("expected domain error, got %v", err)
			}
			if de.Reason != tt.wantErr {
				t.Fatalf("expected reason %q, got %q", tt.wantErr, de.Reason)
			}
		})
	}
}

func TestVMTAGroupValidate(t *testing.T) {
	tests := []struct {
		name    string
		group   VMTAGroup
		wantErr string
	}{
		{"valid", VMTAGroup{Name: "g1", Members: []VMTAGroupMember{{VMTAID: "a", Weight: 1}}}, ""},
		{"missing name", VMTAGroup{Members: []VMTAGroupMember{{VMTAID: "a", Weight: 1}}}, "VMTA_GROUP_NAME_REQUIRED"},
		{"active empty", VMTAGroup{Name: "g1"}, "VMTA_GROUP_EMPTY"},
		{"zero weight", VMTAGroup{Name: "g1", Members: []VMTAGroupMember{{VMTAID: "a", Weight: 0}}}, "VMTA_GROUP_WEIGHT_INVALID"},
		{"duplicate member", VMTAGroup{Name: "g1", Members: []VMTAGroupMember{{VMTAID: "a", Weight: 1}, {VMTAID: "a", Weight: 2}}}, "VMTA_GROUP_MEMBER_DUPLICATE"},
		{"disabled empty ok", VMTAGroup{Name: "g1", Status: VMTAGroupStatusDisabled}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := tt.group
			err := g.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			de, ok := AsDomainError(err)
			if !ok {
				t.Fatalf("expected domain error, got %v", err)
			}
			if de.Reason != tt.wantErr {
				t.Fatalf("expected reason %q, got %q", tt.wantErr, de.Reason)
			}
		})
	}
}

func TestVMTAGroupTotalWeight(t *testing.T) {
	g := VMTAGroup{Members: []VMTAGroupMember{{VMTAID: "a", Weight: 3}, {VMTAID: "b", Weight: 7}}}
	if got := g.TotalWeight(); got != 10 {
		t.Fatalf("expected total weight 10, got %d", got)
	}
}
