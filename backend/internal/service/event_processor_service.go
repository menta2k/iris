package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListEventProcessors returns all Event Processor rules.
func (s *Service) ListEventProcessors(ctx context.Context, _ *adminv1.ListEventProcessorsRequest) (*adminv1.ListEventProcessorsReply, error) {
	if s.eventProcessors == nil {
		return nil, notImplemented("ListEventProcessors")
	}
	items, err := s.eventProcessors.List(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListEventProcessors", err)
	}
	out := &adminv1.ListEventProcessorsReply{}
	for _, p := range items {
		out.Items = append(out.Items, eventProcessorToProto(p))
	}
	return out, nil
}

// CreateEventProcessor adds a processor.
func (s *Service) CreateEventProcessor(ctx context.Context, req *adminv1.CreateEventProcessorRequest) (*adminv1.EventProcessor, error) {
	if s.eventProcessors == nil {
		return nil, notImplemented("CreateEventProcessor")
	}
	out, err := s.eventProcessors.Create(ctx, &biz.EventProcessor{
		Name: req.GetName(), EventTypes: req.GetEventTypes(), Mailclasses: req.GetMailclasses(),
		Driver: req.GetDriver(), DriverConfig: req.GetDriverConfig(), Mode: req.GetMode(),
		BatchMaxSize: int(req.GetBatchMaxSize()), BatchMaxWait: req.GetBatchMaxWait(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateEventProcessor", err)
	}
	return eventProcessorToProto(out), nil
}

// UpdateEventProcessor edits a processor.
func (s *Service) UpdateEventProcessor(ctx context.Context, req *adminv1.UpdateEventProcessorRequest) (*adminv1.EventProcessor, error) {
	if s.eventProcessors == nil {
		return nil, notImplemented("UpdateEventProcessor")
	}
	out, err := s.eventProcessors.Update(ctx, req.GetId(), &biz.EventProcessor{
		Name: req.GetName(), EventTypes: req.GetEventTypes(), Mailclasses: req.GetMailclasses(),
		Driver: req.GetDriver(), DriverConfig: req.GetDriverConfig(), Mode: req.GetMode(),
		BatchMaxSize: int(req.GetBatchMaxSize()), BatchMaxWait: req.GetBatchMaxWait(), Status: req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateEventProcessor", err)
	}
	return eventProcessorToProto(out), nil
}

// DeleteEventProcessor removes a processor.
func (s *Service) DeleteEventProcessor(ctx context.Context, req *adminv1.DeleteEventProcessorRequest) (*adminv1.DeleteEventProcessorReply, error) {
	if s.eventProcessors == nil {
		return nil, notImplemented("DeleteEventProcessor")
	}
	if err := s.eventProcessors.Delete(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteEventProcessor", err)
	}
	return &adminv1.DeleteEventProcessorReply{}, nil
}

// TestEventProcessor sends a synthetic event through the processor's driver.
func (s *Service) TestEventProcessor(ctx context.Context, req *adminv1.TestEventProcessorRequest) (*adminv1.TestEventProcessorReply, error) {
	if s.eventProcessors == nil {
		return nil, notImplemented("TestEventProcessor")
	}
	err := s.eventProcessors.Test(ctx, &biz.EventProcessor{
		Name: req.GetName(), EventTypes: req.GetEventTypes(), Mailclasses: req.GetMailclasses(),
		Driver: req.GetDriver(), DriverConfig: req.GetDriverConfig(), Mode: req.GetMode(),
		BatchMaxSize: int(req.GetBatchMaxSize()), BatchMaxWait: req.GetBatchMaxWait(),
	})
	if err != nil {
		// A delivery failure is a normal (reportable) outcome, not an RPC error.
		if de, ok := err.(*biz.DomainError); ok && de.Reason == "EVENT_TEST_DELIVERY_FAILED" {
			return &adminv1.TestEventProcessorReply{Ok: false, Error: de.Message}, nil
		}
		return nil, s.fail(ctx, "TestEventProcessor", err)
	}
	return &adminv1.TestEventProcessorReply{Ok: true}, nil
}

func eventProcessorToProto(p *biz.EventProcessor) *adminv1.EventProcessor {
	out := &adminv1.EventProcessor{
		Id: p.ID, Name: p.Name, EventTypes: p.EventTypes, Mailclasses: p.Mailclasses,
		Driver: p.Driver, DriverConfig: p.DriverConfig, Mode: p.Mode,
		BatchMaxSize: int32(p.BatchMaxSize), BatchMaxWait: p.BatchMaxWait, Status: p.Status,
	}
	if !p.CreatedAt.IsZero() {
		out.CreatedAt = p.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !p.UpdatedAt.IsZero() {
		out.UpdatedAt = p.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}
