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
	// 1) Rule match. Regex rules (tested against the raw subject) and the best
	// similarity rule compete on priority; the highest-priority match wins and,
	// on a tie, the explicit regex rule is preferred. This realizes
	// "match by priority, first match wins" across both rule kinds.
	match, err := uc.bestRuleMatch(ctx, subject, pol.Threshold)
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

	norm := NormalizeSubject(subject)
	if norm == "" {
		return "", nil
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

// bestRuleMatch resolves the highest-priority rule that matches subject, or nil
// when none do. Regex rules are tested against the raw subject and the best
// similarity rule against the normalized key; the two compete on priority, and
// an equal-priority tie is resolved in favour of the explicit regex rule.
func (uc *SubjectClassifierUsecase) bestRuleMatch(ctx context.Context, subject string, threshold float64) (*SubjectClassification, error) {
	var sim *SubjectClassification
	if norm := NormalizeSubject(subject); norm != "" {
		m, err := uc.repo.BestMatch(ctx, norm, threshold)
		if err != nil {
			return nil, err
		}
		sim = m
	}

	regexMatch, err := uc.firstRegexMatch(ctx, subject)
	if err != nil {
		return nil, err
	}

	switch {
	case regexMatch != nil && sim != nil:
		if regexMatch.Priority >= sim.Priority {
			return regexMatch, nil
		}
		return sim, nil
	case regexMatch != nil:
		return regexMatch, nil
	default:
		return sim, nil // sim may be nil → no match
	}
}

// firstRegexMatch returns the highest-priority regex rule whose pattern matches
// the raw subject, or nil. Rules arrive priority-ordered, so the first match is
// the winner. A rule that fails to compile is skipped (a stored rule is
// validated on write, so this is defensive against data drift).
func (uc *SubjectClassifierUsecase) firstRegexMatch(ctx context.Context, subject string) (*SubjectClassification, error) {
	rules, err := uc.repo.RegexRules(ctx)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		re, err := rule.CompileRegex()
		if err != nil {
			LoggerFrom(ctx).Warn("classify: skipping invalid regex rule", "id", rule.ID, "error", err.Error())
			continue
		}
		if re.MatchString(subject) {
			return rule, nil
		}
	}
	return nil, nil
}
