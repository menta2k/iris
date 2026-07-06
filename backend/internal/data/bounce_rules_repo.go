package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/internal/biz"
)

// BounceRuleRepo persists bounce-action rules.
type BounceRuleRepo struct {
	db *DB
}

// NewBounceRuleRepo constructs the repository.
func NewBounceRuleRepo(db *DB) *BounceRuleRepo { return &BounceRuleRepo{db: db} }

var _ biz.BounceRuleRepo = (*BounceRuleRepo)(nil)

const bounceRuleCols = `id, smtp_code, enhanced_code, provider, pattern, class,
	category, action, action_config, suggested_action, priority, source, status,
	created_at, updated_at`

func scanBounceRule(row interface{ Scan(...any) error }) (*biz.BounceActionRule, error) {
	r := &biz.BounceActionRule{}
	if err := row.Scan(&r.ID, &r.SMTPCode, &r.EnhancedCode, &r.Provider, &r.Pattern, &r.Class,
		&r.Category, &r.Action, &r.ActionConfig, &r.SuggestedAction, &r.Priority, &r.Source, &r.Status,
		&r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	return r, nil
}

// CreateBounceRule inserts an overlay rule and returns the stored record.
func (r *BounceRuleRepo) CreateBounceRule(ctx context.Context, a *biz.BounceActionRule) (*biz.BounceActionRule, error) {
	out, err := scanBounceRule(r.db.Pool.QueryRow(ctx, `
		INSERT INTO bounce_action_rules
			(smtp_code, enhanced_code, provider, pattern, class, category, action, action_config, suggested_action, priority, source, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+bounceRuleCols,
		a.SMTPCode, a.EnhancedCode, a.Provider, a.Pattern, a.Class, a.Category, a.Action,
		a.ActionConfig, a.SuggestedAction, a.Priority, sourceOrOverlay(a.Source), a.Status))
	if err != nil {
		return nil, mapConstraint(err, "bounce_action_rule")
	}
	return out, nil
}

// UpdateBounceRule updates a rule by id.
func (r *BounceRuleRepo) UpdateBounceRule(ctx context.Context, id string, a *biz.BounceActionRule) (*biz.BounceActionRule, error) {
	out, err := scanBounceRule(r.db.Pool.QueryRow(ctx, `
		UPDATE bounce_action_rules
		SET smtp_code = $2, enhanced_code = $3, provider = $4, pattern = $5, class = $6,
		    category = $7, action = $8, action_config = $9, suggested_action = $10,
		    priority = $11, status = $12, updated_at = now()
		WHERE id = $1
		RETURNING `+bounceRuleCols,
		id, a.SMTPCode, a.EnhancedCode, a.Provider, a.Pattern, a.Class, a.Category, a.Action,
		a.ActionConfig, a.SuggestedAction, a.Priority, a.Status))
	if err != nil {
		return nil, mapConstraint(err, "bounce_action_rule")
	}
	return out, nil
}

// DeleteBounceRule removes a rule by id.
func (r *BounceRuleRepo) DeleteBounceRule(ctx context.Context, id string) error {
	if _, err := r.db.Pool.Exec(ctx, `DELETE FROM bounce_action_rules WHERE id = $1`, id); err != nil {
		return mapConstraint(err, "bounce_action_rule")
	}
	return nil
}

// ListBounceRules returns every rule (priority desc for display).
func (r *BounceRuleRepo) ListBounceRules(ctx context.Context) ([]*biz.BounceActionRule, error) {
	return r.query(ctx, `SELECT `+bounceRuleCols+` FROM bounce_action_rules ORDER BY priority DESC, smtp_code, enhanced_code`)
}

// ListActiveBounceRules returns active rules (for the matcher / TSA render).
func (r *BounceRuleRepo) ListActiveBounceRules(ctx context.Context) ([]*biz.BounceActionRule, error) {
	return r.query(ctx, `SELECT `+bounceRuleCols+` FROM bounce_action_rules WHERE status = 'active' ORDER BY priority DESC`)
}

// CountBounceRules returns the total number of rules (used to seed on first use).
func (r *BounceRuleRepo) CountBounceRules(ctx context.Context) (int, error) {
	var n int
	if err := r.db.Pool.QueryRow(ctx, `SELECT count(*) FROM bounce_action_rules`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count bounce_action_rules: %w", err)
	}
	return n, nil
}

// ReplaceDefaultBounceRules deletes the seeded default rules and re-inserts the
// given set in one transaction, leaving operator overlay rules untouched. Used
// by both first-use seeding and "Reset to defaults".
func (r *BounceRuleRepo) ReplaceDefaultBounceRules(ctx context.Context, rules []*biz.BounceActionRule) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM bounce_action_rules WHERE source = 'default'`); err != nil {
		return fmt.Errorf("clear default bounce rules: %w", err)
	}
	for _, a := range rules {
		if _, err := tx.Exec(ctx, `
			INSERT INTO bounce_action_rules
				(smtp_code, enhanced_code, provider, pattern, class, category, action, action_config, suggested_action, priority, source, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'default', $11)`,
			a.SMTPCode, a.EnhancedCode, a.Provider, a.Pattern, a.Class, a.Category, a.Action,
			a.ActionConfig, a.SuggestedAction, a.Priority, a.Status); err != nil {
			return fmt.Errorf("insert default bounce rule: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *BounceRuleRepo) query(ctx context.Context, sql string) ([]*biz.BounceActionRule, error) {
	rows, err := r.db.Pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query bounce_action_rules: %w", err)
	}
	defer rows.Close()
	var out []*biz.BounceActionRule
	for rows.Next() {
		a, err := scanBounceRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bounce_action_rule: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func sourceOrOverlay(s string) string {
	if s == biz.BounceRuleSourceDefault {
		return biz.BounceRuleSourceDefault
	}
	return biz.BounceRuleSourceOverlay
}
