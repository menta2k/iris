package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// maxWidgetsJSONBytes caps the size of a dashboard's widget document. The layout
// is opaque to the backend (the frontend owns the widget schema); this is a
// sanity bound, not a schema check.
const maxWidgetsJSONBytes = 64 * 1024

// UserDashboard is one operator-owned custom dashboard. Widgets is an opaque
// JSON array of widget configs (gridstack geometry + metric source) that the
// frontend interprets; the backend only validates it is an array within the
// size cap.
type UserDashboard struct {
	ID        string
	UserID    string
	Name      string
	IsDefault bool
	Widgets   json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserDashboardRepo is the persistence boundary. Every method is scoped by
// userID so a user can only ever read/write their own dashboards.
type UserDashboardRepo interface {
	List(ctx context.Context, userID string) ([]*UserDashboard, error)
	Get(ctx context.Context, userID, id string) (*UserDashboard, error)
	Create(ctx context.Context, d *UserDashboard) (*UserDashboard, error)
	Update(ctx context.Context, d *UserDashboard) (*UserDashboard, error)
	Delete(ctx context.Context, userID, id string) error
	// SetDefault clears the user's prior default and sets id as default in one
	// transaction.
	SetDefault(ctx context.Context, userID, id string) error
}

// UserDashboardUsecase is the per-user dashboard CRUD. Dashboards are personal
// data: any authenticated user with dashboard:read may manage their OWN
// dashboards, and every operation is scoped to the caller's identity.
type UserDashboardUsecase struct {
	repo UserDashboardRepo
}

// NewUserDashboardUsecase constructs the use case.
func NewUserDashboardUsecase(repo UserDashboardRepo) *UserDashboardUsecase {
	return &UserDashboardUsecase{repo: repo}
}

// List returns the caller's dashboards.
func (uc *UserDashboardUsecase) List(ctx context.Context) ([]*UserDashboard, error) {
	id, err := RequirePermission(ctx, PermDashboardRead)
	if err != nil {
		return nil, err
	}
	return uc.repo.List(ctx, id.UserID)
}

// Create adds a dashboard for the caller.
func (uc *UserDashboardUsecase) Create(ctx context.Context, name string, widgets json.RawMessage, makeDefault bool) (*UserDashboard, error) {
	id, err := RequirePermission(ctx, PermDashboardRead)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, Invalid("DASHBOARD_NAME_REQUIRED", "dashboard name is required")
	}
	widgets, err = normalizeWidgets(widgets)
	if err != nil {
		return nil, err
	}
	out, err := uc.repo.Create(ctx, &UserDashboard{
		UserID: id.UserID, Name: name, IsDefault: makeDefault, Widgets: widgets,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Update edits the caller's dashboard (name + widget layout).
func (uc *UserDashboardUsecase) Update(ctx context.Context, dashID, name string, widgets json.RawMessage) (*UserDashboard, error) {
	id, err := RequirePermission(ctx, PermDashboardRead)
	if err != nil {
		return nil, err
	}
	if dashID == "" {
		return nil, Invalid("DASHBOARD_ID_REQUIRED", "id is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, Invalid("DASHBOARD_NAME_REQUIRED", "dashboard name is required")
	}
	widgets, err = normalizeWidgets(widgets)
	if err != nil {
		return nil, err
	}
	return uc.repo.Update(ctx, &UserDashboard{ID: dashID, UserID: id.UserID, Name: name, Widgets: widgets})
}

// Delete removes the caller's dashboard.
func (uc *UserDashboardUsecase) Delete(ctx context.Context, dashID string) error {
	id, err := RequirePermission(ctx, PermDashboardRead)
	if err != nil {
		return err
	}
	if dashID == "" {
		return Invalid("DASHBOARD_ID_REQUIRED", "id is required")
	}
	return uc.repo.Delete(ctx, id.UserID, dashID)
}

// SetDefault marks the caller's dashboard as their default.
func (uc *UserDashboardUsecase) SetDefault(ctx context.Context, dashID string) (*UserDashboard, error) {
	id, err := RequirePermission(ctx, PermDashboardRead)
	if err != nil {
		return nil, err
	}
	if dashID == "" {
		return nil, Invalid("DASHBOARD_ID_REQUIRED", "id is required")
	}
	if err := uc.repo.SetDefault(ctx, id.UserID, dashID); err != nil {
		return nil, err
	}
	return uc.repo.Get(ctx, id.UserID, dashID)
}

// normalizeWidgets validates the widget document (a JSON array within the size
// cap) and defaults an empty value to "[]".
func normalizeWidgets(widgets json.RawMessage) (json.RawMessage, error) {
	if len(widgets) == 0 {
		return json.RawMessage("[]"), nil
	}
	if len(widgets) > maxWidgetsJSONBytes {
		return nil, Invalid("DASHBOARD_WIDGETS_TOO_LARGE", "widget layout exceeds %d bytes", maxWidgetsJSONBytes)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(widgets, &arr); err != nil {
		return nil, Invalid("DASHBOARD_WIDGETS_INVALID", "widgets must be a JSON array")
	}
	return widgets, nil
}
