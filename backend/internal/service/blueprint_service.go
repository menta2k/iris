package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListDeliveryBlueprints returns all base shaping blueprints.
func (s *Service) ListDeliveryBlueprints(ctx context.Context, _ *adminv1.ListDeliveryBlueprintsRequest) (*adminv1.ListDeliveryBlueprintsReply, error) {
	if s.blueprints == nil {
		return nil, notImplemented("ListDeliveryBlueprints")
	}
	items, err := s.blueprints.List(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListDeliveryBlueprints", err)
	}
	out := &adminv1.ListDeliveryBlueprintsReply{}
	for _, b := range items {
		out.Items = append(out.Items, blueprintToProto(b))
	}
	return out, nil
}

// CreateDeliveryBlueprint adds a base shaping rule.
func (s *Service) CreateDeliveryBlueprint(ctx context.Context, req *adminv1.CreateDeliveryBlueprintRequest) (*adminv1.DeliveryBlueprint, error) {
	if s.blueprints == nil {
		return nil, notImplemented("CreateDeliveryBlueprint")
	}
	out, err := s.blueprints.Create(ctx, &biz.DeliveryBlueprint{
		Provider: req.GetProvider(), MXPattern: req.GetMxPattern(), ConnRate: req.GetConnRate(),
		DeliveriesPerConn: int(req.GetDeliveriesPerConn()), ConnLimit: int(req.GetConnLimit()), DailyCap: int(req.GetDailyCap()),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateDeliveryBlueprint", err)
	}
	return blueprintToProto(out), nil
}

// UpdateDeliveryBlueprint edits a base shaping rule.
func (s *Service) UpdateDeliveryBlueprint(ctx context.Context, req *adminv1.UpdateDeliveryBlueprintRequest) (*adminv1.DeliveryBlueprint, error) {
	if s.blueprints == nil {
		return nil, notImplemented("UpdateDeliveryBlueprint")
	}
	out, err := s.blueprints.Update(ctx, req.GetId(), &biz.DeliveryBlueprint{
		Provider: req.GetProvider(), MXPattern: req.GetMxPattern(), ConnRate: req.GetConnRate(),
		DeliveriesPerConn: int(req.GetDeliveriesPerConn()), ConnLimit: int(req.GetConnLimit()), DailyCap: int(req.GetDailyCap()),
		Status: req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateDeliveryBlueprint", err)
	}
	return blueprintToProto(out), nil
}

// SetDeliveryBlueprintStatus enables/disables a blueprint (the power toggle).
func (s *Service) SetDeliveryBlueprintStatus(ctx context.Context, req *adminv1.SetDeliveryBlueprintStatusRequest) (*adminv1.DeliveryBlueprint, error) {
	if s.blueprints == nil {
		return nil, notImplemented("SetDeliveryBlueprintStatus")
	}
	out, err := s.blueprints.SetStatus(ctx, req.GetId(), req.GetStatus())
	if err != nil {
		return nil, s.fail(ctx, "SetDeliveryBlueprintStatus", err)
	}
	return blueprintToProto(out), nil
}

// SeedDeliveryBlueprints imports the built-in provider defaults.
func (s *Service) SeedDeliveryBlueprints(ctx context.Context, _ *adminv1.SeedDeliveryBlueprintsRequest) (*adminv1.SeedDeliveryBlueprintsReply, error) {
	if s.blueprints == nil {
		return nil, notImplemented("SeedDeliveryBlueprints")
	}
	n, err := s.blueprints.SeedDefaults(ctx)
	if err != nil {
		return nil, s.fail(ctx, "SeedDeliveryBlueprints", err)
	}
	return &adminv1.SeedDeliveryBlueprintsReply{Inserted: int32(n)}, nil
}

func blueprintToProto(b *biz.DeliveryBlueprint) *adminv1.DeliveryBlueprint {
	p := &adminv1.DeliveryBlueprint{
		Id:                b.ID,
		Provider:          b.Provider,
		MxPattern:         b.MXPattern,
		ConnRate:          b.ConnRate,
		DeliveriesPerConn: int32(b.DeliveriesPerConn),
		ConnLimit:         int32(b.ConnLimit),
		DailyCap:          int32(b.DailyCap),
		Status:            b.Status,
	}
	if !b.CreatedAt.IsZero() {
		p.CreatedAt = b.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !b.UpdatedAt.IsZero() {
		p.UpdatedAt = b.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return p
}
