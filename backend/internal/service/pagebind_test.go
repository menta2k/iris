package service

import (
	"net/url"
	"testing"

	"github.com/go-kratos/kratos/v2/transport/http/binding"
	adminv1 "github.com/menta2k/iris/backend/api/iris/admin/v1"
)

func TestPageQueryBinding(t *testing.T) {
	vals := url.Values{
		"page.page_size":  {"10"},
		"page.page_token": {"abc123"},
		"mailclass":       {"promo"},
		"vmta_id":         {"vmta-1"},
	}
	var req adminv1.ListMailRecordsRequest
	if err := binding.BindQuery(vals, &req); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if req.GetPage().GetPageSize() != 10 {
		t.Fatalf("page_size: got %d want 10", req.GetPage().GetPageSize())
	}
	if req.GetPage().GetPageToken() != "abc123" {
		t.Fatalf("page_token: got %q", req.GetPage().GetPageToken())
	}
	if req.GetMailclass() != "promo" || req.GetVmtaId() != "vmta-1" {
		t.Fatalf("filters not bound: %+v", &req)
	}
	// camelCase nested key must also work.
	var req2 adminv1.ListMailRecordsRequest
	_ = binding.BindQuery(url.Values{"page.pageSize": {"25"}}, &req2)
	if req2.GetPage().GetPageSize() != 25 {
		t.Fatalf("camelCase page.pageSize not bound: %d", req2.GetPage().GetPageSize())
	}
}
