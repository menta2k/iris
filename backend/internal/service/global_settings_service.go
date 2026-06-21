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
		FBLDomain:               req.GetFblDomain(),
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
		FblDomain:               g.FBLDomain,
		UpdatedAt:               updatedAt,
		UpdatedBy:               g.UpdatedBy,
	}
}
