package biz

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// fakeUserDashboardRepo is an in-memory UserDashboardRepo scoped by userID.
type fakeUserDashboardRepo struct {
	items  map[string]*UserDashboard // id -> dashboard
	nextID int
}

func newFakeUDRepo() *fakeUserDashboardRepo {
	return &fakeUserDashboardRepo{items: map[string]*UserDashboard{}}
}

func (r *fakeUserDashboardRepo) List(_ context.Context, userID string) ([]*UserDashboard, error) {
	var out []*UserDashboard
	for _, d := range r.items {
		if d.UserID == userID {
			cp := *d
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeUserDashboardRepo) Get(_ context.Context, userID, id string) (*UserDashboard, error) {
	d, ok := r.items[id]
	if !ok || d.UserID != userID {
		return nil, NotFound("DASHBOARD_NOT_FOUND", "not found")
	}
	cp := *d
	return &cp, nil
}

func (r *fakeUserDashboardRepo) Create(_ context.Context, d *UserDashboard) (*UserDashboard, error) {
	r.nextID++
	cp := *d
	cp.ID = time.Now().Format("150405") + string(rune('a'+r.nextID))
	cp.CreatedAt = time.Unix(int64(r.nextID), 0)
	cp.UpdatedAt = cp.CreatedAt
	r.items[cp.ID] = &cp
	ret := cp
	return &ret, nil
}

func (r *fakeUserDashboardRepo) Update(_ context.Context, d *UserDashboard) (*UserDashboard, error) {
	existing, ok := r.items[d.ID]
	if !ok || existing.UserID != d.UserID {
		return nil, NotFound("DASHBOARD_NOT_FOUND", "not found")
	}
	existing.Name = d.Name
	existing.Widgets = d.Widgets
	cp := *existing
	return &cp, nil
}

func (r *fakeUserDashboardRepo) Delete(_ context.Context, userID, id string) error {
	d, ok := r.items[id]
	if !ok || d.UserID != userID {
		return NotFound("DASHBOARD_NOT_FOUND", "not found")
	}
	delete(r.items, id)
	return nil
}

func (r *fakeUserDashboardRepo) SetDefault(_ context.Context, userID, id string) error {
	target, ok := r.items[id]
	if !ok || target.UserID != userID {
		return NotFound("DASHBOARD_NOT_FOUND", "not found")
	}
	for _, d := range r.items {
		if d.UserID == userID {
			d.IsDefault = false
		}
	}
	target.IsDefault = true
	return nil
}

func TestUserDashboardCreateValidatesName(t *testing.T) {
	uc := NewUserDashboardUsecase(newFakeUDRepo())
	_, err := uc.Create(ownerCtx(), "  ", nil, false)
	de, ok := AsDomainError(err)
	if !ok || de.Reason != "DASHBOARD_NAME_REQUIRED" {
		t.Fatalf("expected DASHBOARD_NAME_REQUIRED, got %v", err)
	}
}

func TestUserDashboardCreateDefaultsEmptyWidgets(t *testing.T) {
	uc := NewUserDashboardUsecase(newFakeUDRepo())
	d, err := uc.Create(ownerCtx(), "Ops", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(d.Widgets) != "[]" {
		t.Fatalf("expected empty widgets to default to [], got %q", d.Widgets)
	}
}

func TestUserDashboardCreateRejectsNonArrayWidgets(t *testing.T) {
	uc := NewUserDashboardUsecase(newFakeUDRepo())
	_, err := uc.Create(ownerCtx(), "Ops", json.RawMessage(`{"not":"array"}`), false)
	de, ok := AsDomainError(err)
	if !ok || de.Reason != "DASHBOARD_WIDGETS_INVALID" {
		t.Fatalf("expected DASHBOARD_WIDGETS_INVALID, got %v", err)
	}
}

func TestUserDashboardWidgetsSizeCap(t *testing.T) {
	uc := NewUserDashboardUsecase(newFakeUDRepo())
	big := make([]byte, maxWidgetsJSONBytes+1)
	for i := range big {
		big[i] = 'x'
	}
	_, err := uc.Create(ownerCtx(), "Ops", json.RawMessage(big), false)
	de, ok := AsDomainError(err)
	if !ok || de.Reason != "DASHBOARD_WIDGETS_TOO_LARGE" {
		t.Fatalf("expected DASHBOARD_WIDGETS_TOO_LARGE, got %v", err)
	}
}

func TestUserDashboardSetDefaultClearsPrior(t *testing.T) {
	repo := newFakeUDRepo()
	uc := NewUserDashboardUsecase(repo)
	a, _ := uc.Create(ownerCtx(), "A", nil, false)
	b, _ := uc.Create(ownerCtx(), "B", nil, false)

	if _, err := uc.SetDefault(ownerCtx(), a.ID); err != nil {
		t.Fatalf("set default A: %v", err)
	}
	if _, err := uc.SetDefault(ownerCtx(), b.ID); err != nil {
		t.Fatalf("set default B: %v", err)
	}
	// Only one default remains.
	defaults := 0
	for _, d := range repo.items {
		if d.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("expected exactly one default, got %d", defaults)
	}
	if !repo.items[b.ID].IsDefault || repo.items[a.ID].IsDefault {
		t.Fatal("expected B to be the sole default")
	}
}

func TestUserDashboardOwnershipIsolation(t *testing.T) {
	repo := newFakeUDRepo()
	// A dashboard owned by another user.
	repo.items["x"] = &UserDashboard{ID: "x", UserID: "other", Name: "theirs", Widgets: json.RawMessage("[]")}
	uc := NewUserDashboardUsecase(repo)

	// Caller "tester" (from ownerCtx) cannot delete another user's dashboard.
	if err := uc.Delete(ownerCtx(), "x"); err == nil {
		t.Fatal("expected NotFound deleting another user's dashboard")
	}
	// And it does not appear in their list.
	list, err := uc.List(ownerCtx())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, d := range list {
		if d.ID == "x" {
			t.Fatal("another user's dashboard leaked into the list")
		}
	}
}

func TestUserDashboardRequiresPermission(t *testing.T) {
	uc := NewUserDashboardUsecase(newFakeUDRepo())
	ctx := WithIdentity(context.Background(), &Identity{Permissions: NewPermissionSet(nil), MFAVerified: true})
	if _, err := uc.List(ctx); err == nil {
		t.Fatal("expected permission error")
	}
}
