package data

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migration is a single ordered SQL migration file.
type Migration struct {
	Name string
	SQL  string
}

// LoadMigrations reads embedded migration files in lexical (numbered) order.
func LoadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	migrations := make([]Migration, 0, len(names))
	for _, name := range names {
		raw, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		migrations = append(migrations, Migration{Name: name, SQL: string(raw)})
	}
	return migrations, nil
}

// Migrate applies all pending migrations inside a tracking table. Each
// migration is recorded so reruns are idempotent.
func (d *DB) Migrate(ctx context.Context) error {
	migrations, err := LoadMigrations()
	if err != nil {
		return err
	}

	_, err = d.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name        TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var exists bool
		if err := d.Pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, m.Name,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", m.Name, err)
		}
		if exists {
			continue
		}
		if err := d.InTx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, m.SQL); err != nil {
				return fmt.Errorf("apply %s: %w", m.Name, err)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO schema_migrations (name) VALUES ($1)`, m.Name,
			); err != nil {
				return fmt.Errorf("record %s: %w", m.Name, err)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
