package biz

import (
	"context"
	"strings"
)

// SubjectClassifierUsecase resolves a classification label for a subject: it
// first looks for a similar, already-labeled subject (pg_trgm), and only on a
// miss falls back to the LLM — caching the result so the corpus self-populates.
// It performs no permission check; it is driven by the background worker.
type SubjectClassifierUsecase struct {
	repo   SubjectClassificationRepo
	ai     SubjectAIClassifier // nil when no API key → AI fallback disabled
	policy ClassifyPolicyProvider
}

// NewSubjectClassifierUsecase constructs the use case. ai may be nil.
func NewSubjectClassifierUsecase(repo SubjectClassificationRepo, ai SubjectAIClassifier, policy ClassifyPolicyProvider) *SubjectClassifierUsecase {
	return &SubjectClassifierUsecase{repo: repo, ai: ai, policy: policy}
}

// Classify returns a label for the given raw subject, or "" (no error) when the
// feature is off, the subject has no classifiable text, or no label could be
// derived (e.g. AI disabled or returned nothing). Errors are returned only for
// unexpected failures (DB/LLM transport) so the worker can retry.
func (uc *SubjectClassifierUsecase) Classify(ctx context.Context, subject string) (string, error) {
	pol := uc.policy.ClassifyPolicyNow(ctx)
	if !pol.Enabled {
		return "", nil
	}
	norm := NormalizeSubject(subject)
	if norm == "" {
		return "", nil
	}

	// 1) Similarity match against previously-labeled subjects.
	match, err := uc.repo.BestMatch(ctx, norm, pol.Threshold)
	if err != nil {
		return "", err
	}
	if match != nil {
		if err := uc.repo.IncrementHit(ctx, match.ID); err != nil {
			// Non-fatal: the label is still valid even if the counter update fails.
			LoggerFrom(ctx).Warn("classify: increment hit failed", "id", match.ID, "error", err.Error())
		}
		return match.Label, nil
	}

	// 2) LLM fallback (self-populates the corpus).
	if uc.ai == nil {
		return "", nil
	}
	raw, err := uc.ai.ClassifySubject(ctx, subject, pol.Model, pol.APIBase)
	if err != nil {
		return "", err
	}
	label := normalizeLabel(raw)
	if label == "" {
		return "", nil
	}
	if _, err := uc.repo.Upsert(ctx, &SubjectClassification{
		Subject:           strings.TrimSpace(subject),
		SubjectNormalized: norm,
		Label:             label,
		Source:            ClassificationSourceAI,
	}); err != nil {
		return "", err
	}
	return label, nil
}
