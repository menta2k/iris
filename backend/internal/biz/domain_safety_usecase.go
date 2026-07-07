package biz

import (
	"context"
	"strings"
)

// DomainSafetyRepo is the persistence boundary for DKIM and suppressions.
type DomainSafetyRepo interface {
	CreateDKIMDomain(ctx context.Context, d *DKIMDomain) (*DKIMDomain, error)
	UpdateDKIMDomain(ctx context.Context, id string, d *DKIMDomain) (*DKIMDomain, error)
	ListDKIMDomains(ctx context.Context, page Page) ([]*DKIMDomain, error)
	CreateSuppression(ctx context.Context, s *SuppressionEntry) (*SuppressionEntry, error)
	UpdateSuppression(ctx context.Context, id string, s *SuppressionEntry) (*SuppressionEntry, error)
	ListSuppressions(ctx context.Context, search string, page Page) ([]*SuppressionEntry, error)
	IsSuppressed(ctx context.Context, recipient string) (bool, error)
	// SuppressionValueByID resolves a suppression's value (the recipient) by id;
	// "" when no such entry exists.
	SuppressionValueByID(ctx context.Context, id string) (string, error)
	// ListDSNMessages returns the raw DSN messages archived for a recipient,
	// newest first, bounded by limit.
	ListDSNMessages(ctx context.Context, recipient string, limit int) ([]*DSNMessage, error)
	CreateTLSPolicy(ctx context.Context, p *TLSPolicy) (*TLSPolicy, error)
	ListTLSPolicies(ctx context.Context, page Page) ([]*TLSPolicy, error)
	DeleteTLSPolicy(ctx context.Context, id string) error
}

// DomainSafetyUsecase implements DKIM and suppression management (US4) and
// exposes the suppression eligibility check used by outbound send logic.
type DomainSafetyUsecase struct {
	repo    DomainSafetyRepo
	auditor *Auditor
}

// NewDomainSafetyUsecase constructs the use case.
func NewDomainSafetyUsecase(repo DomainSafetyRepo, auditor *Auditor) *DomainSafetyUsecase {
	return &DomainSafetyUsecase{repo: repo, auditor: auditor}
}

// ListDKIMDomains returns DKIM configurations with private key refs stripped.
func (uc *DomainSafetyUsecase) ListDKIMDomains(ctx context.Context, page Page) ([]*DKIMDomain, error) {
	if _, err := RequirePermission(ctx, PermDKIMRead); err != nil {
		return nil, err
	}
	items, err := uc.repo.ListDKIMDomains(ctx, page)
	if err != nil {
		return nil, err
	}
	for _, d := range items {
		d.PrivateKeyRef = "" // never expose the secret reference over the API
	}
	return items, nil
}

// CreateDKIMDomain validates and persists a DKIM configuration.
func (uc *DomainSafetyUsecase) CreateDKIMDomain(ctx context.Context, d *DKIMDomain) (*DKIMDomain, error) {
	if _, err := RequirePermission(ctx, PermDKIMWrite); err != nil {
		return nil, err
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if err := deriveDKIMFingerprint(d); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateDKIMDomain(ctx, d)
	if err != nil {
		uc.audit(ctx, "dkim.create", "dkim", d.Domain, AuditFailure, map[string]any{"domain": d.Domain})
		return nil, err
	}
	// The audit summary intentionally omits private_key_ref.
	uc.audit(ctx, "dkim.create", "dkim", out.ID, AuditSuccess, map[string]any{
		"domain": out.Domain, "selector": out.Selector, "status": out.Status,
	})
	out.PrivateKeyRef = ""
	return out, nil
}

// UpdateDKIMDomain updates the editable DKIM fields (selector, fingerprint,
// key reference, status). The domain itself is immutable. Setting status to
// "ready" is what enables signing for the domain in the generated policy.
func (uc *DomainSafetyUsecase) UpdateDKIMDomain(ctx context.Context, id string, d *DKIMDomain) (*DKIMDomain, error) {
	if _, err := RequirePermission(ctx, PermDKIMWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("DKIM_ID_REQUIRED", "dkim id is required")
	}
	d.Selector = strings.TrimSpace(d.Selector)
	if d.Selector == "" || !dkimSelectorRe.MatchString(d.Selector) {
		return nil, Invalid("DKIM_SELECTOR_INVALID", "selector %q is not valid", d.Selector)
	}
	// A provided key replaces the stored one; an empty key preserves it (the repo
	// COALESCEs both the key and its fingerprint).
	if strings.TrimSpace(d.PrivateKeyRef) != "" {
		if _, err := ParseDKIMPrivateKey(d.PrivateKeyRef); err != nil {
			return nil, Invalid("DKIM_PRIVATE_KEY_INVALID", "private key is not a valid RSA PEM: %v", err)
		}
		if err := deriveDKIMFingerprint(d); err != nil {
			return nil, err
		}
	}
	if d.Status == "" {
		d.Status = DKIMNeedsAttention
	}
	switch d.Status {
	case DKIMReady, DKIMDisabled, DKIMNeedsAttention:
	default:
		return nil, Invalid("DKIM_STATUS_INVALID", "status %q is not valid", d.Status)
	}
	out, err := uc.repo.UpdateDKIMDomain(ctx, id, d)
	if err != nil {
		uc.audit(ctx, "dkim.update", "dkim", id, AuditFailure, map[string]any{"selector": d.Selector})
		return nil, err
	}
	uc.audit(ctx, "dkim.update", "dkim", out.ID, AuditSuccess, map[string]any{
		"domain": out.Domain, "selector": out.Selector, "status": out.Status,
	})
	out.PrivateKeyRef = ""
	return out, nil
}

// DKIMKeyMaterial is the result of generating a DKIM key pair: the private key
// to store (PEM) and the public DNS record the operator must publish.
type DKIMKeyMaterial struct {
	PrivateKeyPEM string
	RecordName    string
	RecordValue   string
	Fingerprint   string
}

// GenerateDKIMKey mints a fresh RSA key pair for a domain/selector. It does NOT
// persist anything: it returns the private key (for the caller to save on the
// DKIM domain) and the DNS TXT record to publish. The private key is returned
// exactly once, here.
func (uc *DomainSafetyUsecase) GenerateDKIMKey(ctx context.Context, domain, selector string) (*DKIMKeyMaterial, error) {
	if _, err := RequirePermission(ctx, PermDKIMWrite); err != nil {
		return nil, err
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	selector = strings.TrimSpace(selector)
	if domain == "" || len(domain) > 253 || !dnsNameRe.MatchString(domain) {
		return nil, Invalid("DKIM_DOMAIN_INVALID", "domain %q is not a valid DNS name", domain)
	}
	if selector == "" || !dkimSelectorRe.MatchString(selector) {
		return nil, Invalid("DKIM_SELECTOR_INVALID", "selector %q is not valid", selector)
	}
	pemStr, err := GenerateDKIMPrivateKey()
	if err != nil {
		return nil, err
	}
	recordValue, fingerprint, err := DKIMPublicRecord(pemStr)
	if err != nil {
		return nil, err
	}
	uc.audit(ctx, "dkim.generate_key", "dkim", domain, AuditSuccess, map[string]any{
		"domain": domain, "selector": selector, "fingerprint": fingerprint,
	})
	return &DKIMKeyMaterial{
		PrivateKeyPEM: pemStr,
		RecordName:    DKIMRecordName(selector, domain),
		RecordValue:   recordValue,
		Fingerprint:   fingerprint,
	}, nil
}

// deriveDKIMFingerprint sets the public-key fingerprint from the (already
// validated) private key, when one is present.
func deriveDKIMFingerprint(d *DKIMDomain) error {
	if strings.TrimSpace(d.PrivateKeyRef) == "" {
		return nil
	}
	_, fingerprint, err := DKIMPublicRecord(d.PrivateKeyRef)
	if err != nil {
		return err
	}
	d.PublicKeyFingerprint = fingerprint
	return nil
}

// ListSuppressions returns suppression entries, optionally filtered by a
// case-insensitive substring of the suppressed value (email/domain).
func (uc *DomainSafetyUsecase) ListSuppressions(ctx context.Context, search string, page Page) ([]*SuppressionEntry, error) {
	if _, err := RequirePermission(ctx, PermSuppressionRead); err != nil {
		return nil, err
	}
	search = strings.ToLower(strings.TrimSpace(search))
	return uc.repo.ListSuppressions(ctx, search, page)
}

// SuppressionDSNMessages returns the raw DSN notifications archived for the
// recipient behind a suppression, so an operator can inspect the full
// asynchronous bounce. Empty when the suppression isn't dsn-sourced or nothing
// was archived.
func (uc *DomainSafetyUsecase) SuppressionDSNMessages(ctx context.Context, suppressionID string) ([]*DSNMessage, error) {
	if _, err := RequirePermission(ctx, PermSuppressionRead); err != nil {
		return nil, err
	}
	value, err := uc.repo.SuppressionValueByID(ctx, suppressionID)
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, nil
	}
	return uc.repo.ListDSNMessages(ctx, value, 20)
}

// CreateSuppression validates and persists a suppression entry.
func (uc *DomainSafetyUsecase) CreateSuppression(ctx context.Context, s *SuppressionEntry) (*SuppressionEntry, error) {
	if _, err := RequirePermission(ctx, PermSuppressionWrite); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateSuppression(ctx, s)
	if err != nil {
		uc.audit(ctx, "suppression.create", "suppression", s.Value, AuditFailure, map[string]any{"type": s.Type})
		return nil, err
	}
	uc.audit(ctx, "suppression.create", "suppression", out.ID, AuditSuccess, map[string]any{
		"type": out.Type, "value": out.Value, "source": out.Source,
	})
	return out, nil
}

// UpdateSuppression updates the editable fields (reason, status) of an entry.
// The type and value are immutable (they identify the suppression).
func (uc *DomainSafetyUsecase) UpdateSuppression(ctx context.Context, id string, s *SuppressionEntry) (*SuppressionEntry, error) {
	if _, err := RequirePermission(ctx, PermSuppressionWrite); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, Invalid("SUPPRESSION_ID_REQUIRED", "suppression id is required")
	}
	if s.Status == "" {
		s.Status = SuppressActive
	}
	switch s.Status {
	case SuppressActive, SuppressDisabled, SuppressExpired:
	default:
		return nil, Invalid("SUPPRESSION_STATUS_INVALID", "status %q is not valid", s.Status)
	}
	out, err := uc.repo.UpdateSuppression(ctx, id, s)
	if err != nil {
		uc.audit(ctx, "suppression.update", "suppression", id, AuditFailure, map[string]any{"status": s.Status})
		return nil, err
	}
	uc.audit(ctx, "suppression.update", "suppression", out.ID, AuditSuccess, map[string]any{
		"type": out.Type, "value": out.Value, "status": out.Status,
	})
	return out, nil
}

// ListTLSPolicies returns the require-TLS destination-domain policies.
func (uc *DomainSafetyUsecase) ListTLSPolicies(ctx context.Context, page Page) ([]*TLSPolicy, error) {
	if _, err := RequirePermission(ctx, PermDKIMRead); err != nil {
		return nil, err
	}
	return uc.repo.ListTLSPolicies(ctx, page)
}

// CreateTLSPolicy validates and persists a require-TLS domain policy.
func (uc *DomainSafetyUsecase) CreateTLSPolicy(ctx context.Context, p *TLSPolicy) (*TLSPolicy, error) {
	if _, err := RequirePermission(ctx, PermDKIMWrite); err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	out, err := uc.repo.CreateTLSPolicy(ctx, p)
	if err != nil {
		uc.audit(ctx, "tls_policy.create", "tls_policy", p.Domain, AuditFailure, map[string]any{"domain": p.Domain})
		return nil, err
	}
	uc.audit(ctx, "tls_policy.create", "tls_policy", out.ID, AuditSuccess, map[string]any{
		"domain": out.Domain, "mode": out.Mode, "status": out.Status,
	})
	return out, nil
}

// DeleteTLSPolicy removes a require-TLS domain policy by id.
func (uc *DomainSafetyUsecase) DeleteTLSPolicy(ctx context.Context, id string) error {
	if _, err := RequirePermission(ctx, PermDKIMWrite); err != nil {
		return err
	}
	if id == "" {
		return Invalid("TLS_POLICY_ID_REQUIRED", "tls policy id is required")
	}
	if err := uc.repo.DeleteTLSPolicy(ctx, id); err != nil {
		uc.audit(ctx, "tls_policy.delete", "tls_policy", id, AuditFailure, nil)
		return err
	}
	uc.audit(ctx, "tls_policy.delete", "tls_policy", id, AuditSuccess, nil)
	return nil
}

// IsRecipientEligible reports whether a recipient may receive outbound mail.
// It returns false when an active suppression matches the recipient. This is
// the integration point used by outbound send-eligibility logic (US1/US4).
func (uc *DomainSafetyUsecase) IsRecipientEligible(ctx context.Context, recipient string) (bool, error) {
	suppressed, err := uc.repo.IsSuppressed(ctx, recipient)
	if err != nil {
		return false, err
	}
	return !suppressed, nil
}

func (uc *DomainSafetyUsecase) audit(ctx context.Context, op, targetType, targetID string, outcome AuditOutcome, summary map[string]any) {
	if uc.auditor == nil {
		return
	}
	if err := uc.auditor.Record(ctx, op, targetType, targetID, outcome, summary); err != nil {
		LoggerFrom(ctx).Error("audit write failed", "op", op, "error", err.Error())
	}
}
