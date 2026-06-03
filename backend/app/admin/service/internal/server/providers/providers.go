// Package providers exposes the wire ProviderSet for transport servers and
// the middleware chain. Kept separate so cmd/server/wire.go has a single
// import path per layer (matching the go-wind-admin convention).
package providers

import (
	"github.com/go-kratos/kratos/v2/middleware"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/google/wire"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data"
	"github.com/menta2k/iris/backend/app/admin/service/internal/server"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
	authmw "github.com/menta2k/iris/backend/pkg/middleware/audit"
	identitymw "github.com/menta2k/iris/backend/pkg/middleware/auth"
)

// NewMiddlewares wires the kratos middleware chain. The auth middleware is
// not mounted yet (login/refresh are public and the enforcement selector
// hasn't been wired); we still pull identity from context via
// identitymw.IdentityFunc so that once auth is mounted, audit rows
// automatically pick up actor info without further changes here.
//
// IMPORTANT: only operations that flow through the kratos service-stub
// chain (i.e. gRPC RPCs and HTTP routes registered by generated stubs) are
// observed by this middleware. The hand-rolled HTTP handlers in registrar.go
// / registrar_admin.go bypass it — converting those to proto-driven HTTP
// (via google.api.http annotations) is the next step to get full coverage.
func NewMiddlewares(writer *data.AuditWriter) []middleware.Middleware {
	return []middleware.Middleware{
		authmw.Server(authmw.Options{
			Write:    writer.Write,
			Identity: identitymw.IdentityFunc,
		}),
	}
}

// RegisterServers binds every service onto both transports. Wire calls this
// for its side-effect; the returned `_` keeps it part of the build graph.
//
// Returning a marker type lets wire treat this as a real provider so it gets
// invoked exactly once during initApp construction.
type Registered struct{}

func RegisterServers(
	gs *kratosgrpc.Server,
	hs *kratoshttp.Server,
	auth *service.AuthenticationGRPC,
	users *service.UserService,
	audit *service.AuditService,
	queues *service.QueueService,
	suppressions *service.SuppressionService,
	vmtas *service.VirtualMtaService,
	routing *service.RoutingService,
	dkim *service.DkimService,
	feedback *service.FeedbackService,
	logs *service.LogService,
	policy *service.PolicyService,
	mailClasses *service.MailClassService,
	vmtaGroups *service.VmtaGroupService,
	dashboard *service.DashboardService,
	dsns *service.DsnService,
	gsvc *service.GlobalSettingsService,
	listeners *service.ListenerService,
	acme *service.AcmeService,
	loginPolicies *service.LoginPolicyService,
	writer *data.AuditWriter,
) Registered {
	server.RegisterServices(gs, hs, auth, users, audit,
		queues, suppressions, vmtas, routing, dkim, feedback, logs, policy,
		mailClasses, vmtaGroups, dashboard, dsns, gsvc, listeners, acme, loginPolicies, writer.Write)
	return Registered{}
}

// ProviderSet wires the transport layer + service registration.
var ProviderSet = wire.NewSet(
	NewMiddlewares,
	server.NewGRPCServer,
	server.NewHTTPServer,
	server.NewLogstreamServer,
	server.NewDsnstreamServer,
	server.NewAcmeChallengeServer,
	server.NewAcmeRenewerServer,
	server.NewGeoIPUpdaterServer,
	server.NewHTTPSServer,
	server.NewSuppressionResyncServer,
	server.NewMetricsServer,
	RegisterServers,
)
