package integration

import (
	"context"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// stubProducer satisfies biz.CommandProducer without touching Redis; security
// tests must be rejected before any command is enqueued.
type stubProducer struct{ published int }

func (s *stubProducer) PublishQueueCommand(context.Context, string, string, string) (string, error) {
	s.published++
	return "q", nil
}
func (s *stubProducer) PublishServiceCommand(context.Context, string, string) (string, error) {
	s.published++
	return "s", nil
}

// TestUnauthorizedServiceControlAndQueueDenied verifies that callers lacking the
// queue:control or service:control permission cannot enqueue commands, and that
// no command is published when authorization fails.
func TestUnauthorizedServiceControlAndQueueDenied(t *testing.T) {
	db := setupDB(t)
	prod := &stubProducer{}
	uc := biz.NewMailOpsUsecase(data.NewMailOpsRepo(db), prod, nil)

	// Identity with only read permissions.
	ctx := biz.WithIdentity(context.Background(), &biz.Identity{
		UserID:      "00000000-0000-0000-0000-000000000001",
		Permissions: biz.NewPermissionSet([]string{string(biz.PermMailRead), string(biz.PermQueueRead)}),
		MFAVerified: true,
	})

	if _, err := uc.RequestQueueAction(ctx, "suspend", "example.com", "", "c1"); err == nil {
		t.Fatal("expected queue action to be denied")
	} else if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}

	if _, err := uc.RequestServiceControl(ctx, "reload", "c1"); err == nil {
		t.Fatal("expected service control to be denied")
	} else if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindForbidden {
		t.Fatalf("expected forbidden, got %v", err)
	}

	if prod.published != 0 {
		t.Fatalf("no commands should be published on denial, got %d", prod.published)
	}
}

// TestServiceControlRequiresConfirmation verifies confirmation is mandatory.
func TestServiceControlRequiresConfirmation(t *testing.T) {
	db := setupDB(t)
	uc := biz.NewMailOpsUsecase(data.NewMailOpsRepo(db), &stubProducer{}, nil)
	_, err := uc.RequestServiceControl(ownerCtx(), "reload", "")
	if de, ok := biz.AsDomainError(err); !ok || de.Reason != "CONFIRMATION_REQUIRED" {
		t.Fatalf("expected CONFIRMATION_REQUIRED, got %v", err)
	}
}
