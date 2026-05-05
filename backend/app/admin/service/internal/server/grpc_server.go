// Package server holds the kratos transport server constructors.
//
// Wire pulls these via providers.ProviderSet in cmd/server/wire.go. Each
// constructor takes the bootstrap context (carrying parsed config) plus the
// middlewares the service should mount; the generated init function chains
// providers together.
package server

import (
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport/grpc"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bootstraprpc "github.com/tx7do/kratos-bootstrap/rpc"
)

// NewGRPCServer constructs the gRPC transport from the bootstrap config.
//
// Service registration (kratos generated `RegisterFooServiceServer` calls)
// is performed by the wire-injected closures in cmd/server. Keeping this
// constructor service-free isolates transport configuration from service
// wiring and matches the layout of go-wind-admin.
func NewGRPCServer(ctx *bootstrap.Context, ms []middleware.Middleware) (*grpc.Server, error) {
	return bootstraprpc.CreateGrpcServer(ctx.GetConfig(), ms...)
}
