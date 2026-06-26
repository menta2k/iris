package contract

import (
	"strings"
	"testing"

	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

// TestListenerVMTAContract verifies the listener CRUD and that a VMTA attaches
// to a listener, resolving its IP/EHLO from it, and that the generated policy
// emits the listener block + egress source from the listener.
func TestListenerVMTAContract(t *testing.T) {
	svc := newService(t)
	ctx := ownerCtx()

	l, err := svc.CreateListener(ctx, &adminv1.CreateListenerRequest{
		Name: "mx-public", IpAddress: "203.0.113.50", Port: 25, Hostname: "mta1.example.com",
	})
	if err != nil {
		t.Fatalf("CreateListener: %v", err)
	}
	if l.GetId() == "" || l.GetIpAddress() != "203.0.113.50" || l.GetStatus() != "active" {
		t.Fatalf("unexpected listener: %+v", l)
	}

	// VMTA owns its egress ip/ehlo; the listener is an optional association.
	v, err := svc.CreateVMTA(ctx, &adminv1.CreateVMTARequest{
		Name: "vmta-fast", IpAddress: "203.0.113.50", EhloName: "mta1.example.com",
		ListenerId: l.GetId(), MaxConnections: 10,
	})
	if err != nil {
		t.Fatalf("CreateVMTA: %v", err)
	}
	if v.GetIpAddress() != "203.0.113.50" || v.GetEhloName() != "mta1.example.com" ||
		v.GetListenerName() != "mx-public" || v.GetMaxConnections() != 10 {
		t.Fatalf("VMTA did not persist its egress fields / listener name: %+v", v)
	}

	// A VMTA referencing a missing listener is rejected.
	if _, err := svc.CreateVMTA(ctx, &adminv1.CreateVMTARequest{
		Name: "bad", ListenerId: "00000000-0000-0000-0000-0000000000ff",
	}); err == nil {
		t.Fatal("expected error for a VMTA attached to a missing listener")
	}

	// The generated policy emits the listener and an egress source from it.
	cfg, err := svc.GenerateKumoConfig(ctx, &adminv1.GenerateKumoConfigRequest{})
	if err != nil {
		t.Fatalf("GenerateKumoConfig: %v", err)
	}
	if !cfg.GetValid() {
		t.Fatalf("policy invalid: %v", cfg.GetLintIssues())
	}
	if !strings.Contains(cfg.GetContent(), `listen = "203.0.113.50:25"`) ||
		!strings.Contains(cfg.GetContent(), `SOURCES["vmta-fast"] = { source_address = "203.0.113.50", ehlo_domain = "mta1.example.com" }`) ||
		!strings.Contains(cfg.GetContent(), `SOURCE_LIMITS["vmta-fast"] = 10`) {
		t.Fatalf("policy missing listener/egress/limit wiring:\n%s", cfg.GetContent())
	}

	// Edit the listener (e.g. change hostname); listing reflects it.
	if _, err := svc.UpdateListener(ctx, &adminv1.UpdateListenerRequest{
		Id: l.GetId(), Name: "mx-public", IpAddress: "203.0.113.50", Port: 25,
		Hostname: "mta2.example.com", Status: "active",
	}); err != nil {
		t.Fatalf("UpdateListener: %v", err)
	}
	list, err := svc.ListListeners(ctx, &adminv1.ListListenersRequest{})
	if err != nil {
		t.Fatalf("ListListeners: %v", err)
	}
	if len(list.GetItems()) != 1 || list.GetItems()[0].GetHostname() != "mta2.example.com" {
		t.Fatalf("listener edit not reflected: %+v", list.GetItems())
	}
}
