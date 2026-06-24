package biz

import "testing"

func TestFBLEndpointValidate(t *testing.T) {
	cases := []struct {
		name string
		in   FBLEndpoint
		want string // expected DomainError reason ("" = valid)
	}{
		{
			name: "valid awaiting",
			in:   FBLEndpoint{Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com", ForwardAddress: "ops@example.com", Status: FBLAwaitingApproval},
		},
		{
			name: "valid approved without forward",
			in:   FBLEndpoint{Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com", Status: FBLApproved},
		},
		{
			name: "status defaults to awaiting and then needs forward",
			in:   FBLEndpoint{Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com"},
			want: "FBL_FORWARD_ADDRESS_REQUIRED",
		},
		{
			name: "missing domain",
			in:   FBLEndpoint{FeedbackAddress: "fbl@fbl.example.com", Status: FBLApproved},
			want: "FBL_DOMAIN_REQUIRED",
		},
		{
			name: "invalid domain",
			in:   FBLEndpoint{Domain: "not a domain", FeedbackAddress: "fbl@fbl.example.com", Status: FBLApproved},
			want: "FBL_DOMAIN_INVALID",
		},
		{
			name: "missing feedback address",
			in:   FBLEndpoint{Domain: "fbl.example.com", Status: FBLApproved},
			want: "FBL_FEEDBACK_ADDRESS_REQUIRED",
		},
		{
			name: "invalid feedback address",
			in:   FBLEndpoint{Domain: "fbl.example.com", FeedbackAddress: "notanemail", Status: FBLApproved},
			want: "FBL_FEEDBACK_ADDRESS_INVALID",
		},
		{
			name: "feedback domain mismatch",
			in:   FBLEndpoint{Domain: "fbl.example.com", FeedbackAddress: "fbl@other.example.com", Status: FBLApproved},
			want: "FBL_FEEDBACK_DOMAIN_MISMATCH",
		},
		{
			name: "invalid forward address",
			in:   FBLEndpoint{Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com", ForwardAddress: "nope", Status: FBLAwaitingApproval},
			want: "FBL_FORWARD_ADDRESS_INVALID",
		},
		{
			name: "invalid status",
			in:   FBLEndpoint{Domain: "fbl.example.com", FeedbackAddress: "fbl@fbl.example.com", Status: "bogus"},
			want: "FBL_STATUS_INVALID",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.in
			assertReason(t, f.Validate(), tc.want)
		})
	}
}

func TestFBLEndpointValidateNormalizes(t *testing.T) {
	f := FBLEndpoint{
		Domain:          " FBL.Example.COM ",
		FeedbackAddress: " FBL@FBL.Example.COM ",
		ForwardAddress:  " OPS@Example.COM ",
		Status:          " AWAITING_APPROVAL ",
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Domain != "fbl.example.com" || f.FeedbackAddress != "fbl@fbl.example.com" ||
		f.ForwardAddress != "ops@example.com" || f.Status != FBLAwaitingApproval {
		t.Fatalf("not normalized: %+v", f)
	}
}
