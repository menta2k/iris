// AuthenticationGRPC adapts the transport-agnostic AuthenticationService to
// the proto-generated AuthenticationServiceServer interface. It is the only
// place where proto types ↔ Go service types translation lives.
package service

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	authenticationpb "github.com/menta2k/iris/backend/api/gen/go/authentication/service/v1"
)

// AuthenticationGRPC implements authenticationpb.AuthenticationServiceServer.
type AuthenticationGRPC struct {
	authenticationpb.UnimplementedAuthenticationServiceServer
	svc *AuthenticationService
}

// NewAuthenticationGRPC constructs the adapter.
func NewAuthenticationGRPC(svc *AuthenticationService) *AuthenticationGRPC {
	return &AuthenticationGRPC{svc: svc}
}

// Login proxies to AuthenticationService.Login. Client IP is best-effort —
// audit middleware extracts the canonical X-Forwarded-For elsewhere; this
// adapter only forwards the empty string until that wiring is in place.
func (a *AuthenticationGRPC) Login(ctx context.Context, req *authenticationpb.LoginRequest) (*authenticationpb.LoginResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "login: request required")
	}
	resp, err := a.svc.Login(ctx, &LoginRequest{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
	}, clientIPFromCtx(ctx))
	if err != nil {
		return nil, mapAuthError(err)
	}
	return loginResponseProto(resp), nil
}

// RefreshToken proxies to AuthenticationService.RefreshToken.
func (a *AuthenticationGRPC) RefreshToken(ctx context.Context, req *authenticationpb.RefreshTokenRequest) (*authenticationpb.LoginResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "refresh: request required")
	}
	resp, err := a.svc.RefreshToken(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, mapAuthError(err)
	}
	return loginResponseProto(resp), nil
}

// Logout currently invalidates client-side state only; a future enhancement
// is a server-side refresh-token blocklist (Redis-backed).
func (a *AuthenticationGRPC) Logout(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// Whoami returns the caller identity from the auth-middleware context.
// Until the auth middleware is mounted, returns a zero payload.
func (a *AuthenticationGRPC) Whoami(ctx context.Context, _ *emptypb.Empty) (*authenticationpb.UserTokenPayload, error) {
	// The kratos auth middleware (pkg/middleware/auth) will inject identity
	// once selectors are configured. Returning the zero payload keeps the
	// endpoint functional during bootstrap.
	return &authenticationpb.UserTokenPayload{
		IssuedAt:  timestamppb.Now(),
		ExpiresAt: timestamppb.Now(),
	}, nil
}

// loginResponseProto converts the service-layer response into the proto.
func loginResponseProto(r *LoginResponse) *authenticationpb.LoginResponse {
	if r == nil {
		return nil
	}
	return &authenticationpb.LoginResponse{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		ExpiresIn:    r.ExpiresIn,
		User: &authenticationpb.UserTokenPayload{
			UserId:    r.UserID,
			Username:  r.Username,
			Roles:     r.Roles,
			IssuedAt:  timestamppb.Now(),
			ExpiresAt: timestamppb.New(timestamppb.Now().AsTime().Add(0)),
		},
	}
}

// mapAuthError translates internal sentinels to gRPC status codes that
// mirror what the HTTP gateway will surface to the SPA.
func mapAuthError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, "invalid credentials")
	case errors.Is(err, ErrAccountInactive):
		return status.Error(codes.PermissionDenied, "account inactive")
	case errors.Is(err, ErrAccountLocked):
		return status.Error(codes.ResourceExhausted, "account locked")
	case errors.Is(err, ErrLoginBlocked):
		return status.Error(codes.PermissionDenied, "login blocked by security policy")
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
