package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// ListDKIMDomains returns DKIM domain configurations (US4).
func (s *Service) ListDKIMDomains(ctx context.Context, req *adminv1.ListDKIMDomainsRequest) (*adminv1.ListDKIMDomainsReply, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("ListDKIMDomains")
	}
	page := pageFrom(req.GetPage())
	items, err := s.domainSafety.ListDKIMDomains(ctx, page)
	if err != nil {
		return nil, s.fail(ctx, "ListDKIMDomains", err)
	}
	out := &adminv1.ListDKIMDomainsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, d := range items {
		out.Items = append(out.Items, dkimToProto(d))
	}
	return out, nil
}

// CreateDKIMDomain creates a DKIM domain configuration (US4).
func (s *Service) CreateDKIMDomain(ctx context.Context, req *adminv1.CreateDKIMDomainRequest) (*adminv1.DKIMDomain, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("CreateDKIMDomain")
	}
	out, err := s.domainSafety.CreateDKIMDomain(ctx, &biz.DKIMDomain{
		Domain:               req.GetDomain(),
		Selector:             req.GetSelector(),
		PublicKeyFingerprint: req.GetPublicKeyFingerprint(),
		PrivateKeyRef:        req.GetPrivateKeyRef(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateDKIMDomain", err)
	}
	return dkimToProto(out), nil
}

// UpdateDKIMDomain updates an existing DKIM configuration (US4).
func (s *Service) UpdateDKIMDomain(ctx context.Context, req *adminv1.UpdateDKIMDomainRequest) (*adminv1.DKIMDomain, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("UpdateDKIMDomain")
	}
	out, err := s.domainSafety.UpdateDKIMDomain(ctx, req.GetId(), &biz.DKIMDomain{
		Selector:             req.GetSelector(),
		PublicKeyFingerprint: req.GetPublicKeyFingerprint(),
		PrivateKeyRef:        req.GetPrivateKeyRef(),
		Status:               req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateDKIMDomain", err)
	}
	return dkimToProto(out), nil
}

// ListSuppressions returns suppression entries (US4).
func (s *Service) ListSuppressions(ctx context.Context, req *adminv1.ListSuppressionsRequest) (*adminv1.ListSuppressionsReply, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("ListSuppressions")
	}
	page := pageFrom(req.GetPage())
	f := biz.SuppressionFilter{
		Search:    req.GetSearch(),
		Type:      req.GetType(),
		Status:    req.GetStatus(),
		Source:    req.GetSource(),
		Mailclass: req.GetMailclass(),
		Expiry:    req.GetExpiry(),
		Sort:      req.GetSort(),
		Desc:      req.GetDesc(),
	}
	items, err := s.domainSafety.ListSuppressions(ctx, f, page)
	if err != nil {
		return nil, s.fail(ctx, "ListSuppressions", err)
	}
	out := &adminv1.ListSuppressionsReply{Page: &adminv1.PageReply{NextPageToken: page.NextToken(len(items))}}
	for _, e := range items {
		out.Items = append(out.Items, suppressionToProto(e))
	}
	return out, nil
}

// CreateSuppression creates a suppression entry (US4).
func (s *Service) CreateSuppression(ctx context.Context, req *adminv1.CreateSuppressionRequest) (*adminv1.Suppression, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("CreateSuppression")
	}
	out, err := s.domainSafety.CreateSuppression(ctx, &biz.SuppressionEntry{
		Type:   req.GetType(),
		Value:  req.GetValue(),
		Reason: req.GetReason(),
	})
	if err != nil {
		return nil, s.fail(ctx, "CreateSuppression", err)
	}
	return suppressionToProto(out), nil
}

// UpdateSuppression updates an existing suppression entry (US4).
func (s *Service) UpdateSuppression(ctx context.Context, req *adminv1.UpdateSuppressionRequest) (*adminv1.Suppression, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("UpdateSuppression")
	}
	out, err := s.domainSafety.UpdateSuppression(ctx, req.GetId(), &biz.SuppressionEntry{
		Reason: req.GetReason(),
		Status: req.GetStatus(),
	})
	if err != nil {
		return nil, s.fail(ctx, "UpdateSuppression", err)
	}
	return suppressionToProto(out), nil
}

// GenerateDKIMKey mints a key pair and returns the private key plus the DNS TXT
// record to publish. Nothing is persisted; the caller saves the key on a domain.
func (s *Service) GenerateDKIMKey(ctx context.Context, req *adminv1.GenerateDKIMKeyRequest) (*adminv1.GenerateDKIMKeyReply, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("GenerateDKIMKey")
	}
	out, err := s.domainSafety.GenerateDKIMKey(ctx, req.GetDomain(), req.GetSelector())
	if err != nil {
		return nil, s.fail(ctx, "GenerateDKIMKey", err)
	}
	return &adminv1.GenerateDKIMKeyReply{
		PrivateKeyPem:        out.PrivateKeyPEM,
		RecordName:           out.RecordName,
		RecordValue:          out.RecordValue,
		PublicKeyFingerprint: out.Fingerprint,
	}, nil
}

func dkimToProto(d *biz.DKIMDomain) *adminv1.DKIMDomain {
	return &adminv1.DKIMDomain{
		Id: d.ID, Domain: d.Domain, Selector: d.Selector,
		PublicKeyFingerprint: d.PublicKeyFingerprint, Status: d.Status,
	}
}

// ListSuppressionDsnMessages returns the raw DSN notifications behind a
// dsn-sourced suppression (US4).
func (s *Service) ListSuppressionDsnMessages(ctx context.Context, req *adminv1.ListSuppressionDsnMessagesRequest) (*adminv1.ListSuppressionDsnMessagesReply, error) {
	if s.domainSafety == nil {
		return nil, notImplemented("ListSuppressionDsnMessages")
	}
	msgs, err := s.domainSafety.SuppressionDSNMessages(ctx, req.GetId())
	if err != nil {
		return nil, s.fail(ctx, "ListSuppressionDsnMessages", err)
	}
	out := &adminv1.ListSuppressionDsnMessagesReply{}
	for _, m := range msgs {
		out.Items = append(out.Items, &adminv1.DsnMessage{
			Id:         m.ID,
			MessageId:  m.MessageID,
			RawMessage: m.RawMessage,
			ReceivedAt: m.ReceivedAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func suppressionToProto(e *biz.SuppressionEntry) *adminv1.Suppression {
	createdAt := ""
	if !e.CreatedAt.IsZero() {
		createdAt = e.CreatedAt.UTC().Format(time.RFC3339)
	}
	expiresAt := ""
	if e.ExpiresAt != nil {
		expiresAt = e.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return &adminv1.Suppression{
		Id: e.ID, Type: e.Type, Value: e.Value, Reason: e.Reason, Source: e.Source, Status: e.Status,
		Mailclass: e.Mailclass, CreatedAt: createdAt, ExpiresAt: expiresAt,
	}
}
