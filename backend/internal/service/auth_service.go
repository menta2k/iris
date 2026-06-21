package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// Login exchanges email + password for a session token (US3 auth).
func (s *Service) Login(ctx context.Context, req *adminv1.LoginRequest) (*adminv1.LoginReply, error) {
	if s.auth == nil {
		return nil, notImplemented("Login")
	}
	res, err := s.auth.Login(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, s.fail(ctx, "Login", err)
	}
	return loginReply(res), nil
}

// VerifyMFA validates a TOTP code and upgrades the session to fully authenticated.
func (s *Service) VerifyMFA(ctx context.Context, req *adminv1.VerifyMFARequest) (*adminv1.LoginReply, error) {
	if s.auth == nil {
		return nil, notImplemented("VerifyMFA")
	}
	res, err := s.auth.VerifyMFA(ctx, req.GetCode())
	if err != nil {
		return nil, s.fail(ctx, "VerifyMFA", err)
	}
	return loginReply(res), nil
}

// CurrentUser returns the calling user's profile and effective permissions.
func (s *Service) CurrentUser(ctx context.Context, _ *adminv1.CurrentUserRequest) (*adminv1.CurrentUserReply, error) {
	if s.auth == nil {
		return nil, notImplemented("CurrentUser")
	}
	user, perms, err := s.auth.CurrentUser(ctx)
	if err != nil {
		return nil, s.fail(ctx, "CurrentUser", err)
	}
	return &adminv1.CurrentUserReply{User: userToProto(user), Permissions: perms}, nil
}

// ChangePassword updates the calling user's own password.
func (s *Service) ChangePassword(ctx context.Context, req *adminv1.ChangePasswordRequest) (*adminv1.ChangePasswordReply, error) {
	if s.auth == nil {
		return nil, notImplemented("ChangePassword")
	}
	if err := s.auth.ChangePassword(ctx, req.GetCurrentPassword(), req.GetNewPassword()); err != nil {
		return nil, s.fail(ctx, "ChangePassword", err)
	}
	return &adminv1.ChangePasswordReply{}, nil
}

// Logout is a no-op for stateless tokens; the client discards its token. The
// call is audited for traceability.
func (s *Service) Logout(ctx context.Context, _ *adminv1.LogoutRequest) (*adminv1.LogoutReply, error) {
	if s.auditor != nil {
		_ = s.auditor.Record(ctx, "auth.logout", "auth", "", biz.AuditSuccess, nil)
	}
	return &adminv1.LogoutReply{}, nil
}

func loginReply(res *biz.LoginResult) *adminv1.LoginReply {
	return &adminv1.LoginReply{
		Token:       res.Token,
		Status:      res.Status,
		User:        userToProto(res.User),
		Permissions: res.Permissions,
	}
}
