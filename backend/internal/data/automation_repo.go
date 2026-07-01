package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// AutomationRepo persists TSA automation rules.
type AutomationRepo struct {
	db *DB
}

// NewAutomationRepo constructs the repository.
func NewAutomationRepo(db *DB) *AutomationRepo { return &AutomationRepo{db: db} }

var _ biz.AutomationRepo = (*AutomationRepo)(nil)

const automationCols = `id, domain, regex, action, config_name, config_value,
	trigger_spec, duration, status, created_at, updated_at`

func scanAutomation(row interface{ Scan(...any) error }) (*biz.AutomationRule, error) {
	r := &biz.AutomationRule{}
	if err := row.Scan(&r.ID, &r.Domain, &r.Regex, &r.Action, &r.ConfigName, &r.ConfigValue,
		&r.Trigger, &r.Duration, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	return r, nil
}

// CreateAutomationRule inserts a rule and returns the stored record.
func (r *AutomationRepo) CreateAutomationRule(ctx context.Context, a *biz.AutomationRule) (*biz.AutomationRule, error) {
	out, err := scanAutomation(r.db.Pool.QueryRow(ctx, `
		INSERT INTO tsa_automation_rules (domain, regex, action, config_name, config_value, trigger_spec, duration, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+automationCols,
		a.Domain, a.Regex, a.Action, a.ConfigName, a.ConfigValue, a.Trigger, a.Duration, a.Status))
	if err != nil {
		return nil, mapConstraint(err, "tsa_automation_rule")
	}
	return out, nil
}

// UpdateAutomationRule updates a rule by id (edits and enable/disable toggles).
func (r *AutomationRepo) UpdateAutomationRule(ctx context.Context, id string, a *biz.AutomationRule) (*biz.AutomationRule, error) {
	out, err := scanAutomation(r.db.Pool.QueryRow(ctx, `
		UPDATE tsa_automation_rules
		SET domain = $2, regex = $3, action = $4, config_name = $5, config_value = $6,
		    trigger_spec = $7, duration = $8, status = $9, updated_at = now()
		WHERE id = $1
		RETURNING `+automationCols,
		id, a.Domain, a.Regex, a.Action, a.ConfigName, a.ConfigValue, a.Trigger, a.Duration, a.Status))
	if err != nil {
		return nil, mapConstraint(err, "tsa_automation_rule")
	}
	return out, nil
}

// GetAutomationRule returns one rule by id.
func (r *AutomationRepo) GetAutomationRule(ctx context.Context, id string) (*biz.AutomationRule, error) {
	out, err := scanAutomation(r.db.Pool.QueryRow(ctx,
		`SELECT `+automationCols+` FROM tsa_automation_rules WHERE id = $1`, id))
	if err != nil {
		return nil, mapConstraint(err, "tsa_automation_rule")
	}
	return out, nil
}

// ListAutomationRules returns all rules (by domain then creation time).
func (r *AutomationRepo) ListAutomationRules(ctx context.Context) ([]*biz.AutomationRule, error) {
	return r.query(ctx, `SELECT `+automationCols+` FROM tsa_automation_rules ORDER BY domain, created_at`)
}

// ListActiveAutomationForPolicy returns active rules for rendering.
func (r *AutomationRepo) ListActiveAutomationForPolicy(ctx context.Context) ([]*biz.AutomationRule, error) {
	return r.query(ctx, `SELECT `+automationCols+` FROM tsa_automation_rules WHERE status = 'active' ORDER BY domain, regex`)
}

func (r *AutomationRepo) query(ctx context.Context, sql string) ([]*biz.AutomationRule, error) {
	rows, err := r.db.Pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query tsa_automation_rules: %w", err)
	}
	defer rows.Close()
	var out []*biz.AutomationRule
	for rows.Next() {
		a, err := scanAutomation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tsa_automation_rule: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
