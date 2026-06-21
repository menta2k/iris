package service

import (
	"context"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

// GenerateKumoConfig renders the current configuration into KumoMTA policy
// without applying it (preview).
func (s *Service) GenerateKumoConfig(ctx context.Context, req *adminv1.GenerateKumoConfigRequest) (*adminv1.KumoConfig, error) {
	if s.kumoConfig == nil {
		return nil, notImplemented("GenerateKumoConfig")
	}
	r, err := s.kumoConfig.Generate(ctx)
	if err != nil {
		return nil, s.fail(ctx, "GenerateKumoConfig", err)
	}
	return &adminv1.KumoConfig{
		Content:          r.Content,
		VmtaCount:        int32(r.VMTACount),
		PoolCount:        int32(r.PoolCount),
		RouteCount:       int32(r.RouteCount),
		DkimCount:        int32(r.DKIMCount),
		SuppressionCount: int32(r.SuppressionCount),
		Checksum:         r.Checksum,
		Valid:            r.Valid,
		LintIssues:       r.LintIssues,
	}, nil
}

// KumoConfigStatus reports whether the current config has drifted from the last
// applied policy.
func (s *Service) KumoConfigStatus(ctx context.Context, _ *adminv1.KumoConfigStatusRequest) (*adminv1.KumoConfigStatusReply, error) {
	if s.kumoConfig == nil {
		return nil, notImplemented("KumoConfigStatus")
	}
	st, err := s.kumoConfig.Status(ctx)
	if err != nil {
		return nil, s.fail(ctx, "KumoConfigStatus", err)
	}
	appliedAt := ""
	if st.AppliedAt != nil {
		appliedAt = st.AppliedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return &adminv1.KumoConfigStatusReply{
		Drift:           st.Drift,
		NeverApplied:    st.NeverApplied,
		CurrentChecksum: st.CurrentChecksum,
		AppliedChecksum: st.AppliedChecksum,
		AppliedAt:       appliedAt,
		RestartRequired: st.RestartRequired,
	}, nil
}

// ApplyKumoConfig renders, writes, and reloads the KumoMTA configuration.
func (s *Service) ApplyKumoConfig(ctx context.Context, req *adminv1.ApplyKumoConfigRequest) (*adminv1.ApplyKumoConfigReply, error) {
	if s.kumoConfig == nil {
		return nil, notImplemented("ApplyKumoConfig")
	}
	res, err := s.kumoConfig.Apply(ctx, req.GetConfirmationId())
	if err != nil {
		return nil, s.fail(ctx, "ApplyKumoConfig", err)
	}
	return &adminv1.ApplyKumoConfigReply{
		RequestId:     res.RequestID,
		Status:        res.Status,
		Checksum:      res.Checksum,
		AppliedPath:   res.AppliedPath,
		ResultSummary: res.ResultSummary,
		Restarted:     res.Restarted,
	}, nil
}
