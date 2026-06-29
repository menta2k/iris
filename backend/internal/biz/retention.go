package biz

import "time"

// ManagedTable describes an event hypertable whose retention Iris manages. The
// allowlist is fixed in code: table names reaching the data layer come only from
// here, so the dynamic SQL the retention repo builds can never include an
// arbitrary identifier.
type ManagedTable struct {
	Name string
	// TimeColumn is the hypertable's time-partitioning column, used for the
	// compression order-by and the oldest/newest-data stats.
	TimeColumn string
	// Label is a human-friendly name for the UI.
	Label string
}

// ManagedTables is the fixed set of retention-managed hypertables, in display
// order. It mirrors the hypertables created in migration 0002.
var ManagedTables = []ManagedTable{
	{Name: "mail_records", TimeColumn: "event_time", Label: "Mail logs"},
	{Name: "bounce_records", TimeColumn: "event_time", Label: "Bounces"},
	{Name: "feedback_reports", TimeColumn: "received_at", Label: "Feedback (ARF)"},
	{Name: "rspamd_filter_results", TimeColumn: "event_time", Label: "Rspamd results"},
	{Name: "queue_snapshots", TimeColumn: "observed_at", Label: "Queue snapshots"},
	{Name: "service_control_requests", TimeColumn: "requested_at", Label: "Service control"},
	{Name: "audit_entries", TimeColumn: "occurred_at", Label: "Audit log"},
}

// ManagedTableByName returns the managed-table descriptor and whether it exists.
func ManagedTableByName(name string) (ManagedTable, bool) {
	for _, t := range ManagedTables {
		if t.Name == name {
			return t, true
		}
	}
	return ManagedTable{}, false
}

// RetentionPolicy is the per-table cleanup configuration.
type RetentionPolicy struct {
	TableName string
	// RetentionDays drops chunks older than this many days. 0 = keep forever.
	RetentionDays int
	// CompressAfterDays compresses chunks older than this many days before they
	// are eligible for dropping. 0 = no compression.
	CompressAfterDays int
	Enabled           bool
	UpdatedAt         time.Time
	UpdatedBy         string
}

// Validate normalizes and checks a policy. The table must be in the allowlist,
// counts non-negative, and (when both are set) compression must happen before
// dropping — you cannot compress data you have already dropped.
func (p *RetentionPolicy) Validate() error {
	if _, ok := ManagedTableByName(p.TableName); !ok {
		return Invalid("RETENTION_TABLE_INVALID", "table %q is not retention-managed", p.TableName)
	}
	if p.RetentionDays < 0 || p.CompressAfterDays < 0 {
		return Invalid("RETENTION_DAYS_NEGATIVE", "retention and compression days must be >= 0")
	}
	if p.RetentionDays > 0 && p.CompressAfterDays > 0 && p.CompressAfterDays >= p.RetentionDays {
		return Invalid("RETENTION_COMPRESS_ORDER",
			"compress_after_days (%d) must be less than retention_days (%d)", p.CompressAfterDays, p.RetentionDays)
	}
	return nil
}

// RetentionStatus is the read-only, live disk picture for a managed table. When
// the table is not a TimescaleDB hypertable (plain PostgreSQL), Hypertable is
// false and the size fields are zero — retention cannot run.
type RetentionStatus struct {
	TableName        string
	Hypertable       bool
	ChunkCount       int
	CompressedChunks int
	TotalBytes       int64
	// CompressedBytes is the on-disk size of compressed chunks; UncompressedBytes
	// the rest. Their sum is TotalBytes.
	CompressedBytes   int64
	UncompressedBytes int64
	OldestData        *time.Time
	NewestData        *time.Time
	LastRun           *RetentionRun
}

// RetentionRun records the outcome of one cleanup pass over a table.
type RetentionRun struct {
	ID               string
	TableName        string
	StartedAt        time.Time
	FinishedAt       *time.Time
	ChunksCompressed int
	ChunksDropped    int
	BytesBefore      int64
	BytesAfter       int64
	Error            string
}

// BytesFreed is the disk reclaimed by the run (never negative for display).
func (r *RetentionRun) BytesFreed() int64 {
	if r.BytesBefore > r.BytesAfter {
		return r.BytesBefore - r.BytesAfter
	}
	return 0
}

// RetentionView bundles a table's policy with its live status for the UI.
type RetentionView struct {
	Policy RetentionPolicy
	Status RetentionStatus
	Label  string
}
