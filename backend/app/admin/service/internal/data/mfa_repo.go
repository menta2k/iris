// MfaRepo persists MFA credentials (the mfa_credentials table).
package data

import (
	"context"
	"fmt"
	"time"

	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent"
	"github.com/menta2k/iris/backend/app/admin/service/internal/data/ent/mfacredential"
	"github.com/menta2k/iris/backend/app/admin/service/internal/service"
)

type MfaRepo struct{ client *ent.Client }

func NewMfaRepo(c *ent.Client) *MfaRepo { return &MfaRepo{client: c} }

func (r *MfaRepo) HasActiveMFA(ctx context.Context, userID uint32) (bool, error) {
	n, err := r.client.MfaCredential.Query().
		Where(
			mfacredential.UserIDEQ(userID),
			mfacredential.StatusEQ(mfacredential.StatusActive),
			mfacredential.KindIn(mfacredential.KindTotp, mfacredential.KindWebauthn),
		).
		Count(ctx)
	if err != nil {
		return false, fmt.Errorf("mfa_repo: has_active: %w", err)
	}
	return n > 0, nil
}

func (r *MfaRepo) CountActive(ctx context.Context, userID uint32, kind string) (int, error) {
	n, err := r.client.MfaCredential.Query().
		Where(
			mfacredential.UserIDEQ(userID),
			mfacredential.StatusEQ(mfacredential.StatusActive),
			mfacredential.KindEQ(mfacredential.Kind(kind)),
		).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("mfa_repo: count_active: %w", err)
	}
	return n, nil
}

func (r *MfaRepo) ListActive(ctx context.Context, userID uint32, kind string) ([]service.MFACredentialRow, error) {
	rows, err := r.client.MfaCredential.Query().
		Where(
			mfacredential.UserIDEQ(userID),
			mfacredential.StatusEQ(mfacredential.StatusActive),
			mfacredential.KindEQ(mfacredential.Kind(kind)),
		).
		Order(ent.Asc(mfacredential.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("mfa_repo: list_active: %w", err)
	}
	out := make([]service.MFACredentialRow, 0, len(rows))
	for _, e := range rows {
		out = append(out, mfaToRow(e))
	}
	return out, nil
}

func (r *MfaRepo) GetActiveTOTP(ctx context.Context, userID uint32) (*service.MFACredentialRow, error) {
	e, err := r.client.MfaCredential.Query().
		Where(
			mfacredential.UserIDEQ(userID),
			mfacredential.StatusEQ(mfacredential.StatusActive),
			mfacredential.KindEQ(mfacredential.KindTotp),
		).
		Order(ent.Desc(mfacredential.FieldID)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("mfa_repo: get_active_totp: %w", err)
	}
	row := mfaToRow(e)
	return &row, nil
}

func (r *MfaRepo) GetActive(ctx context.Context, userID, id uint32) (*service.MFACredentialRow, error) {
	e, err := r.client.MfaCredential.Query().
		Where(
			mfacredential.IDEQ(int(id)),
			mfacredential.UserIDEQ(userID),
			mfacredential.StatusEQ(mfacredential.StatusActive),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("mfa_repo: get_active: %w", err)
	}
	row := mfaToRow(e)
	return &row, nil
}

func (r *MfaRepo) Create(ctx context.Context, in service.MFACredentialRow) (*service.MFACredentialRow, error) {
	c := r.client.MfaCredential.Create().
		SetUserID(in.UserID).
		SetKind(mfacredential.Kind(in.Kind)).
		SetSecret(in.Secret).
		SetLabel(in.Label).
		SetSignCount(in.SignCount)
	if in.ConfirmedAt != nil {
		c = c.SetConfirmedAt(*in.ConfirmedAt)
	}
	saved, err := c.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mfa_repo: create: %w", err)
	}
	row := mfaToRow(saved)
	return &row, nil
}

func (r *MfaRepo) Disable(ctx context.Context, id uint32) error {
	err := r.client.MfaCredential.UpdateOneID(int(id)).
		SetStatus(mfacredential.StatusDisabled).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mfa_repo: disable: %w", err)
	}
	return nil
}

func (r *MfaRepo) DisableByKind(ctx context.Context, userID uint32, kind string) error {
	_, err := r.client.MfaCredential.Update().
		Where(
			mfacredential.UserIDEQ(userID),
			mfacredential.StatusEQ(mfacredential.StatusActive),
			mfacredential.KindEQ(mfacredential.Kind(kind)),
		).
		SetStatus(mfacredential.StatusDisabled).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("mfa_repo: disable_by_kind: %w", err)
	}
	return nil
}

func (r *MfaRepo) DisableAll(ctx context.Context, userID uint32) error {
	_, err := r.client.MfaCredential.Update().
		Where(
			mfacredential.UserIDEQ(userID),
			mfacredential.StatusEQ(mfacredential.StatusActive),
		).
		SetStatus(mfacredential.StatusDisabled).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("mfa_repo: disable_all: %w", err)
	}
	return nil
}

func (r *MfaRepo) MarkUsed(ctx context.Context, id uint32) error {
	err := r.client.MfaCredential.UpdateOneID(int(id)).
		SetStatus(mfacredential.StatusDisabled).
		SetUsedAt(time.Now()).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mfa_repo: mark_used: %w", err)
	}
	return nil
}

func (r *MfaRepo) UpdateSecret(ctx context.Context, id uint32, secret string, signCount uint32) error {
	err := r.client.MfaCredential.UpdateOneID(int(id)).
		SetSecret(secret).
		SetSignCount(signCount).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mfa_repo: update_secret: %w", err)
	}
	return nil
}

func mfaToRow(e *ent.MfaCredential) service.MFACredentialRow {
	row := service.MFACredentialRow{
		ID:        uint32(e.ID),
		UserID:    e.UserID,
		Kind:      string(e.Kind),
		Secret:    e.Secret,
		Label:     e.Label,
		Status:    string(e.Status),
		SignCount: e.SignCount,
		CreatedAt: e.CreatedAt,
	}
	if e.ConfirmedAt != nil {
		t := *e.ConfirmedAt
		row.ConfirmedAt = &t
	}
	if e.UsedAt != nil {
		t := *e.UsedAt
		row.UsedAt = &t
	}
	return row
}

var _ service.MFAStore = (*MfaRepo)(nil)
