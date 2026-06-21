// Package data implements storage adapters for TimescaleDB and Redis Streams
// and the repository implementations backing the business use cases.
package data

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/menta2k/iris/backend/internal/conf"
)

// DB wraps a pgx connection pool with transaction helpers.
type DB struct {
	Pool *pgxpool.Pool
}

// NewDB opens a TimescaleDB/PostgreSQL connection pool from configuration.
func NewDB(ctx context.Context, c conf.Database) (*DB, func(), error) {
	pcfg, err := pgxpool.ParseConfig(c.DSN)
	if err != nil {
		return nil, nil, fmt.Errorf("parse dsn: %w", err)
	}
	if c.MaxConns > 0 {
		pcfg.MaxConns = c.MaxConns
	}
	if c.MinConns > 0 {
		pcfg.MinConns = c.MinConns
	}
	if c.ConnMaxLifetime > 0 {
		pcfg.MaxConnLifetime = c.ConnMaxLifetime
	}

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{Pool: pool}
	cleanup := func() { pool.Close() }
	return db, cleanup, nil
}

// nullableUUID returns the id as a query argument when it is a valid UUID, or
// nil otherwise so a non-UUID actor (e.g. a synthetic identity) is stored as
// NULL rather than triggering an invalid-syntax error.
func nullableUUID(id string) any {
	if _, err := uuid.Parse(id); err != nil {
		return nil
	}
	return id
}

// nullableText returns the string as a query argument, or nil when empty so an
// optional column is stored as NULL rather than an empty string.
func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Health verifies database connectivity for readiness checks.
func (d *DB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return d.Pool.Ping(ctx)
}

// InTx runs fn inside a transaction, rolling back on error and committing on
// success. The provided pgx.Tx must be used for all queries within fn.
func (d *DB) InTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Rollback is a no-op if the tx was already committed.
		_ = tx.Rollback(ctx)
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
