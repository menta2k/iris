package biz

import "context"

// InboundRepo is the persistence boundary for inbound Rspamd results.
type InboundRepo interface {
	CreateRspamdResult(ctx context.Context, res *RspamdFilterResult) error
	ListRspamdResults(ctx context.Context, page Page) ([]*RspamdFilterResult, error)
}

// InboundUsecase implements inbound Rspamd result handling (US5).
type InboundUsecase struct {
	repo          InboundRepo
	auditor       *Auditor
	allowInsecure bool
}

// NewInboundUsecase constructs the use case. allowInsecure is retained for
// signature compatibility with the caller wiring.
func NewInboundUsecase(repo InboundRepo, auditor *Auditor, allowInsecure bool) *InboundUsecase {
	return &InboundUsecase{repo: repo, auditor: auditor, allowInsecure: allowInsecure}
}

// ListRspamdResults returns filter results after an authorization check.
func (uc *InboundUsecase) ListRspamdResults(ctx context.Context, page Page) ([]*RspamdFilterResult, error) {
	if _, err := RequirePermission(ctx, PermRspamdRead); err != nil {
		return nil, err
	}
	return uc.repo.ListRspamdResults(ctx, page)
}

// IngestRspamdResult persists a filter result coming from the ingestion worker.
func (uc *InboundUsecase) IngestRspamdResult(ctx context.Context, res *RspamdFilterResult) error {
	return uc.repo.CreateRspamdResult(ctx, res)
}
