package service

import (
	"context"
	"log/slog"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// Service implements the generated IrisAdminService for both HTTP and gRPC.
// It delegates to per-domain use cases. Embedding the generated Unimplemented
// server keeps gRPC forward-compatible if new RPCs are added to the proto.
//
// Use-case dependencies are added to this struct as each user story is wired in
// (see the per-domain *_service.go files in this package).
type Service struct {
	adminv1.UnimplementedIrisAdminServiceServer

	log     *slog.Logger
	auditor *biz.Auditor

	outbound     *biz.OutboundConfigUsecase
	mailOps      *biz.MailOpsUsecase
	identity     *biz.IdentityUsecase
	auth         *biz.AuthUsecase
	domainSafety *biz.DomainSafetyUsecase
	inbound      *biz.InboundUsecase
	dashboard    *biz.DashboardUsecase
	kumoConfig   *biz.KumoConfigUsecase
	settings     *biz.GlobalSettingsUsecase
	acme         *biz.AcmeUsecase
	domainCheck  *biz.DomainCheckUsecase
}

// Deps bundles the use cases the service delegates to. Fields may be nil for
// user stories that are not yet wired; their RPCs return NOT_IMPLEMENTED.
type Deps struct {
	Log          *slog.Logger
	Auditor      *biz.Auditor
	Outbound     *biz.OutboundConfigUsecase
	MailOps      *biz.MailOpsUsecase
	Identity     *biz.IdentityUsecase
	Auth         *biz.AuthUsecase
	DomainSafety *biz.DomainSafetyUsecase
	Inbound      *biz.InboundUsecase
	Dashboard    *biz.DashboardUsecase
	KumoConfig   *biz.KumoConfigUsecase
	Settings     *biz.GlobalSettingsUsecase
	Acme         *biz.AcmeUsecase
	DomainCheck  *biz.DomainCheckUsecase
}

// NewService constructs the admin API service.
func NewService(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		log:          log,
		auditor:      d.Auditor,
		outbound:     d.Outbound,
		mailOps:      d.MailOps,
		identity:     d.Identity,
		auth:         d.Auth,
		domainSafety: d.DomainSafety,
		inbound:      d.Inbound,
		dashboard:    d.Dashboard,
		kumoConfig:   d.KumoConfig,
		settings:     d.Settings,
		acme:         d.Acme,
		domainCheck:  d.DomainCheck,
	}
}

var _ adminv1.IrisAdminServiceHTTPServer = (*Service)(nil)

// notImplemented is returned by RPCs whose user story has not yet been wired.
func notImplemented(name string) error {
	return mapError(&biz.DomainError{
		Kind:    biz.KindUnavailable,
		Reason:  "NOT_IMPLEMENTED",
		Message: name + " is not implemented yet",
	})
}

// pageFrom converts a proto PageRequest into a bounded, validated biz.Page.
func pageFrom(p *adminv1.PageRequest) biz.Page {
	if p == nil {
		return biz.NormalizePage(0, "")
	}
	return biz.NormalizePage(int(p.GetPageSize()), p.GetPageToken())
}

// fail logs unexpected failures (internal domain errors and any non-domain
// error, both of which surface as 500s) and maps the error to a transport error.
func (s *Service) fail(ctx context.Context, op string, err error) error {
	if de, ok := biz.AsDomainError(err); ok {
		if de.Kind == biz.KindInternal {
			s.log.ErrorContext(ctx, "operation failed", "op", op, "error", de.Error())
		}
	} else {
		s.log.ErrorContext(ctx, "operation failed", "op", op, "error", err.Error())
	}
	return mapError(err)
}
