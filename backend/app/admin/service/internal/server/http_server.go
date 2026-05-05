package server

import (
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport/http"

	"github.com/tx7do/kratos-bootstrap/bootstrap"
	bootstraprpc "github.com/tx7do/kratos-bootstrap/rpc"
)

// NewHTTPServer constructs the HTTP transport.
func NewHTTPServer(ctx *bootstrap.Context, ms []middleware.Middleware) (*http.Server, error) {
	return bootstraprpc.CreateRestServer(ctx.GetConfig(), ms...)
}
