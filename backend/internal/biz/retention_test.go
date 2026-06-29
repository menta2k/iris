package biz

import "testing"

func TestRetentionPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  RetentionPolicy
		wantErr bool
	}{
		{"valid keep+compress", RetentionPolicy{TableName: "mail_records", RetentionDays: 90, CompressAfterDays: 7}, false},
		{"keep forever, compress only", RetentionPolicy{TableName: "mail_records", RetentionDays: 0, CompressAfterDays: 7}, false},
		{"retention only", RetentionPolicy{TableName: "bounce_records", RetentionDays: 30}, false},
		{"keep forever no compress", RetentionPolicy{TableName: "audit_entries"}, false},
		{"unknown table", RetentionPolicy{TableName: "users", RetentionDays: 30}, true},
		{"negative retention", RetentionPolicy{TableName: "mail_records", RetentionDays: -1}, true},
		{"negative compress", RetentionPolicy{TableName: "mail_records", CompressAfterDays: -1}, true},
		{"compress >= retention", RetentionPolicy{TableName: "mail_records", RetentionDays: 7, CompressAfterDays: 7}, true},
		{"compress > retention", RetentionPolicy{TableName: "mail_records", RetentionDays: 7, CompressAfterDays: 14}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestManagedTableByName(t *testing.T) {
	if _, ok := ManagedTableByName("mail_records"); !ok {
		t.Fatal("mail_records should be managed")
	}
	if mt, _ := ManagedTableByName("feedback_reports"); mt.TimeColumn != "received_at" {
		t.Fatalf("feedback_reports time column = %q, want received_at", mt.TimeColumn)
	}
	if _, ok := ManagedTableByName("not_a_table"); ok {
		t.Fatal("unknown table should not be managed")
	}
}

func TestRetentionRunBytesFreed(t *testing.T) {
	r := &RetentionRun{BytesBefore: 1000, BytesAfter: 250}
	if r.BytesFreed() != 750 {
		t.Fatalf("BytesFreed = %d, want 750", r.BytesFreed())
	}
	// Growth (compression overhead before a drop, or new data) never shows negative.
	r = &RetentionRun{BytesBefore: 100, BytesAfter: 200}
	if r.BytesFreed() != 0 {
		t.Fatalf("BytesFreed = %d, want 0", r.BytesFreed())
	}
}
