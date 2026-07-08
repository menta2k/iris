package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/menta2k/iris/backend/internal/biz"
)

// SubjectClassificationRepo persists subject → label classification rules and
// serves the pg_trgm similarity lookups the classifier uses.
type SubjectClassificationRepo struct {
	db *DB
}

// NewSubjectClassificationRepo constructs the repository.
func NewSubjectClassificationRepo(db *DB) *SubjectClassificationRepo {
	return &SubjectClassificationRepo{db: db}
}

var _ biz.SubjectClassificationRepo = (*SubjectClassificationRepo)(nil)

const subjectClassificationCols = `id, subject, subject_normalized, label, source, match_type, priority, hit_count, created_at, updated_at`

func scanSubjectClassification(row interface{ Scan(...any) error }) (*biz.SubjectClassification, error) {
	c := &biz.SubjectClassification{}
	if err := row.Scan(&c.ID, &c.Subject, &c.SubjectNormalized, &c.Label,
		&c.Source, &c.MatchType, &c.Priority, &c.HitCount, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return c, nil
}

// BestMatch returns the labeled rule most similar to normalized at or above
// threshold, or (nil, nil) when nothing qualifies. Pending rows (label = '')
// are excluded so an unlabeled subject never matches.
func (r *SubjectClassificationRepo) BestMatch(ctx context.Context, normalized string, threshold float64) (*biz.SubjectClassification, error) {
	out, err := scanSubjectClassification(r.db.Pool.QueryRow(ctx, `
		SELECT `+subjectClassificationCols+`
		FROM subject_classifications
		WHERE match_type = 'similarity' AND label <> '' AND similarity(subject_normalized, $1) >= $2
		ORDER BY priority DESC, similarity(subject_normalized, $1) DESC, hit_count DESC
		LIMIT 1`, normalized, threshold))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("classification best match: %w", err)
	}
	return out, nil
}

// RegexRules returns all labeled regex rules, highest priority first, so the
// matcher can test them against a raw subject in priority order.
func (r *SubjectClassificationRepo) RegexRules(ctx context.Context) ([]*biz.SubjectClassification, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT `+subjectClassificationCols+`
		FROM subject_classifications
		WHERE match_type = 'regex' AND label <> ''
		ORDER BY priority DESC, updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list regex classifications: %w", err)
	}
	defer rows.Close()
	var out []*biz.SubjectClassification
	for rows.Next() {
		c, err := scanSubjectClassification(rows)
		if err != nil {
			return nil, fmt.Errorf("scan regex classification: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Upsert inserts a rule or, when its normalized key already exists, updates its
// label/source. Returns the stored row.
func (r *SubjectClassificationRepo) Upsert(ctx context.Context, c *biz.SubjectClassification) (*biz.SubjectClassification, error) {
	source := c.Source
	if source == "" {
		source = biz.ClassificationSourceAI
	}
	out, err := scanSubjectClassification(r.db.Pool.QueryRow(ctx, `
		INSERT INTO subject_classifications (subject, subject_normalized, label, source, match_type, priority)
		VALUES ($1, $2, $3, $4, 'similarity', $5)
		ON CONFLICT (subject_normalized) WHERE match_type = 'similarity' DO UPDATE
		SET label = EXCLUDED.label, source = EXCLUDED.source, updated_at = now()
		RETURNING `+subjectClassificationCols,
		c.Subject, c.SubjectNormalized, c.Label, source, c.Priority))
	if err != nil {
		return nil, mapConstraint(err, "subject_classification")
	}
	return out, nil
}

// IncrementHit bumps the match counter for a rule.
func (r *SubjectClassificationRepo) IncrementHit(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx,
		`UPDATE subject_classifications SET hit_count = hit_count + 1, updated_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("increment classification hit: %w", err)
	}
	return nil
}

// List returns all rules, most-used first.
func (r *SubjectClassificationRepo) List(ctx context.Context) ([]*biz.SubjectClassification, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+subjectClassificationCols+` FROM subject_classifications ORDER BY priority DESC, hit_count DESC, updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list subject_classifications: %w", err)
	}
	defer rows.Close()
	var out []*biz.SubjectClassification
	for rows.Next() {
		c, err := scanSubjectClassification(rows)
		if err != nil {
			return nil, fmt.Errorf("scan subject_classification: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Create inserts an operator-authored rule.
func (r *SubjectClassificationRepo) Create(ctx context.Context, c *biz.SubjectClassification) (*biz.SubjectClassification, error) {
	matchType := c.MatchType
	if matchType == "" {
		matchType = biz.ClassificationMatchSimilarity
	}
	out, err := scanSubjectClassification(r.db.Pool.QueryRow(ctx, `
		INSERT INTO subject_classifications (subject, subject_normalized, label, source, match_type, priority)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+subjectClassificationCols,
		c.Subject, c.SubjectNormalized, c.Label, c.Source, matchType, c.Priority))
	if err != nil {
		return nil, mapConstraint(err, "subject_classification")
	}
	return out, nil
}

// Update edits a rule by id.
func (r *SubjectClassificationRepo) Update(ctx context.Context, c *biz.SubjectClassification) (*biz.SubjectClassification, error) {
	out, err := scanSubjectClassification(r.db.Pool.QueryRow(ctx, `
		UPDATE subject_classifications
		SET subject = $2, subject_normalized = $3, label = $4, source = $5,
		    match_type = $6, priority = $7, updated_at = now()
		WHERE id = $1
		RETURNING `+subjectClassificationCols,
		c.ID, c.Subject, c.SubjectNormalized, c.Label, c.Source, c.MatchType, c.Priority))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, biz.NotFound("CLASSIFICATION_NOT_FOUND", "subject_classification %s not found", c.ID)
	}
	if err != nil {
		return nil, mapConstraint(err, "subject_classification")
	}
	return out, nil
}

// Delete removes a rule by id.
func (r *SubjectClassificationRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Pool.Exec(ctx, `DELETE FROM subject_classifications WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete subject_classification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return biz.NotFound("CLASSIFICATION_NOT_FOUND", "subject_classification %s not found", id)
	}
	return nil
}
