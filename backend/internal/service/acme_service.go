package service

import (
	"context"
	"time"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
	"github.com/menta2k/iris/backend/internal/biz"
)

// fmtTimePtr formats a nullable timestamp as RFC3339, or "" when nil.
func fmtTimePtr(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z07:00")
}

// GetAcmeAccount returns the ACME account (secrets stripped).
func (s *Service) GetAcmeAccount(ctx context.Context, _ *adminv1.GetAcmeAccountRequest) (*adminv1.AcmeAccount, error) {
	if s.acme == nil {
		return nil, notImplemented("GetAcmeAccount")
	}
	acc, err := s.acme.GetAccount(ctx)
	if err != nil {
		return nil, s.fail(ctx, "GetAcmeAccount", err)
	}
	return acmeAccountToProto(acc), nil
}

// SaveAcmeAccount sets the account email + directory URL.
func (s *Service) SaveAcmeAccount(ctx context.Context, req *adminv1.SaveAcmeAccountRequest) (*adminv1.AcmeAccount, error) {
	if s.acme == nil {
		return nil, notImplemented("SaveAcmeAccount")
	}
	if err := s.acme.SaveAccount(ctx, req.GetEmail(), req.GetServerUrl()); err != nil {
		return nil, s.fail(ctx, "SaveAcmeAccount", err)
	}
	acc, err := s.acme.GetAccount(ctx)
	if err != nil {
		return nil, s.fail(ctx, "SaveAcmeAccount", err)
	}
	return acmeAccountToProto(acc), nil
}

// ListAcmeCertificates lists issued certificates.
func (s *Service) ListAcmeCertificates(ctx context.Context, _ *adminv1.ListAcmeCertificatesRequest) (*adminv1.ListAcmeCertificatesReply, error) {
	if s.acme == nil {
		return nil, notImplemented("ListAcmeCertificates")
	}
	items, err := s.acme.ListCertificates(ctx)
	if err != nil {
		return nil, s.fail(ctx, "ListAcmeCertificates", err)
	}
	out := &adminv1.ListAcmeCertificatesReply{}
	for _, c := range items {
		out.Items = append(out.Items, acmeCertToProto(c))
	}
	return out, nil
}

// RequestAcmeCertificate issues (or re-issues) a certificate via HTTP-01.
func (s *Service) RequestAcmeCertificate(ctx context.Context, req *adminv1.RequestAcmeCertificateRequest) (*adminv1.AcmeCertificate, error) {
	if s.acme == nil {
		return nil, notImplemented("RequestAcmeCertificate")
	}
	c, err := s.acme.RequestCertificate(ctx, req.GetDomain(), req.GetAltNames())
	if err != nil {
		return nil, s.fail(ctx, "RequestAcmeCertificate", err)
	}
	return acmeCertToProto(c), nil
}

// DeleteAcmeCertificate removes a certificate record.
func (s *Service) DeleteAcmeCertificate(ctx context.Context, req *adminv1.DeleteAcmeCertificateRequest) (*adminv1.DeleteAcmeCertificateReply, error) {
	if s.acme == nil {
		return nil, notImplemented("DeleteAcmeCertificate")
	}
	if err := s.acme.DeleteCertificate(ctx, req.GetId()); err != nil {
		return nil, s.fail(ctx, "DeleteAcmeCertificate", err)
	}
	return &adminv1.DeleteAcmeCertificateReply{}, nil
}

func acmeAccountToProto(a *biz.AcmeAccount) *adminv1.AcmeAccount {
	updatedAt := ""
	if !a.UpdatedAt.IsZero() {
		updatedAt = a.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return &adminv1.AcmeAccount{
		Email:      a.Email,
		ServerUrl:  a.ServerURL,
		Configured: a.Configured(),
		Registered: a.RegistrationJSON != "",
		UpdatedAt:  updatedAt,
	}
}

func acmeCertToProto(c *biz.AcmeCertificate) *adminv1.AcmeCertificate {
	return &adminv1.AcmeCertificate{
		Id:            c.ID,
		Domain:        c.Domain,
		AltNames:      c.AltNames,
		ChallengeType: c.ChallengeType,
		CertPath:      c.CertPath,
		KeyPath:       c.KeyPath,
		ExpiresAt:     fmtTimePtr(c.ExpiresAt),
		LastRenewedAt: fmtTimePtr(c.LastRenewedAt),
		Status:        c.Status,
		LastError:     c.LastError,
	}
}
