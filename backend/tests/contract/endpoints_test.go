package contract

import (
	"testing"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

// TestMailOpsContract covers the mail, bounce, feedback, queue, and
// service-control handlers (T051).
func TestMailOpsContract(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()

	if _, err := svc.ListMailRecords(ctx, &adminv1.ListMailRecordsRequest{Mailclass: "bulk"}); err != nil {
		t.Fatalf("ListMailRecords: %v", err)
	}
	if _, err := svc.ListBounces(ctx, &adminv1.ListBouncesRequest{}); err != nil {
		t.Fatalf("ListBounces: %v", err)
	}
	if _, err := svc.ListFeedbackReports(ctx, &adminv1.ListFeedbackReportsRequest{}); err != nil {
		t.Fatalf("ListFeedbackReports: %v", err)
	}
	if _, err := svc.ListQueues(ctx, &adminv1.ListQueuesRequest{}); err != nil {
		t.Fatalf("ListQueues: %v", err)
	}

	q, err := svc.RequestQueueAction(ctx, &adminv1.RequestQueueActionRequest{
		Action: "suspend", Domain: "example.com",
	})
	if err != nil {
		t.Fatalf("RequestQueueAction: %v", err)
	}
	if q.GetStatus() != "ok" {
		t.Fatalf("expected ok queue action, got %q", q.GetStatus())
	}

	sc, err := svc.RequestServiceControl(ctx, &adminv1.RequestServiceControlRequest{
		Operation: "reload", ConfirmationId: "c2",
	})
	if err != nil {
		t.Fatalf("RequestServiceControl: %v", err)
	}
	if sc.GetId() == "" || sc.GetOperation() != "reload" {
		t.Fatalf("unexpected service-control reply: %+v", sc)
	}

	// Missing confirmation must be rejected.
	if _, err := svc.RequestServiceControl(ctx, &adminv1.RequestServiceControlRequest{Operation: "reload"}); err == nil {
		t.Fatal("expected confirmation-required error")
	}
}

// TestIdentityAndAuditContract covers users and audit handlers (T069).
func TestIdentityAndAuditContract(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()
	if _, err := svc.ListUsers(ctx, &adminv1.ListUsersRequest{}); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if _, err := svc.ListAuditEntries(ctx, &adminv1.ListAuditEntriesRequest{}); err != nil {
		t.Fatalf("ListAuditEntries: %v", err)
	}
}

// TestDomainSafetyContract covers DKIM and suppression handlers (T083).
func TestDomainSafetyContract(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()
	if _, err := svc.ListDKIMDomains(ctx, &adminv1.ListDKIMDomainsRequest{}); err != nil {
		t.Fatalf("ListDKIMDomains: %v", err)
	}
	if _, err := svc.ListSuppressions(ctx, &adminv1.ListSuppressionsRequest{}); err != nil {
		t.Fatalf("ListSuppressions: %v", err)
	}
}

// TestInboundContract covers the inbound-route and Rspamd handlers (T097).
func TestInboundContract(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()
	if _, err := svc.ListInboundRoutes(ctx, &adminv1.ListInboundRoutesRequest{}); err != nil {
		t.Fatalf("ListInboundRoutes: %v", err)
	}
	if _, err := svc.ListRspamdResults(ctx, &adminv1.ListRspamdResultsRequest{}); err != nil {
		t.Fatalf("ListRspamdResults: %v", err)
	}
}

// TestDashboardContract covers the dashboard summary handler (T112).
func TestDashboardContract(t *testing.T) {
	svc := newService(t)
	summary, err := svc.GetDashboardSummary(ownerCtx(), &adminv1.GetDashboardSummaryRequest{})
	if err != nil {
		t.Fatalf("GetDashboardSummary: %v", err)
	}
	if summary.GetServiceState() == "" {
		t.Fatalf("expected a service state, got empty")
	}
}

// TestUnauthenticatedRejected verifies handlers reject an unauthenticated ctx.
func TestUnauthenticatedRejected(t *testing.T) {
	svc := newService(t)
	if _, err := svc.ListVMTAs(t.Context(), &adminv1.ListVMTAsRequest{}); err == nil {
		t.Fatal("expected unauthenticated rejection")
	}
}
