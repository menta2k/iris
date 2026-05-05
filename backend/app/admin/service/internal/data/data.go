// Package data wires the ent client and repositories.
//
// On startup we run two migration phases against the configured Postgres:
//
//  1. ent's schema diff (idempotent, schema/*.go is the source of truth).
//  2. our hand-written SQL files under /app/sql, applied in lexical order.
//     These cover TimescaleDB-specific DDL ent cannot emit — hypertable
//     conversion, compression policies, retention policies, and the
//     metric_delivery_hourly continuous aggregate. Each statement is
//     guarded by IF (NOT) EXISTS so re-running is safe.
//
// The migration directory path is configurable via IRIS_SQL_MIGRATIONS_DIR
// (default: /app/sql, matching the Dockerfile COPY layout). When the dir is
// missing we log and continue — useful for unit tests that bring up an
// in-memory client without the SQL bundle.
package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx Postgres driver, registered as "pgx"

	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/migrate"
)

// NewEntClient opens the Postgres connection and runs both migration phases.
func NewEntClient(ctx *bootstrap.Context) (*ent.Client, func(), error) {
	cfg := ctx.GetConfig().GetData().GetDatabase()
	if cfg == nil {
		return nil, func() {}, fmt.Errorf("data: missing data.database config")
	}

	db, err := sql.Open("pgx", cfg.Source)
	if err != nil {
		return nil, func() {}, fmt.Errorf("data: open postgres: %w", err)
	}
	db.SetMaxIdleConns(10)
	db.SetMaxOpenConns(100)
	db.SetConnMaxLifetime(10 * time.Minute)

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))

	if cfg.Migrate {
		mctx, cancel := context.WithTimeout(ctx.Context(), 30*time.Second)
		defer cancel()
		if err := client.Schema.Create(mctx,
			migrate.WithDropIndex(false),
			migrate.WithDropColumn(false),
		); err != nil {
			// Once a table has been converted to a hypertable, its primary
			// key must include the partitioning column (TimescaleDB error
			// SQLSTATE TS103). ent's schema-diff doesn't know that and on
			// every subsequent boot tries to "fix" the composite PK back
			// to id-only — which TimescaleDB rightfully rejects. The DDL
			// the migrator wanted is a no-op for our purposes (the schema
			// is the same; only the index shape differs), so we treat
			// that specific error as harmless and proceed to phase 2.
			if !isHypertablePartitionConflict(err) {
				_ = client.Close()
				return nil, func() {}, fmt.Errorf("data: ent migrate: %w", err)
			}
			log.Printf("data: ignoring hypertable partition conflict from ent migrate (%v)", err)
		}
		// Phase 2: hand-written TimescaleDB DDL. Runs after ent so the
		// tables we're hypertabling already exist.
		if err := applySQLMigrations(mctx, db, sqlMigrationsDir()); err != nil {
			_ = client.Close()
			return nil, func() {}, fmt.Errorf("data: sql migrate: %w", err)
		}
	}

	cleanup := func() { _ = client.Close() }
	return client, cleanup, nil
}

// sqlMigrationsDir resolves the directory holding *.sql files. Lets tests
// override via env without touching wire.
func sqlMigrationsDir() string {
	if v := strings.TrimSpace(os.Getenv("IRIS_SQL_MIGRATIONS_DIR")); v != "" {
		return v
	}
	return "/app/sql"
}

// isHypertablePartitionConflict matches TimescaleDB's "cannot create a
// unique index without the column used in partitioning" error. The error
// surfaces on every boot after the first (when ent's diff decides to revert
// our composite PK back to id-only). Matching by code+message text rather
// than a typed error because pgx wraps and unwraps these inconsistently.
func isHypertablePartitionConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "TS103") &&
		strings.Contains(msg, "cannot create a unique index without")
}

// applySQLMigrations executes every *.sql file in dir in lexical order. Each
// file is run as a single batch (no per-statement splitting) — TimescaleDB
// DO blocks span multiple lines and would break a naive splitter. Files are
// expected to be idempotent.
func applySQLMigrations(ctx context.Context, db *sql.DB, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			log.Printf("data: sql migrations dir %s missing — skipping (this is fine for tests)", dir)
			return nil
		}
		return fmt.Errorf("read %s: %w", dir, err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil
	}
	for _, name := range files {
		path := filepath.Join(dir, name)
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		log.Printf("data: applying sql migration %s (%d bytes)", name, len(body))
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("exec %s: %w", name, err)
		}
	}
	return nil
}
