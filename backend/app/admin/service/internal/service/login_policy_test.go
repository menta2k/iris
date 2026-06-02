package service

import (
	"context"
	"errors"
	"testing"
)

// fakeLPStore is an in-memory LoginPolicyStore for service tests.
type fakeLPStore struct {
	rows   []LoginPolicyRow
	nextID uint32
}

func (f *fakeLPStore) List(_ context.Context, _, _ int) ([]LoginPolicyRow, error) {
	return f.rows, nil
}
func (f *fakeLPStore) Count(_ context.Context) (int, error) { return len(f.rows), nil }
func (f *fakeLPStore) Get(_ context.Context, id uint32) (*LoginPolicyRow, error) {
	for i := range f.rows {
		if f.rows[i].ID == id {
			r := f.rows[i]
			return &r, nil
		}
	}
	return nil, nil
}
func (f *fakeLPStore) Create(_ context.Context, in LoginPolicyRow) (*LoginPolicyRow, error) {
	f.nextID++
	in.ID = f.nextID
	f.rows = append(f.rows, in)
	return &in, nil
}
func (f *fakeLPStore) Update(_ context.Context, id uint32, in LoginPolicyRow) (*LoginPolicyRow, error) {
	in.ID = id
	return &in, nil
}
func (f *fakeLPStore) Delete(_ context.Context, _, _ uint32) error { return nil }
func (f *fakeLPStore) ListApplicable(_ context.Context, userID *uint32) ([]LoginPolicyRow, error) {
	var out []LoginPolicyRow
	for _, r := range f.rows {
		if !r.Enabled {
			continue
		}
		if r.TargetID == 0 || (userID != nil && r.TargetID == *userID) {
			out = append(out, r)
		}
	}
	return out, nil
}

func TestValidateLoginPolicy(t *testing.T) {
	tests := []struct {
		name    string
		in      LoginPolicyRow
		wantErr bool
	}{
		{"valid ip cidr", LoginPolicyRow{Type: PolicyTypeBlacklist, Method: MethodIP, Value: "10.0.0.0/8"}, false},
		{"valid bare ip", LoginPolicyRow{Type: PolicyTypeBlacklist, Method: MethodIP, Value: "1.2.3.4"}, false},
		{"bad ip", LoginPolicyRow{Type: PolicyTypeBlacklist, Method: MethodIP, Value: "not-an-ip"}, true},
		{"valid region", LoginPolicyRow{Type: PolicyTypeWhitelist, Method: MethodRegion, Value: "bg"}, false},
		{"bad region", LoginPolicyRow{Type: PolicyTypeWhitelist, Method: MethodRegion, Value: "BGR"}, true},
		{"mac rejected", LoginPolicyRow{Type: PolicyTypeBlacklist, Method: MethodMAC, Value: "aa:bb"}, true},
		{"device rejected", LoginPolicyRow{Type: PolicyTypeBlacklist, Method: MethodDevice, Value: "x"}, true},
		{"bad type", LoginPolicyRow{Type: "MAYBE", Method: MethodIP, Value: "1.2.3.4"}, true},
		{"time missing window", LoginPolicyRow{Type: PolicyTypeWhitelist, Method: MethodTime}, true},
		{"time valid", LoginPolicyRow{Type: PolicyTypeWhitelist, Method: MethodTime,
			TimeWindow: &TimeWindow{Start: "09:00", End: "17:00", Timezone: "Europe/Sofia"}}, false},
		{"time bad tz", LoginPolicyRow{Type: PolicyTypeWhitelist, Method: MethodTime,
			TimeWindow: &TimeWindow{Start: "09:00", End: "17:00", Timezone: "Mars/Phobos"}}, true},
		{"time wrap rejected", LoginPolicyRow{Type: PolicyTypeWhitelist, Method: MethodTime,
			TimeWindow: &TimeWindow{Start: "22:00", End: "06:00"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := tt.in
			normaliseLoginPolicy(&in)
			err := validateLoginPolicy(&in)
			if tt.wantErr != (err != nil) {
				t.Fatalf("validateLoginPolicy err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestRegionNormalisedToUpper(t *testing.T) {
	svc := NewLoginPolicyService(&fakeLPStore{}, nil)
	row, err := svc.Create(context.Background(),
		LoginPolicyRow{Type: PolicyTypeWhitelist, Method: MethodRegion, Value: "bg", Enabled: true},
		1, "1.2.3.4", true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if row.Value != "BG" {
		t.Fatalf("region value = %q, want BG", row.Value)
	}
}

func TestSelfLockoutGuard(t *testing.T) {
	svc := NewLoginPolicyService(&fakeLPStore{}, nil)
	// A global blacklist covering the acting admin's current IP.
	rule := LoginPolicyRow{Type: PolicyTypeBlacklist, Method: MethodIP, Value: "1.2.3.0/24", Enabled: true}

	// Without acknowledge → blocked by the guard.
	_, err := svc.Create(context.Background(), rule, 1, "1.2.3.4", false)
	if !errors.Is(err, ErrWouldLockOutSelf) {
		t.Fatalf("expected ErrWouldLockOutSelf, got %v", err)
	}

	// With acknowledge → allowed.
	_, err = svc.Create(context.Background(), rule, 1, "1.2.3.4", true)
	if err != nil {
		t.Fatalf("acknowledge should bypass guard, got %v", err)
	}
}

func TestSelfLockoutGuardIgnoresUnrelatedIP(t *testing.T) {
	svc := NewLoginPolicyService(&fakeLPStore{}, nil)
	// Blacklist a range the acting admin is NOT in → no lockout, no ack needed.
	rule := LoginPolicyRow{Type: PolicyTypeBlacklist, Method: MethodIP, Value: "10.0.0.0/8", Enabled: true}
	if _, err := svc.Create(context.Background(), rule, 1, "1.2.3.4", false); err != nil {
		t.Fatalf("guard should not trigger for an unrelated range, got %v", err)
	}
}
