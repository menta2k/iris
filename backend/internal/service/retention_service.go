package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListRetentionPolicies returns each managed table's policy + live disk status.
func (s *Service) ListRetentionPolicies(ctx context.Context, _ *adminv1.ListRetentionPoliciesRequest) (*adminv1.ListRetentionPoliciesReply, error) {
	if s.retention == nil {
		return nil, notImplemented("ListRetentionPolicies")
	}
	views, err := s.retention.List(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListRetentionPolicies", err)
	}
	out := &adminv1.ListRetentionPoliciesReply{}
	for _, v := range views {
		out.Items = append(out.Items, retentionViewToProto(v))
	}
	return out, nil
}

// UpdateRetentionPolicy updates one table's retention/compression configuration.
func (s *Service) UpdateRetentionPolicy(ctx context.Context, req *adminv1.UpdateRetentionPolicyRequest) (*adminv1.RetentionPolicy, error) {
	if s.retention == nil {
		return nil, notImplemented("UpdateRetentionPolicy")
	}
	out, err := s.retention.Update(ctx, &biz.RetentionPolicy{
		TableName:         req.GetTableName(),
		RetentionDays:     int(req.GetRetentionDays()),
		CompressAfterDays: int(req.GetCompressAfterDays()),
		Enabled:           req.GetEnabled(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateRetentionPolicy", err)
	}
	return retentionPolicyToProto(out), nil
}

// RunRetention enqueues an on-demand cleanup (empty table = all).
func (s *Service) RunRetention(ctx context.Context, req *adminv1.RunRetentionRequest) (*adminv1.RunRetentionReply, error) {
	if s.retention == nil {
		return nil, notImplemented("RunRetention")
	}
	if err := s.retention.RunNow(ctx, req.GetTableName()); err != nil {
		return nil, s.fail(ctx, "RunRetention", err)
	}
	return &adminv1.RunRetentionReply{Ok: true}, nil
}

func retentionPolicyToProto(p *biz.RetentionPolicy) *adminv1.RetentionPolicy {
	return &adminv1.RetentionPolicy{
		TableName:         p.TableName,
		RetentionDays:     int32(p.RetentionDays),
		CompressAfterDays: int32(p.CompressAfterDays),
		Enabled:           p.Enabled,
		UpdatedAt:         rfc3339(&p.UpdatedAt),
		UpdatedBy:         p.UpdatedBy,
	}
}

func retentionViewToProto(v *biz.RetentionView) *adminv1.RetentionView {
	out := &adminv1.RetentionView{
		Policy:            retentionPolicyToProto(&v.Policy),
		Label:             v.Label,
		Hypertable:        v.Status.Hypertable,
		ChunkCount:        int64(v.Status.ChunkCount),
		CompressedChunks:  int64(v.Status.CompressedChunks),
		TotalBytes:        v.Status.TotalBytes,
		CompressedBytes:   v.Status.CompressedBytes,
		UncompressedBytes: v.Status.UncompressedBytes,
		OldestData:        rfc3339(v.Status.OldestData),
		NewestData:        rfc3339(v.Status.NewestData),
	}
	if r := v.Status.LastRun; r != nil {
		out.LastRun = &adminv1.RetentionRun{
			Id:               r.ID,
			TableName:        r.TableName,
			StartedAt:        rfc3339(&r.StartedAt),
			FinishedAt:       rfc3339(r.FinishedAt),
			ChunksCompressed: int32(r.ChunksCompressed),
			ChunksDropped:    int32(r.ChunksDropped),
			BytesBefore:      r.BytesBefore,
			BytesAfter:       r.BytesAfter,
			Error:            r.Error,
		}
	}
	return out
}

// rfc3339 formats an optional timestamp, empty when nil/zero.
func rfc3339(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
