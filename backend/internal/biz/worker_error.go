package biz

import (
	"context"
	"strings"
	"time"
)

// WorkerError is one persisted worker error-log entry: a Warn/Error a background
// worker emitted, mirrored into the durable store by the errlog handler.
type WorkerError struct {
	ID        string
	EventTime time.Time
	Level     string // "warn" | "error"
	Worker    string
	Message   string
	Detail    map[string]any
}

// WorkerErrorFilter narrows the error-log listing. Zero-value fields match all.
type WorkerErrorFilter struct {
	Level  string
	Worker string
	From   *time.Time
	To     *time.Time
}

// NormalizeWorkerErrorFilter sanitizes and bounds the free-text filter fields.
func NormalizeWorkerErrorFilter(f WorkerErrorFilter) (WorkerErrorFilter, error) {
	f.Level = strings.ToLower(SanitizeFilter(f.Level))
	f.Worker = SanitizeFilter(f.Worker)
	if f.Level != "" && f.Level != "warn" && f.Level != "error" {
		return f, Invalid("WORKER_ERROR_LEVEL", "level must be 'warn' or 'error'")
	}
	if f.From != nil && f.To != nil && f.To.Before(*f.From) {
		return f, Invalid("WORKER_ERROR_RANGE", "to must not be before from")
	}
	return f, nil
}

// WorkerErrorRepo is the persistence boundary for reading the worker error log.
// Writes are performed by the errlog sink (data.WorkerErrorRepo), not here.
type WorkerErrorRepo interface {
	List(ctx context.Context, f WorkerErrorFilter, page Page) ([]*WorkerError, error)
}

// WorkerErrorUsecase serves the worker error-log listing API.
type WorkerErrorUsecase struct {
	repo WorkerErrorRepo
}

// NewWorkerErrorUsecase constructs the use case.
func NewWorkerErrorUsecase(repo WorkerErrorRepo) *WorkerErrorUsecase {
	return &WorkerErrorUsecase{repo: repo}
}

// List returns recent worker errors (newest first) after an authorization check.
func (uc *WorkerErrorUsecase) List(ctx context.Context, f WorkerErrorFilter, page Page) ([]*WorkerError, error) {
	if _, err := RequirePermission(ctx, PermWorkerLogsRead); err != nil {
		return nil, err
	}
	nf, err := NormalizeWorkerErrorFilter(f)
	if err != nil {
		return nil, err
	}
	return uc.repo.List(ctx, nf, page)
}
