package biz

import "context"

// RetentionRepo is the persistence + TimescaleDB boundary for retention.
type RetentionRepo interface {
	ListPolicies(ctx context.Context) ([]*RetentionPolicy, error)
	GetPolicy(ctx context.Context, table string) (*RetentionPolicy, error)
	UpdatePolicy(ctx context.Context, p *RetentionPolicy, actor string) (*RetentionPolicy, error)
	Status(ctx context.Context, table ManagedTable) (RetentionStatus, error)
	// RunRetention compresses then drops eligible chunks for one table and records
	// the run. It is a no-op (with Hypertable handling) on plain PostgreSQL.
	RunRetention(ctx context.Context, p *RetentionPolicy, table ManagedTable) (*RetentionRun, error)
}

// RetentionCommandProducer enqueues an on-demand cleanup request for the
// retention worker to execute (so the API does not block on a long compress).
type RetentionCommandProducer interface {
	EnqueueRetentionRun(ctx context.Context, table string) error
}

// RetentionUsecase manages per-table retention configuration and exposes live
// disk status. Cleanup itself runs in the retention worker.
type RetentionUsecase struct {
	repo    RetentionRepo
	trigger RetentionCommandProducer
	auditor *Auditor
}

// NewRetentionUsecase constructs the use case. trigger may be nil to disable the
// on-demand "run now" action (the daily worker still runs).
func NewRetentionUsecase(repo RetentionRepo, trigger RetentionCommandProducer, auditor *Auditor) *RetentionUsecase {
	return &RetentionUsecase{repo: repo, trigger: trigger, auditor: auditor}
}

// List returns every managed table's policy joined with its live status, in
// display order. Missing policy rows fall back to a disabled keep-forever
// default so a newly-added table still appears.
func (uc *RetentionUsecase) List(ctx context.Context) ([]*RetentionView, error) {
	if _, err := RequirePermission(ctx, PermSettingsRead); err != nil {
		return nil, err
	}
	policies, err := uc.repo.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*RetentionPolicy, len(policies))
	for _, p := range policies {
		byName[p.TableName] = p
	}
	out := make([]*RetentionView, 0, len(ManagedTables))
	for _, t := range ManagedTables {
		p := byName[t.Name]
		if p == nil {
			p = &RetentionPolicy{TableName: t.Name, Enabled: false}
		}
		status, err := uc.repo.Status(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, &RetentionView{Policy: *p, Status: status, Label: t.Label})
	}
	return out, nil
}

// Update validates and persists a table's policy.
func (uc *RetentionUsecase) Update(ctx context.Context, p *RetentionPolicy) (*RetentionPolicy, error) {
	id, err := RequirePermission(ctx, PermSettingsWrite)
	if err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.UpdatePolicy(ctx, p, id.UserID)
	if err != nil {
		uc.audit(ctx, "retention.update", p.TableName, AuditFailure, map[string]any{"table": p.TableName})
		return nil, err
	}
	uc.audit(ctx, "retention.update", out.TableName, AuditSuccess, map[string]any{
		"table": out.TableName, "retention_days": out.RetentionDays,
		"compress_after_days": out.CompressAfterDays, "enabled": out.Enabled,
	})
	return out, nil
}

// RunNow enqueues an immediate cleanup for a table (or all managed tables when
// table is empty). Requires write permission since it deletes data.
func (uc *RetentionUsecase) RunNow(ctx context.Context, table string) error {
	if _, err := RequirePermission(ctx, PermSettingsWrite); err != nil {
		return err
	}
	if table != "" {
		if _, ok := ManagedTableByName(table); !ok {
			return Invalid("RETENTION_TABLE_INVALID", "table %q is not retention-managed", table)
		}
	}
	if uc.trigger == nil {
		return Unavailable("RETENTION_RUN_UNAVAILABLE", "on-demand retention is not available")
	}
	if err := uc.trigger.EnqueueRetentionRun(ctx, table); err != nil {
		return err
	}
	uc.audit(ctx, "retention.run", table, AuditSuccess, map[string]any{"table": table})
	return nil
}

func (uc *RetentionUsecase) audit(ctx context.Context, op, target string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, "retention", target, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
