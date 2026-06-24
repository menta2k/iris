package biz

import (
	"context"
	"testing"
)

func TestValidateQueueActionRequest(t *testing.T) {
	tests := []struct {
		name, mailclass, action, confirm, wantErr string
	}{
		{"valid", "bulk", "pause", "c1", ""},
		{"missing mailclass", "", "pause", "c1", "QUEUE_MAILCLASS_REQUIRED"},
		{"bad action", "bulk", "explode", "c1", "QUEUE_ACTION_INVALID"},
		{"missing confirmation", "bulk", "pause", "", "CONFIRMATION_REQUIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQueueActionRequest(tt.mailclass, tt.action, tt.confirm)
			assertReason(t, err, tt.wantErr)
		})
	}
}

func TestValidateServiceControlRequest(t *testing.T) {
	if err := ValidateServiceControlRequest("restart", "c1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertReason(t, ValidateServiceControlRequest("nuke", "c1"), "SERVICE_OPERATION_INVALID")
	assertReason(t, ValidateServiceControlRequest("restart", ""), "CONFIRMATION_REQUIRED")
}

func TestBounceIsHardBounce(t *testing.T) {
	if !(&BounceRecord{SMTPStatus: "550"}).IsHardBounce() {
		t.Fatal("550 should be a hard bounce")
	}
	if (&BounceRecord{SMTPStatus: "421"}).IsHardBounce() {
		t.Fatal("421 should not be a hard bounce")
	}
}

// fakeMailOpsRepo and fakeProducer support service-control concurrency tests.
type fakeMailOpsRepo struct {
	active  bool
	created []*ServiceControlRecord
	updates []string
}

func (f *fakeMailOpsRepo) ListMailRecords(context.Context, MailFilter, Page) ([]*MailRecord, error) {
	return nil, nil
}
func (f *fakeMailOpsRepo) ListBounces(context.Context, Page) ([]*BounceRecord, error) {
	return nil, nil
}
func (f *fakeMailOpsRepo) ListFeedbackReports(context.Context, Page) ([]*FeedbackReport, error) {
	return nil, nil
}
func (f *fakeMailOpsRepo) ListQueues(context.Context, Page) ([]*MailclassQueue, error) {
	return nil, nil
}
func (f *fakeMailOpsRepo) CreateServiceControlRequest(_ context.Context, rec *ServiceControlRecord) (*ServiceControlRecord, error) {
	rec.ID = "sc-1"
	rec.Status = SvcRequested
	f.created = append(f.created, rec)
	return rec, nil
}
func (f *fakeMailOpsRepo) ActiveServiceControlExists(context.Context) (bool, error) {
	return f.active, nil
}
func (f *fakeMailOpsRepo) UpdateServiceControlStatus(_ context.Context, id, status, _ string) error {
	f.updates = append(f.updates, id+":"+status)
	return nil
}

type fakeProducer struct{ queueCalls, svcCalls int }

func (f *fakeProducer) PublishQueueCommand(context.Context, string, string, string) (string, error) {
	f.queueCalls++
	return "q-1", nil
}
func (f *fakeProducer) PublishServiceCommand(context.Context, string, string) (string, error) {
	f.svcCalls++
	return "s-1", nil
}

func TestRequestServiceControlRejectsConcurrent(t *testing.T) {
	repo := &fakeMailOpsRepo{active: true}
	uc := NewMailOpsUsecase(repo, &fakeProducer{}, nil)
	_, err := uc.RequestServiceControl(ownerCtx(), "restart", "c1")
	assertReasonKind(t, err, "SERVICE_CONTROL_ACTIVE", KindConflict)
}

func TestRequestServiceControlSucceeds(t *testing.T) {
	repo := &fakeMailOpsRepo{}
	prod := &fakeProducer{}
	uc := NewMailOpsUsecase(repo, prod, nil)
	rec, err := uc.RequestServiceControl(ownerCtx(), "restart", "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ID != "sc-1" || prod.svcCalls != 1 {
		t.Fatalf("expected request persisted and enqueued, got id=%q calls=%d", rec.ID, prod.svcCalls)
	}
}

func TestRequestQueueActionDeniedWithoutPermission(t *testing.T) {
	uc := NewMailOpsUsecase(&fakeMailOpsRepo{}, &fakeProducer{}, nil)
	ctx := WithIdentity(context.Background(), &Identity{
		Permissions: NewPermissionSet([]string{string(PermMailRead)}), MFAVerified: true,
	})
	_, err := uc.RequestQueueAction(ctx, "suspend", "example.com", "", "c1")
	if de, ok := AsDomainError(err); !ok || de.Kind != KindForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func assertReason(t *testing.T, err error, wantReason string) {
	t.Helper()
	if wantReason == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	de, ok := AsDomainError(err)
	if !ok || de.Reason != wantReason {
		t.Fatalf("expected reason %q, got %v", wantReason, err)
	}
}

func assertReasonKind(t *testing.T, err error, wantReason string, wantKind ErrorKind) {
	t.Helper()
	de, ok := AsDomainError(err)
	if !ok || de.Reason != wantReason || de.Kind != wantKind {
		t.Fatalf("expected reason %q kind %d, got %v", wantReason, wantKind, err)
	}
}
