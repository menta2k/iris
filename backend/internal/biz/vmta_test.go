package biz

import "testing"

func TestVMTAValidate(t *testing.T) {
	tests := []struct {
		name    string
		vmta    VMTA
		wantErr string // empty means no error
	}{
		// The VMTA now owns its egress identity: IP + EHLO required, listener optional.
		{"valid", VMTA{Name: "v1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com"}, ""},
		{"valid with listener + max conns", VMTA{Name: "v1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", ListenerID: "lst-1", MaxConnections: 50}, ""},
		{"missing name", VMTA{IPAddress: "203.0.113.1", EHLOName: "v1.example.com"}, "VMTA_NAME_REQUIRED"},
		{"missing ip", VMTA{Name: "v1", EHLOName: "v1.example.com"}, "VMTA_IP_INVALID"},
		{"wildcard ip", VMTA{Name: "v1", IPAddress: "0.0.0.0", EHLOName: "v1.example.com"}, "VMTA_IP_WILDCARD"},
		{"missing ehlo", VMTA{Name: "v1", IPAddress: "203.0.113.1"}, "VMTA_EHLO_REQUIRED"},
		{"bad ehlo", VMTA{Name: "v1", IPAddress: "203.0.113.1", EHLOName: "not a host!"}, "VMTA_EHLO_INVALID"},
		{"negative max conns", VMTA{Name: "v1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", MaxConnections: -1}, "VMTA_MAX_CONNECTIONS_RANGE"},
		{"bad status", VMTA{Name: "v1", IPAddress: "203.0.113.1", EHLOName: "v1.example.com", Status: "nope"}, "VMTA_STATUS_INVALID"},
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
