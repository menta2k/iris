package biz

import (
	"context"
	"testing"
)

// fakeFBLRepo is an in-memory FBLRepo for use-case tests.
type fakeFBLRepo struct {
	created  *FBLEndpoint
	updated  *FBLEndpoint
	deleted  string
	listResp []*FBLEndpoint
}

func (f *fakeFBLRepo) ListFBLEndpoints(context.Context, Page) ([]*FBLEndpoint, error) {
	return f.listResp, nil
}
func (f *fakeFBLRepo) ListFBLEndpointsForPolicy(context.Context) ([]*FBLEndpoint, error) {
	return f.listResp, nil
}
func (f *fakeFBLRepo) CreateFBLEndpoint(_ context.Context, e *FBLEndpoint) (*FBLEndpoint, error) {
	f.created = e
	out := *e
	out.ID = "fbl-1"
	return &out, nil
}
func (f *fakeFBLRepo) UpdateFBLEndpoint(_ context.Context, id string, e *FBLEndpoint) (*FBLEndpoint, error) {
	f.updated = e
	out := *e
	out.ID = id
	return &out, nil
}
func (f *fakeFBLRepo) DeleteFBLEndpoint(_ context.Context, id string) error {
	f.deleted = id
	return nil
}

func TestFBLUsecaseCreateRequiresPermission(t *testing.T) {
	uc := NewFBLUsecase(&fakeFBLRepo{}, nil)
	// No identity in context → denied.
	_, err := uc.Create(context.Background(), &FBLEndpoint{
		Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com", Status: FBLApproved,
	})
	if _, ok := AsDomainError(err); !ok || err == nil {
		t.Fatalf("expected authorization error, got %v", err)
	}
}

func TestFBLUsecaseCreateValidates(t *testing.T) {
	uc := NewFBLUsecase(&fakeFBLRepo{}, nil)
	_, err := uc.Create(ownerCtx(), &FBLEndpoint{Domain: "", FeedbackAddress: "fbl@fbl.example.com"})
	de, ok := AsDomainError(err)
	if !ok || de.Reason != "FBL_DOMAIN_REQUIRED" {
		t.Fatalf("expected FBL_DOMAIN_REQUIRED, got %v", err)
	}
}

func TestFBLUsecaseCRUDHappyPath(t *testing.T) {
	repo := &fakeFBLRepo{}
	uc := NewFBLUsecase(repo, nil)
	ctx := ownerCtx()

	out, err := uc.Create(ctx, &FBLEndpoint{
		Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com", ForwardAddress: "ops@example.com", Status: FBLAwaitingApproval,
	})
	if err != nil || out.ID != "fbl-1" {
		t.Fatalf("create: out=%+v err=%v", out, err)
	}

	if _, err := uc.Update(ctx, "fbl-1", &FBLEndpoint{
		Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com", Status: FBLApproved,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if repo.updated == nil || repo.updated.Status != FBLApproved {
		t.Fatalf("update not applied: %+v", repo.updated)
	}

	if err := uc.Delete(ctx, "fbl-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if repo.deleted != "fbl-1" {
		t.Fatalf("delete id: got %q", repo.deleted)
	}
}
