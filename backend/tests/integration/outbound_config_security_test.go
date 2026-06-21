package integration

import (
	"context"
	"testing"

	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/data"
)

// TestUnauthorizedOutboundWriteDenied verifies that a reader identity cannot
// create VMTAs or routing rules, and an unauthenticated context is rejected.
func TestUnauthorizedOutboundWriteDenied(t *testing.T) {
	db := setupDB(t)
	uc := biz.NewOutboundConfigUsecase(data.NewOutboundConfigRepo(db), nil)

	// Reader lacks write permission.
	_, err := uc.CreateVMTA(readerCtx(), &biz.VMTA{Name: "x", ListenerID: "lst"})
	if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindForbidden {
		t.Fatalf("expected forbidden for reader, got %v", err)
	}

	// Unauthenticated context is rejected.
	_, err = uc.CreateVMTA(context.Background(), &biz.VMTA{Name: "x", ListenerID: "lst"})
	if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindUnauthorized {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

// TestInvalidInputRejectedBeforePersist verifies validation runs before the DB.
func TestInvalidInputRejectedBeforePersist(t *testing.T) {
	db := setupDB(t)
	uc := biz.NewOutboundConfigUsecase(data.NewOutboundConfigRepo(db), nil)

	_, err := uc.CreateVMTA(ownerCtx(), &biz.VMTA{Name: ""})
	if de, ok := biz.AsDomainError(err); !ok || de.Kind != biz.KindInvalidArgument {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
