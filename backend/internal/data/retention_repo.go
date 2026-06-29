package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
)

// RetentionRepo persists per-table retention policies and drives TimescaleDB
// chunk compression/dropping. All table names it interpolates into dynamic SQL
// come from biz.ManagedTables (a fixed allowlist), never from user input.
type RetentionRepo struct {
	db *DB
}

// NewRetentionRepo constructs the repository.
func NewRetentionRepo(db *DB) *RetentionRepo { return &RetentionRepo{db: db} }

var _ biz.RetentionRepo = (*RetentionRepo)(nil)

// ListPolicies returns all stored policies.
func (r *RetentionRepo) ListPolicies(ctx context.Context) ([]*biz.RetentionPolicy, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT table_name, retention_days, compress_after_days, enabled, updated_at, updated_by
		FROM retention_policies`)
	if err != nil {
		return nil, fmt.Errorf("list retention policies: %w", err)
	}
	defer rows.Close()
	var out []*biz.RetentionPolicy
	for rows.Next() {
		p := &biz.RetentionPolicy{}
		if err := rows.Scan(&p.TableName, &p.RetentionDays, &p.CompressAfterDays, &p.Enabled, &p.UpdatedAt, &p.UpdatedBy); err != nil {
			return nil, fmt.Errorf("scan retention policy: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPolicy returns one table's policy.
func (r *RetentionRepo) GetPolicy(ctx context.Context, table string) (*biz.RetentionPolicy, error) {
	p := &biz.RetentionPolicy{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT table_name, retention_days, compress_after_days, enabled, updated_at, updated_by
		FROM retention_policies WHERE table_name = $1`, table).
		Scan(&p.TableName, &p.RetentionDays, &p.CompressAfterDays, &p.Enabled, &p.UpdatedAt, &p.UpdatedBy)
	if err == pgx.ErrNoRows {
		return &biz.RetentionPolicy{TableName: table}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get retention policy: %w", err)
	}
	return p, nil
}

// UpdatePolicy upserts a policy and returns the stored state.
func (r *RetentionRepo) UpdatePolicy(ctx context.Context, p *biz.RetentionPolicy, actor string) (*biz.RetentionPolicy, error) {
	out := &biz.RetentionPolicy{}
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO retention_policies (table_name, retention_days, compress_after_days, enabled, updated_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (table_name) DO UPDATE SET
			retention_days = excluded.retention_days,
			compress_after_days = excluded.compress_after_days,
			enabled = excluded.enabled,
			updated_by = excluded.updated_by,
			updated_at = now()
		RETURNING table_name, retention_days, compress_after_days, enabled, updated_at, updated_by`,
		p.TableName, p.RetentionDays, p.CompressAfterDays, p.Enabled, actor).
		Scan(&out.TableName, &out.RetentionDays, &out.CompressAfterDays, &out.Enabled, &out.UpdatedAt, &out.UpdatedBy)
	if err != nil {
		return nil, mapConstraint(err, "retention_policy")
	}
	return out, nil
}

// hasTimescale reports whether the timescaledb extension is installed.
func (r *RetentionRepo) hasTimescale(ctx context.Context) (bool, error) {
	var ok bool
	err := r.db.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb')`).Scan(&ok)
	return ok, err
}

// isHypertable reports whether table is a TimescaleDB hypertable.
func (r *RetentionRepo) isHypertable(ctx context.Context, table string) (bool, error) {
	has, err := r.hasTimescale(ctx)
	if err != nil || !has {
		return false, err
	}
	var ok bool
	err = r.db.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = $1)`, table).Scan(&ok)
	return ok, err
}

// Status returns live disk/chunk stats for a managed table. On plain PostgreSQL
// (no hypertable) it returns Hypertable=false with zeroed sizes.
func (r *RetentionRepo) Status(ctx context.Context, table biz.ManagedTable) (biz.RetentionStatus, error) {
	st := biz.RetentionStatus{TableName: table.Name}
	st.LastRun = r.lastRun(ctx, table.Name)

	hyper, err := r.isHypertable(ctx, table.Name)
	if err != nil {
		return st, fmt.Errorf("hypertable check %s: %w", table.Name, err)
	}
	st.Hypertable = hyper
	if !hyper {
		return st, nil
	}

	// Total on-disk size (heap + indexes + toast across all chunks).
	_ = r.db.Pool.QueryRow(ctx, `SELECT coalesce(hypertable_size($1::regclass), 0)`, table.Name).Scan(&st.TotalBytes)

	// Chunk counts.
	_ = r.db.Pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE is_compressed)
		FROM timescaledb_information.chunks WHERE hypertable_name = $1`, table.Name).
		Scan(&st.ChunkCount, &st.CompressedChunks)

	// Oldest/newest chunk boundaries (cheap — no data scan).
	var oldest, newest *time.Time
	_ = r.db.Pool.QueryRow(ctx, `
		SELECT min(range_start), max(range_end)
		FROM timescaledb_information.chunks WHERE hypertable_name = $1`, table.Name).
		Scan(&oldest, &newest)
	st.OldestData, st.NewestData = oldest, newest

	// Compressed footprint is best-effort: the stats view name/columns vary by
	// TimescaleDB version, so a failure here just leaves the split at zero.
	var compBytes *int64
	if err := r.db.Pool.QueryRow(ctx, `
		SELECT coalesce(sum(after_compression_total_bytes), 0)
		FROM chunk_compression_stats($1::regclass)`, table.Name).Scan(&compBytes); err == nil && compBytes != nil {
		st.CompressedBytes = *compBytes
	}
	if st.TotalBytes > st.CompressedBytes {
		st.UncompressedBytes = st.TotalBytes - st.CompressedBytes
	}
	return st, nil
}

// RunRetention compresses then drops eligible chunks for one table and records
// the run. It never returns a hard error for the table being non-hypertable;
// that is recorded on the run instead.
func (r *RetentionRepo) RunRetention(ctx context.Context, p *biz.RetentionPolicy, table biz.ManagedTable) (*biz.RetentionRun, error) {
	run := &biz.RetentionRun{TableName: table.Name, StartedAt: time.Now().UTC()}

	hyper, err := r.isHypertable(ctx, table.Name)
	if err != nil {
		return nil, err
	}
	if !hyper {
		run.Error = "not a TimescaleDB hypertable; retention skipped"
		r.finishRun(ctx, run)
		return run, nil
	}

	run.BytesBefore = r.sizeBytes(ctx, table.Name)

	var errs []string
	if p.CompressAfterDays > 0 {
		n, cerr := r.compressOldChunks(ctx, table, p.CompressAfterDays, p.RetentionDays)
		run.ChunksCompressed = n
		if cerr != nil {
			errs = append(errs, "compress: "+cerr.Error())
		}
	}
	if p.RetentionDays > 0 {
		n, derr := r.dropOldChunks(ctx, table.Name, p.RetentionDays)
		run.ChunksDropped = n
		if derr != nil {
			errs = append(errs, "drop: "+derr.Error())
		}
	}

	run.BytesAfter = r.sizeBytes(ctx, table.Name)
	run.Error = strings.Join(errs, "; ")
	r.finishRun(ctx, run)
	return run, nil
}

func (r *RetentionRepo) sizeBytes(ctx context.Context, table string) int64 {
	var b int64
	_ = r.db.Pool.QueryRow(ctx, `SELECT coalesce(hypertable_size($1::regclass), 0)`, table).Scan(&b)
	return b
}

// compressOldChunks compresses uncompressed chunks older than compressDays and
// (when retentionDays > 0) not yet eligible for dropping, so we never compress
// data that is about to be dropped. Compression is enabled on the table first.
func (r *RetentionRepo) compressOldChunks(ctx context.Context, table biz.ManagedTable, compressDays, retentionDays int) (int, error) {
	// Enable compression on the table (idempotent; best-effort across versions).
	_, _ = r.db.Pool.Exec(ctx, fmt.Sprintf(
		`ALTER TABLE %s SET (timescaledb.compress, timescaledb.compress_orderby = '%s DESC')`,
		pgQuoteIdent(table.Name), table.TimeColumn))

	query := `
		SELECT format('%I.%I', chunk_schema, chunk_name)
		FROM timescaledb_information.chunks
		WHERE hypertable_name = $1 AND NOT is_compressed
		  AND range_end < now() - ($2 || ' days')::interval`
	args := []any{table.Name, compressDays}
	if retentionDays > 0 {
		query += ` AND range_end >= now() - ($3 || ' days')::interval`
		args = append(args, retentionDays)
	}
	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("list chunks: %w", err)
	}
	var chunks []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return 0, err
		}
		chunks = append(chunks, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	n := 0
	for _, c := range chunks {
		if _, err := r.db.Pool.Exec(ctx, `SELECT compress_chunk($1::regclass)`, c); err != nil {
			return n, fmt.Errorf("compress %s: %w", c, err)
		}
		n++
	}
	return n, nil
}

// dropOldChunks drops chunks older than retentionDays and returns the count.
func (r *RetentionRepo) dropOldChunks(ctx context.Context, table string, retentionDays int) (int, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT drop_chunks($1::regclass, older_than => ($2 || ' days')::interval)`, table, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("drop_chunks: %w", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	return n, rows.Err()
}

func (r *RetentionRepo) finishRun(ctx context.Context, run *biz.RetentionRun) {
	fin := time.Now().UTC()
	run.FinishedAt = &fin
	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO retention_runs
			(table_name, started_at, finished_at, chunks_compressed, chunks_dropped, bytes_before, bytes_after, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		run.TableName, run.StartedAt, run.FinishedAt, run.ChunksCompressed, run.ChunksDropped,
		run.BytesBefore, run.BytesAfter, run.Error).Scan(&run.ID)
}

func (r *RetentionRepo) lastRun(ctx context.Context, table string) *biz.RetentionRun {
	run := &biz.RetentionRun{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, table_name, started_at, finished_at, chunks_compressed, chunks_dropped, bytes_before, bytes_after, error
		FROM retention_runs WHERE table_name = $1 ORDER BY started_at DESC LIMIT 1`, table).
		Scan(&run.ID, &run.TableName, &run.StartedAt, &run.FinishedAt, &run.ChunksCompressed,
			&run.ChunksDropped, &run.BytesBefore, &run.BytesAfter, &run.Error)
	if err != nil {
		return nil
	}
	return run
}

// pgQuoteIdent quotes a SQL identifier. Table names come from the fixed
// allowlist, but quote defensively for the dynamic ALTER.
func pgQuoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}
