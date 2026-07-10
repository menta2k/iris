package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// GetGlobalSettings returns the deployment-level policy settings.
func (s *Service) GetGlobalSettings(ctx context.Context, req *adminv1.GetGlobalSettingsRequest) (*adminv1.GlobalSettings, error) {
	if s.settings == nil {
		return nil, notImplemented("GetGlobalSettings")
	}
	out, err := s.settings.Get(ctx)
	if err != nil {
		return nil, s.fail(ctx, "GetGlobalSettings", err)
	}
	return settingsToProto(out), nil
}

// UpdateGlobalSettings validates and persists the settings; the next config
// generate/apply picks up the change.
func (s *Service) UpdateGlobalSettings(ctx context.Context, req *adminv1.UpdateGlobalSettingsRequest) (*adminv1.GlobalSettings, error) {
	if s.settings == nil {
		return nil, notImplemented("UpdateGlobalSettings")
	}
	out, err := s.settings.Update(ctx, &biz.GlobalSettings{
		RspamdMode:              req.GetRspamdMode(),
		RspamdURL:               req.GetRspamdUrl(),
		EgressEHLODomain:        req.GetEgressEhloDomain(),
		LogStreamRedisURL:       req.GetLogStreamRedisUrl(),
		EsmtpListen:             req.GetEsmtpListen(),
		HTTPListen:              req.GetHttpListen(),
		EgressRetryInterval:     req.GetEgressRetryInterval(),
		EgressMaxRetryInterval:  req.GetEgressMaxRetryInterval(),
		EgressMaxAge:            req.GetEgressMaxAge(),
		BounceDomain:            req.GetBounceDomain(),
		AutoSuppressHardBounces: req.GetAutoSuppressHardBounces(),
		SoftBounceThreshold:     int(req.GetSoftBounceThreshold()),
		SuppressionTTL:          req.GetSuppressionTtl(),
		DMARCReportEmail:        req.GetDmarcReportEmail(),
		AdminHTTPAddr:           req.GetAdminHttpAddr(),
		AdminTLSEnabled:         req.GetAdminTlsEnabled(),
		AdminTLSCertDomain:      req.GetAdminTlsCertDomain(),
		AcmeRenewInterval:       req.GetAcmeRenewInterval(),
		AcmeRenewBefore:         req.GetAcmeRenewBefore(),
		PrometheusURL:           req.GetPrometheusUrl(),
		FBLRequireVerification:  req.GetFblRequireVerification(),
		InboundMaildirBasePath:  req.GetInboundMaildirBasePath(),
		BounceDomainTemplate:    req.GetBounceDomainTemplate(),
		ClassifySubjects:        req.GetClassifySubjects(),
		ClassifyModel:           req.GetClassifyModel(),
		ClassifyThreshold:       req.GetClassifyThreshold(),
		ClassifyAPIBase:         req.GetClassifyApiBase(),
		PinEgressPerMessage:     req.GetPinEgressPerMessage(),
		InjectionEnabled:        req.GetInjectionEnabled(),
		InjectionListenAddr:     req.GetInjectionListenAddr(),
		InjectionPath:           req.GetInjectionPath(),
		InjectionTLSEnabled:     req.GetInjectionTlsEnabled(),
		InjectionTLSCertDomain:  req.GetInjectionTlsCertDomain(),

		MonitoringFrom:              req.GetMonitoringFrom(),
		MonitoringReconcileLookback: req.GetMonitoringReconcileLookback(),
		MonitoringFetchTimeout:      req.GetMonitoringFetchTimeout(),
		MonitoringFetchGiveUp:       req.GetMonitoringFetchGiveup(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateGlobalSettings", err)
	}
	return settingsToProto(out), nil
}

func settingsToProto(g *biz.GlobalSettings) *adminv1.GlobalSettings {
	updatedAt := ""
	if !g.UpdatedAt.IsZero() {
		updatedAt = g.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return &adminv1.GlobalSettings{
		RspamdMode:              g.RspamdMode,
		RspamdUrl:               g.RspamdURL,
		EgressEhloDomain:        g.EgressEHLODomain,
		LogStreamRedisUrl:       g.LogStreamRedisURL,
		EsmtpListen:             g.EsmtpListen,
		HttpListen:              g.HTTPListen,
		EgressRetryInterval:     g.EgressRetryInterval,
		EgressMaxRetryInterval:  g.EgressMaxRetryInterval,
		EgressMaxAge:            g.EgressMaxAge,
		BounceDomain:            g.BounceDomain,
		AutoSuppressHardBounces: g.AutoSuppressHardBounces,
		SoftBounceThreshold:     int32(g.SoftBounceThreshold),
		SuppressionTtl:          g.SuppressionTTL,
		DmarcReportEmail:        g.DMARCReportEmail,
		AdminHttpAddr:           g.AdminHTTPAddr,
		AdminTlsEnabled:         g.AdminTLSEnabled,
		AdminTlsCertDomain:      g.AdminTLSCertDomain,
		AcmeRenewInterval:       g.AcmeRenewInterval,
		AcmeRenewBefore:         g.AcmeRenewBefore,
		PrometheusUrl:           g.PrometheusURL,
		FblRequireVerification:  g.FBLRequireVerification,
		InboundMaildirBasePath:  g.InboundMaildirBasePath,
		BounceDomainTemplate:    g.BounceDomainTemplate,
		ClassifySubjects:        g.ClassifySubjects,
		ClassifyModel:           g.ClassifyModel,
		ClassifyThreshold:       g.ClassifyThreshold,
		ClassifyApiBase:         g.ClassifyAPIBase,
		PinEgressPerMessage:     g.PinEgressPerMessage,
		InjectionEnabled:        g.InjectionEnabled,
		InjectionListenAddr:     g.InjectionListenAddr,
		InjectionPath:           g.InjectionPath,
		InjectionTlsEnabled:     g.InjectionTLSEnabled,
		InjectionTlsCertDomain:  g.InjectionTLSCertDomain,

		MonitoringFrom:              g.MonitoringFrom,
		MonitoringReconcileLookback: g.MonitoringReconcileLookback,
		MonitoringFetchTimeout:      g.MonitoringFetchTimeout,
		MonitoringFetchGiveup:       g.MonitoringFetchGiveUp,

		UpdatedAt: updatedAt,
		UpdatedBy:               g.UpdatedBy,
	}
}
