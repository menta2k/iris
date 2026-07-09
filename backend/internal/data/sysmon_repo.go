package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// MonitorRepo persists the singleton monitor settings row and the alert history.
type MonitorRepo struct {
	db *DB
}

// NewMonitorRepo constructs the repository.
func NewMonitorRepo(db *DB) *MonitorRepo { return &MonitorRepo{db: db} }

var _ biz.MonitorRepo = (*MonitorRepo)(nil)

const monitorCols = `enabled, cpu_threshold, mem_threshold, disk_threshold, disk_paths,
	notify_emails, from_email, smtp_host, cooldown_minutes, sample_seconds`

func scanMonitor(row interface {
	Scan(dest ...any) error
}) (*biz.MonitorSettings, error) {
	s := &biz.MonitorSettings{}
	if err := row.Scan(&s.Enabled, &s.CPUThreshold, &s.MemThreshold, &s.DiskThreshold,
		&s.DiskPaths, &s.NotifyEmails, &s.FromEmail, &s.SMTPHost,
		&s.CooldownMinutes, &s.SampleSeconds); err != nil {
		return nil, err
	}
	return s, nil
}

// GetMonitorSettings returns the singleton settings row.
func (r *MonitorRepo) GetMonitorSettings(ctx context.Context) (*biz.MonitorSettings, error) {
	s, err := scanMonitor(r.db.Pool.QueryRow(ctx,
		`SELECT `+monitorCols+` FROM monitor_settings WHERE id = 1`))
	if err != nil {
		return nil, fmt.Errorf("get monitor settings: %w", err)
	}
	return s, nil
}

// UpdateMonitorSettings writes every field on the singleton row.
func (r *MonitorRepo) UpdateMonitorSettings(ctx context.Context, in *biz.MonitorSettings) (*biz.MonitorSettings, error) {
	s, err := scanMonitor(r.db.Pool.QueryRow(ctx, `
		UPDATE monitor_settings SET
			enabled = $1, cpu_threshold = $2, mem_threshold = $3, disk_threshold = $4,
			disk_paths = $5, notify_emails = $6, from_email = $7, smtp_host = $8,
			cooldown_minutes = $9, sample_seconds = $10, updated_at = now()
		WHERE id = 1
		RETURNING `+monitorCols,
		in.Enabled, in.CPUThreshold, in.MemThreshold, in.DiskThreshold,
		in.DiskPaths, in.NotifyEmails, in.FromEmail, in.SMTPHost,
		in.CooldownMinutes, in.SampleSeconds))
	if err != nil {
		return nil, fmt.Errorf("update monitor settings: %w", err)
	}
	return s, nil
}

// InsertMonitorAlert records a threshold transition.
func (r *MonitorRepo) InsertMonitorAlert(ctx context.Context, a *biz.MonitorAlert) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO monitor_alerts (resource, detail, level, value, threshold, message, notified)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		a.Resource, a.Detail, a.Level, a.Value, a.Threshold, a.Message, a.Notified)
	if err != nil {
		return fmt.Errorf("insert monitor alert: %w", err)
	}
	return nil
}

// RecentMonitorAlerts returns the newest alert transitions, bounded by limit.
func (r *MonitorRepo) RecentMonitorAlerts(ctx context.Context, limit int) ([]*biz.MonitorAlert, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, resource, detail, level, value, threshold, message, notified, created_at
		FROM monitor_alerts ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("query monitor alerts: %w", err)
	}
	defer rows.Close()
	var out []*biz.MonitorAlert
	for rows.Next() {
		a := &biz.MonitorAlert{}
		if err := rows.Scan(&a.ID, &a.Resource, &a.Detail, &a.Level, &a.Value,
			&a.Threshold, &a.Message, &a.Notified, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan monitor alert: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
