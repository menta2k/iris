package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// EnrollMFA begins TOTP enrollment for the calling user.
func (s *Service) EnrollMFA(ctx context.Context, _ *adminv1.EnrollMFARequest) (*adminv1.EnrollMFAReply, error) {
	if s.identity == nil {
		return nil, notImplemented("EnrollMFA")
	}
	out, err := s.identity.EnrollMFA(ctx)
	if err != nil {
		return nil, s.fail(ctx, "EnrollMFA", err)
	}
	return &adminv1.EnrollMFAReply{Secret: out.Secret, OtpauthUri: out.URI}, nil
}

// ConfirmMFA verifies a TOTP code and activates MFA for the calling user.
func (s *Service) ConfirmMFA(ctx context.Context, req *adminv1.ConfirmMFARequest) (*adminv1.ConfirmMFAReply, error) {
	if s.identity == nil {
		return nil, notImplemented("ConfirmMFA")
	}
	if err := s.identity.ConfirmMFA(ctx, req.GetCode()); err != nil {
		return nil, s.fail(ctx, "ConfirmMFA", err)
	}
	return &adminv1.ConfirmMFAReply{Enrolled: true}, nil
}

// DisableMFA clears the calling user's MFA enrollment.
func (s *Service) DisableMFA(ctx context.Context, _ *adminv1.DisableMFARequest) (*adminv1.DisableMFAReply, error) {
	if s.identity == nil {
		return nil, notImplemented("DisableMFA")
	}
	if err := s.identity.DisableMFA(ctx); err != nil {
		return nil, s.fail(ctx, "DisableMFA", err)
	}
	return &adminv1.DisableMFAReply{}, nil
}

// ListUsers returns Iris users (US3).
func (s *Service) ListUsers(ctx context.Context, req *adminv1.ListUsersRequest) (*adminv1.ListUsersReply, error) {
	if s.identity == nil {
		return nil, notImplemented("ListUsers")
	}
	page := pageFrom(req.GetPage())
	items, err := s.identity.ListUsers(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListUsers", err)
	}
	out := &adminv1.ListUsersReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, u := range items {
		out.Items = append(out.Items, userToProto(u))
	}
	return out, nil
}

// CreateUser creates an Iris user (US3).
func (s *Service) CreateUser(ctx context.Context, req *adminv1.CreateUserRequest) (*adminv1.User, error) {
	if s.identity == nil {
		return nil, notImplemented("CreateUser")
	}
	out, err := s.identity.CreateUser(ctx, &biz.IrisUser{
		Email:       req.GetEmail(),
		DisplayName: req.GetDisplayName(),
		MFARequired: req.GetMfaRequired(),
		Roles:       req.GetRoles(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateUser", err)
	}
	return userToProto(out), nil
}

// UpdateUser updates an existing Iris user (US3).
func (s *Service) UpdateUser(ctx context.Context, req *adminv1.UpdateUserRequest) (*adminv1.User, error) {
	if s.identity == nil {
		return nil, notImplemented("UpdateUser")
	}
	out, err := s.identity.UpdateUser(ctx, req.GetId(), &biz.IrisUser{
		DisplayName: req.GetDisplayName(),
		Status:      req.GetStatus(),
		MFARequired: req.GetMfaRequired(),
		Roles:       req.GetRoles(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateUser", err)
	}
	return userToProto(out), nil
}

// ListAuditEntries returns audit-log entries (US3).
func (s *Service) ListAuditEntries(ctx context.Context, req *adminv1.ListAuditEntriesRequest) (*adminv1.ListAuditEntriesReply, error) {
	if s.identity == nil {
		return nil, notImplemented("ListAuditEntries")
	}
	page := pageFrom(req.GetPage())
	items, err := s.identity.ListAuditEntries(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListAuditEntries", err)
	}
	out := &adminv1.ListAuditEntriesReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, e := range items {
		out.Items = append(out.Items, &adminv1.AuditEntry{
			Id: e.ID, OccurredAt: e.OccurredAt, ActorUserId: e.ActorUserID, Operation: e.Operation,
			TargetType: e.TargetType, TargetId: e.TargetID, Outcome: e.Outcome, IpAddress: e.IPAddress,
		})
	}
	return out, nil
}

func userToProto(u *biz.IrisUser) *adminv1.User {
	return &adminv1.User{
		Id: u.ID, Email: u.Email, DisplayName: u.DisplayName, Status: u.Status,
		MfaRequired: u.MFARequired, Roles: u.Roles,
	}
}
