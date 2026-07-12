package service

import (
	"context"
	"encoding/json"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// toProtoUserDashboard maps a biz.UserDashboard to its proto form. The widget
// layout is carried verbatim as a JSON string.
func toProtoUserDashboard(d *biz.UserDashboard) *adminv1.UserDashboard {
	return &adminv1.UserDashboard{
		Id:          d.ID,
		Name:        d.Name,
		IsDefault:   d.IsDefault,
		WidgetsJson: string(d.Widgets),
		CreatedAt:   d.CreatedAt.Unix(),
		UpdatedAt:   d.UpdatedAt.Unix(),
	}
}

// ListUserDashboards returns the calling user's custom dashboards.
func (s *Service) ListUserDashboards(ctx context.Context, _ *adminv1.ListUserDashboardsRequest) (*adminv1.ListUserDashboardsResponse, error) {
	if s.userDashboards == nil {
		return nil, notImplemented("ListUserDashboards")
	}
	list, err := s.userDashboards.List(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListUserDashboards", err)
	}
	out := &adminv1.ListUserDashboardsResponse{}
	for _, d := range list {
		out.Dashboards = append(out.Dashboards, toProtoUserDashboard(d))
	}
	return out, nil
}

// CreateUserDashboard creates a dashboard for the calling user.
func (s *Service) CreateUserDashboard(ctx context.Context, req *adminv1.CreateUserDashboardRequest) (*adminv1.UserDashboard, error) {
	if s.userDashboards == nil {
		return nil, notImplemented("CreateUserDashboard")
	}
	d, err := s.userDashboards.Create(ctx, req.GetName(), json.RawMessage(req.GetWidgetsJson()), req.GetMakeDefault())
	if err != nil {
		return nil, s.fail(ctx, "CreateUserDashboard", err)
	}
	return toProtoUserDashboard(d), nil
}

// UpdateUserDashboard edits the calling user's dashboard.
func (s *Service) UpdateUserDashboard(ctx context.Context, req *adminv1.UpdateUserDashboardRequest) (*adminv1.UserDashboard, error) {
	if s.userDashboards == nil {
		return nil, notImplemented("UpdateUserDashboard")
	}
	d, err := s.userDashboards.Update(ctx, req.GetId(), req.GetName(), json.RawMessage(req.GetWidgetsJson()))
	if err != nil {
		return nil, s.fail(ctx, "UpdateUserDashboard", err)
	}
	return toProtoUserDashboard(d), nil
}

// DeleteUserDashboard removes the calling user's dashboard.
func (s *Service) DeleteUserDashboard(ctx context.Context, req *adminv1.DeleteUserDashboardRequest) (*adminv1.DeleteUserDashboardResponse, error) {
	if s.userDashboards == nil {
		return nil, notImplemented("DeleteUserDashboard")
	}
	if err := s.userDashboards.Delete(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteUserDashboard", err)
	}
	return &adminv1.DeleteUserDashboardResponse{}, nil
}

// SetDefaultUserDashboard marks one of the user's dashboards as their default.
func (s *Service) SetDefaultUserDashboard(ctx context.Context, req *adminv1.SetDefaultUserDashboardRequest) (*adminv1.UserDashboard, error) {
	if s.userDashboards == nil {
		return nil, notImplemented("SetDefaultUserDashboard")
	}
	d, err := s.userDashboards.SetDefault(ctx, req.GetId())
	if err != nil {
		return nil, s.fail(ctx, "SetDefaultUserDashboard", err)
	}
	return toProtoUserDashboard(d), nil
}

// ListWidgetCatalog returns the curated metric widget catalog.
func (s *Service) ListWidgetCatalog(ctx context.Context, _ *adminv1.ListWidgetCatalogRequest) (*adminv1.ListWidgetCatalogResponse, error) {
	if s.metrics == nil {
		return nil, notImplemented("ListWidgetCatalog")
	}
	if _, err := biz.RequirePermission(ctx, biz.PermDashboardRead); err != nil {
		return nil, s.fail(ctx, "ListWidgetCatalog", err)
	}
	out := &adminv1.ListWidgetCatalogResponse{}
	for _, w := range biz.WidgetCatalog() {
		out.Widgets = append(out.Widgets, &adminv1.WidgetCatalogEntry{
			Key:             w.Key,
			Category:        w.Category,
			Title:           w.Title,
			Description:     w.Description,
			Unit:            w.Unit,
			Viz:             w.Viz,
			SupportsGroupBy: w.SupportsGroupBy,
			GroupByLabels:   w.GroupByLabels,
			DefaultRange:    w.DefaultRange,
			Instant:         w.Instant,
		})
	}
	return out, nil
}

// GetWidgetData executes one widget's metric query (catalog or guarded PromQL).
func (s *Service) GetWidgetData(ctx context.Context, req *adminv1.GetWidgetDataRequest) (*adminv1.MetricsTimeseries, error) {
	if s.metrics == nil {
		return nil, notImplemented("GetWidgetData")
	}
	ts, err := s.metrics.WidgetData(ctx, biz.WidgetDataRequest{
		Source:     req.GetSource(),
		CatalogKey: req.GetCatalogKey(),
		PromQL:     req.GetPromql(),
		Range:      req.GetRange(),
		GroupBy:    req.GetGroupBy(),
	})
	if err != nil {
		return nil, s.fail(ctx, "GetWidgetData", err)
	}
	return toProtoTimeseries(ts), nil
}
