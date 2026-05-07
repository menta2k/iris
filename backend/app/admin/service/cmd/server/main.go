// Package main is the entrypoint for the kumo-ui admin service.
//
// The binary listens on both gRPC and HTTP (via a Kratos transport) and is
// orchestrated by `tx7do/kratos-bootstrap`, which loads layered configuration
// (server, data, auth, logger, kumo) from the directory specified by the
// `-conf` flag and then dependency-injects providers wired in `wire_gen.go`.
package main

import (
	"context"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"

	conf "github.com/tx7do/kratos-bootstrap/api/gen/go/conf/v1"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"github.com/menta2k/iris/backend/app/admin/service/internal/server"
	serverProviders "github.com/menta2k/iris/backend/app/admin/service/internal/server/providers"
	"github.com/menta2k/iris/backend/pkg/serviceid"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "0.1.0"

// newApp wires together the transport servers and the kratos.App lifecycle.
//
// `_ serverProviders.Registered` is a marker dependency — wire constructs it
// for its side effect (RegisterServers binds the service implementations to
// both servers) and threads it through here so the call happens exactly
// once before the kratos.App spins up.
func newApp(
	ctx *bootstrap.Context,
	hs *http.Server,
	gs *grpc.Server,
	ls *server.LogstreamServer,
	ds *server.DsnstreamServer,
	acs *server.AcmeChallengeServer,
	hss *server.HTTPSServer,
	srs *server.SuppressionResyncServer,
	ms *server.MetricsServer,
	_ serverProviders.Registered,
) *kratos.App {
	// All these servers are no-ops when their respective configuration
	// is unset (Redis / BounceDomain / IRIS_ACME_HTTP_BIND /
	// global_settings.https_listen / metrics). Either way they join
	// the kratos lifecycle so clean shutdown waits on in-flight work.
	return bootstrap.NewApp(ctx, hs, gs, ls, ds, acs, hss, srs, ms)
}

func runApp() error {
	ctx := bootstrap.NewContext(
		context.Background(),
		&conf.AppInfo{
			Project: serviceid.ProjectName,
			AppId:   serviceid.AdminService,
			Version: version,
		},
	)
	return bootstrap.RunApp(ctx, initApp)
}

func main() {
	if err := runApp(); err != nil {
		panic(err)
	}
}
