package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListFeedbackLoops returns feedback-loop endpoints.
func (s *Service) ListFeedbackLoops(ctx context.Context, req *adminv1.ListFeedbackLoopsRequest) (*adminv1.ListFeedbackLoopsReply, error) {
	if s.fbl == nil {
		return nil, notImplemented("ListFeedbackLoops")
	}
	page := pageFrom(req.GetPage())
	items, err := s.fbl.List(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListFeedbackLoops", err)
	}
	out := &adminv1.ListFeedbackLoopsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, f := range items {
		out.Items = append(out.Items, fblToProto(f))
	}
	return out, nil
}

// CreateFeedbackLoop creates a feedback-loop endpoint.
func (s *Service) CreateFeedbackLoop(ctx context.Context, req *adminv1.CreateFeedbackLoopRequest) (*adminv1.FeedbackLoop, error) {
	if s.fbl == nil {
		return nil, notImplemented("CreateFeedbackLoop")
	}
	out, err := s.fbl.Create(ctx, &biz.FBLEndpoint{
		Domain:          req.GetDomain(),
		FeedbackAddress: req.GetFeedbackAddress(),
		ForwardAddress:  req.GetForwardAddress(),
		Status:          req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateFeedbackLoop", err)
	}
	return fblToProto(out), nil
}

// UpdateFeedbackLoop updates an existing feedback-loop endpoint.
func (s *Service) UpdateFeedbackLoop(ctx context.Context, req *adminv1.UpdateFeedbackLoopRequest) (*adminv1.FeedbackLoop, error) {
	if s.fbl == nil {
		return nil, notImplemented("UpdateFeedbackLoop")
	}
	out, err := s.fbl.Update(ctx, req.GetId(), &biz.FBLEndpoint{
		Domain:          req.GetDomain(),
		FeedbackAddress: req.GetFeedbackAddress(),
		ForwardAddress:  req.GetForwardAddress(),
		Status:          req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateFeedbackLoop", err)
	}
	return fblToProto(out), nil
}

// DeleteFeedbackLoop removes a feedback-loop endpoint by id.
func (s *Service) DeleteFeedbackLoop(ctx context.Context, req *adminv1.DeleteFeedbackLoopRequest) (*adminv1.DeleteFeedbackLoopReply, error) {
	if s.fbl == nil {
		return nil, notImplemented("DeleteFeedbackLoop")
	}
	if err := s.fbl.Delete(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteFeedbackLoop", err)
	}
	return &adminv1.DeleteFeedbackLoopReply{}, nil
}

func fblToProto(f *biz.FBLEndpoint) *adminv1.FeedbackLoop {
	return &adminv1.FeedbackLoop{
		Id:              f.ID,
		Domain:          f.Domain,
		FeedbackAddress: f.FeedbackAddress,
		ForwardAddress:  f.ForwardAddress,
		Status:          f.Status,
	}
}
