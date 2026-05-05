package service

import (
	"context"
	"fmt"
	"time"
)

// AuditStore is the read interface AuditService depends on.
type AuditStore interface {
	List(ctx context.Context, in AuditListInput) ([]AuditRow, uint32, error)
}

// AuditListInput captures filtering and pagination for GET /v1/audit.
//
// Limit is clamped server-side; an unset Limit becomes 200. Since/Until are
// inclusive bounds on the `at` column.
type AuditListInput struct {
	Operation   string
	ActorUserID uint32
	Since       time.Time
	Until       time.Time
	Limit       int
	Offset      int
}

// AuditRow is the API-shape of an audit_entry row. Request/response JSON is
// intentionally omitted from the row — those payloads are large and only
// surfaced by a future Inspect endpoint.
type AuditRow struct {
	ID            int64
	At            time.Time
	Operation     string
	ResourceType  string
	ResourceID    string
	ActorUserID   uint32
	ActorUsername string
	ClientIP      string
	UserAgent     string
	RequestID     string
	StatusCode    int32
	StatusMessage string
	DurationMS    int64
}

// AuditService is the read-only service for audit entries.
type AuditService struct {
	store AuditStore
}

// NewAuditService constructs the service.
func NewAuditService(store AuditStore) *AuditService { return &AuditService{store: store} }

const (
	auditDefaultLimit = 200
	auditMaxLimit     = 1000
)

// List paginates and clamps inputs.
func (s *AuditService) List(ctx context.Context, in AuditListInput) ([]AuditRow, uint32, error) {
	if in.Limit <= 0 {
		in.Limit = auditDefaultLimit
	}
	if in.Limit > auditMaxLimit {
		in.Limit = auditMaxLimit
	}
	if in.Offset < 0 {
		in.Offset = 0
	}
	rows, total, err := s.store.List(ctx, in)
	if err != nil {
		return nil, 0, fmt.Errorf("audit: list: %w", err)
	}
	return rows, total, nil
}
