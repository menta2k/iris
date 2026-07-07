package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// EventProcessorRepo persists Event Processor rules.
type EventProcessorRepo struct {
	db *DB
}

// NewEventProcessorRepo constructs the repository.
func NewEventProcessorRepo(db *DB) *EventProcessorRepo { return &EventProcessorRepo{db: db} }

var (
	_ biz.EventProcessorRepo   = (*EventProcessorRepo)(nil)
	_ biz.EventProcessorSource = (*EventProcessorRepo)(nil)
)

const eventProcessorCols = `id, name, event_types, mailclasses, driver, driver_config,
	mode, batch_max_size, batch_max_wait, status, created_at, updated_at`

func scanEventProcessor(row interface{ Scan(...any) error }) (*biz.EventProcessor, error) {
	p := &biz.EventProcessor{}
	var cfg []byte
	if err := row.Scan(&p.ID, &p.Name, &p.EventTypes, &p.Mailclasses, &p.Driver, &cfg,
		&p.Mode, &p.BatchMaxSize, &p.BatchMaxWait, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	if len(cfg) > 0 {
		_ = json.Unmarshal(cfg, &p.DriverConfig)
	}
	if p.DriverConfig == nil {
		p.DriverConfig = map[string]string{}
	}
	return p, nil
}

// CreateEventProcessor inserts a processor and returns the stored record.
func (r *EventProcessorRepo) CreateEventProcessor(ctx context.Context, p *biz.EventProcessor) (*biz.EventProcessor, error) {
	cfg, _ := json.Marshal(p.DriverConfig)
	out, err := scanEventProcessor(r.db.Pool.QueryRow(ctx, `
		INSERT INTO event_processors
			(name, event_types, mailclasses, driver, driver_config, mode, batch_max_size, batch_max_wait, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING `+eventProcessorCols,
		p.Name, p.EventTypes, p.Mailclasses, p.Driver, cfg, p.Mode, p.BatchMaxSize, p.BatchMaxWait, p.Status))
	if err != nil {
		return nil, mapConstraint(err, "event_processor")
	}
	return out, nil
}

// UpdateEventProcessor updates a processor by id.
func (r *EventProcessorRepo) UpdateEventProcessor(ctx context.Context, id string, p *biz.EventProcessor) (*biz.EventProcessor, error) {
	cfg, _ := json.Marshal(p.DriverConfig)
	out, err := scanEventProcessor(r.db.Pool.QueryRow(ctx, `
		UPDATE event_processors
		SET name = $2, event_types = $3, mailclasses = $4, driver = $5, driver_config = $6,
		    mode = $7, batch_max_size = $8, batch_max_wait = $9, status = $10, updated_at = now()
		WHERE id = $1
		RETURNING `+eventProcessorCols,
		id, p.Name, p.EventTypes, p.Mailclasses, p.Driver, cfg, p.Mode, p.BatchMaxSize, p.BatchMaxWait, p.Status))
	if err != nil {
		return nil, mapConstraint(err, "event_processor")
	}
	return out, nil
}

// DeleteEventProcessor removes a processor by id.
func (r *EventProcessorRepo) DeleteEventProcessor(ctx context.Context, id string) error {
	if _, err := r.db.Pool.Exec(ctx, `DELETE FROM event_processors WHERE id = $1`, id); err != nil {
		return mapConstraint(err, "event_processor")
	}
	return nil
}

// ListEventProcessors returns every processor (newest first).
func (r *EventProcessorRepo) ListEventProcessors(ctx context.Context) ([]*biz.EventProcessor, error) {
	return r.query(ctx, `SELECT `+eventProcessorCols+` FROM event_processors ORDER BY created_at DESC`)
}

// ActiveEventProcessors returns active processors (for the dispatcher).
func (r *EventProcessorRepo) ActiveEventProcessors(ctx context.Context) ([]*biz.EventProcessor, error) {
	return r.query(ctx, `SELECT `+eventProcessorCols+` FROM event_processors WHERE status = 'active'`)
}

func (r *EventProcessorRepo) query(ctx context.Context, sql string) ([]*biz.EventProcessor, error) {
	rows, err := r.db.Pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query event_processors: %w", err)
	}
	defer rows.Close()
	var out []*biz.EventProcessor
	for rows.Next() {
		p, err := scanEventProcessor(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event_processor: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
