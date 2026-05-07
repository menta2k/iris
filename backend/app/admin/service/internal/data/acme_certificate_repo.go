// AcmeCertificateRepo persists issued / pending ACME certs.
package data

import (
	"context"
	"fmt"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/acmecertificate"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

type AcmeCertificateRepo struct{ client *ent.Client }

func NewAcmeCertificateRepo(c *ent.Client) *AcmeCertificateRepo {
	return &AcmeCertificateRepo{client: c}
}

func (r *AcmeCertificateRepo) List(ctx context.Context) ([]service.AcmeCertificateRow, error) {
	rows, err := r.client.AcmeCertificate.Query().
		Order(ent.Asc(acmecertificate.FieldDomain)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("acme_certificate_repo: list: %w", err)
	}
	out := make([]service.AcmeCertificateRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, acmeCertToRow(e))
	}
	return out, nil
}

func (r *AcmeCertificateRepo) Get(ctx context.Context, id uint32) (*service.AcmeCertificateRow, error) {
	e, err := r.client.AcmeCertificate.Get(ctx, int(id))
	if err != nil {
		return nil, fmt.Errorf("acme_certificate_repo: get: %w", err)
	}
	row := acmeCertToRow(e)
	return &row, nil
}

func (r *AcmeCertificateRepo) GetByDomain(ctx context.Context, domain string) (*service.AcmeCertificateRow, error) {
	e, err := r.client.AcmeCertificate.Query().
		Where(acmecertificate.DomainEQ(domain)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("acme_certificate_repo: get_by_domain: %w", err)
	}
	row := acmeCertToRow(e)
	return &row, nil
}

// Upsert keys on domain. Operators don't issue two certs for the same
// domain; the renewer overwrites in place.
func (r *AcmeCertificateRepo) Upsert(ctx context.Context, in service.AcmeCertificateRow) (*service.AcmeCertificateRow, error) {
	existing, err := r.client.AcmeCertificate.Query().
		Where(acmecertificate.DomainEQ(in.Domain)).
		Only(ctx)
	if err == nil {
		upd := r.client.AcmeCertificate.UpdateOneID(existing.ID).
			SetAltNames(append([]string(nil), in.AltNames...)).
			SetChallengeType(in.ChallengeType).
			SetDNSProvider(in.DnsProvider).
			SetCertPem(in.CertPEM).
			SetKeyPem(in.KeyPEM).
			SetCertPemPath(in.CertPemPath).
			SetKeyPemPath(in.KeyPemPath).
			SetStatus(in.Status).
			SetLastError(in.LastError)
		if in.ExpiresAt != nil {
			upd = upd.SetExpiresAt(*in.ExpiresAt)
		}
		if in.LastRenewedAt != nil {
			upd = upd.SetLastRenewedAt(*in.LastRenewedAt)
		}
		saved, err := upd.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("acme_certificate_repo: update: %w", err)
		}
		row := acmeCertToRow(saved)
		return &row, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("acme_certificate_repo: upsert lookup: %w", err)
	}
	create := r.client.AcmeCertificate.Create().
		SetDomain(in.Domain).
		SetAltNames(append([]string(nil), in.AltNames...)).
		SetChallengeType(nonEmpty(in.ChallengeType, "http-01")).
		SetDNSProvider(in.DnsProvider).
		SetCertPem(in.CertPEM).
		SetKeyPem(in.KeyPEM).
		SetCertPemPath(in.CertPemPath).
		SetKeyPemPath(in.KeyPemPath).
		SetStatus(nonEmpty(in.Status, "pending")).
		SetLastError(in.LastError)
	if in.ExpiresAt != nil {
		create = create.SetExpiresAt(*in.ExpiresAt)
	}
	if in.LastRenewedAt != nil {
		create = create.SetLastRenewedAt(*in.LastRenewedAt)
	}
	saved, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("acme_certificate_repo: create: %w", err)
	}
	row := acmeCertToRow(saved)
	return &row, nil
}

func (r *AcmeCertificateRepo) Delete(ctx context.Context, id uint32) error {
	if err := r.client.AcmeCertificate.DeleteOneID(int(id)).Exec(ctx); err != nil {
		return fmt.Errorf("acme_certificate_repo: delete: %w", err)
	}
	return nil
}

func acmeCertToRow(g *ent.AcmeCertificate) service.AcmeCertificateRow {
	row := service.AcmeCertificateRow{
		ID:            uint32(g.ID),
		Domain:        g.Domain,
		AltNames:      append([]string(nil), g.AltNames...),
		ChallengeType: g.ChallengeType,
		DnsProvider:   g.DNSProvider,
		CertPEM:       g.CertPem,
		KeyPEM:        g.KeyPem,
		CertPemPath:   g.CertPemPath,
		KeyPemPath:    g.KeyPemPath,
		Status:        g.Status,
		LastError:     g.LastError,
		CreatedAt:     g.CreatedAt,
		UpdatedAt:     g.UpdatedAt,
	}
	if g.ExpiresAt != nil {
		t := *g.ExpiresAt
		row.ExpiresAt = &t
	}
	if g.LastRenewedAt != nil {
		t := *g.LastRenewedAt
		row.LastRenewedAt = &t
	}
	return row
}

var _ service.AcmeCertificateStore = (*AcmeCertificateRepo)(nil)
