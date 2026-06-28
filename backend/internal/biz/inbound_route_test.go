package biz

import "testing"

func TestInboundRouteValidate(t *testing.T) {
	tests := []struct {
		name          string
		route         InboundRoute
		allowInsecure bool
		wantErr       bool
	}{
		{
			name:  "maildir domain default path",
			route: InboundRoute{Name: "md", MatchType: MatchRecipientDomain, MatchValue: "Example.com", Action: InboundActionMaildir},
		},
		{
			name:  "maildir explicit absolute path",
			route: InboundRoute{Name: "md", MatchType: MatchRecipientEmail, MatchValue: "a@example.com", Action: InboundActionMaildir, MaildirPath: "/var/mail/a"},
		},
		{
			name:    "maildir relative path rejected",
			route:   InboundRoute{Name: "md", MatchType: MatchRecipientDomain, MatchValue: "example.com", Action: InboundActionMaildir, MaildirPath: "var/mail"},
			wantErr: true,
		},
		{
			name:    "maildir traversal rejected",
			route:   InboundRoute{Name: "md", MatchType: MatchRecipientDomain, MatchValue: "example.com", Action: InboundActionMaildir, MaildirPath: "/var/../etc"},
			wantErr: true,
		},
		{
			name:  "forward host+port defaults tls",
			route: InboundRoute{Name: "fw", MatchType: MatchRecipientDomain, MatchValue: "example.com", Action: InboundActionForward, ForwardHost: "mail.internal", ForwardPort: 2525},
		},
		{
			name:    "forward missing host",
			route:   InboundRoute{Name: "fw", MatchType: MatchRecipientDomain, MatchValue: "example.com", Action: InboundActionForward},
			wantErr: true,
		},
		{
			name:    "forward host with port inline rejected",
			route:   InboundRoute{Name: "fw", MatchType: MatchRecipientDomain, MatchValue: "example.com", Action: InboundActionForward, ForwardHost: "mail.internal:25"},
			wantErr: true,
		},
		{
			name:    "forward bad tls",
			route:   InboundRoute{Name: "fw", MatchType: MatchRecipientDomain, MatchValue: "example.com", Action: InboundActionForward, ForwardHost: "mail.internal", ForwardTLS: "maybe"},
			wantErr: true,
		},
		{
			name:  "webhook https ok",
			route: InboundRoute{Name: "wh", MatchType: MatchRecipientEmail, MatchValue: "a@example.com", Action: InboundActionWebhook, DestinationURL: "https://app.example.com/in"},
		},
		{
			name:    "webhook http rejected without allowInsecure",
			route:   InboundRoute{Name: "wh", MatchType: MatchRecipientEmail, MatchValue: "a@example.com", Action: InboundActionWebhook, DestinationURL: "http://app.example.com/in"},
			wantErr: true,
		},
		{
			name:          "webhook http ok with allowInsecure",
			route:         InboundRoute{Name: "wh", MatchType: MatchRecipientEmail, MatchValue: "a@example.com", Action: InboundActionWebhook, DestinationURL: "http://app.example.com/in"},
			allowInsecure: true,
		},
		{
			name:    "webhook inline secret rejected",
			route:   InboundRoute{Name: "wh", MatchType: MatchRecipientEmail, MatchValue: "a@example.com", Action: InboundActionWebhook, DestinationURL: "https://app.example.com/in", SecretRef: "-----BEGIN KEY-----"},
			wantErr: true,
		},
		{
			name:    "unknown action",
			route:   InboundRoute{Name: "x", MatchType: MatchRecipientDomain, MatchValue: "example.com", Action: "drop"},
			wantErr: true,
		},
		{
			name:    "recipient_email needs @",
			route:   InboundRoute{Name: "x", MatchType: MatchRecipientEmail, MatchValue: "example.com", Action: InboundActionMaildir},
			wantErr: true,
		},
		{
			name:    "recipient_domain rejects email",
			route:   InboundRoute{Name: "x", MatchType: MatchRecipientDomain, MatchValue: "a@example.com", Action: InboundActionMaildir},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.route
			err := r.Validate(tt.allowInsecure)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestInboundRouteRouteDomain(t *testing.T) {
	cases := map[string]InboundRoute{
		"example.com": {MatchType: MatchRecipientDomain, MatchValue: "example.com"},
		"sub.org":     {MatchType: MatchRecipientEmail, MatchValue: "user@sub.org"},
	}
	for want, r := range cases {
		if got := r.RouteDomain(); got != want {
			t.Errorf("RouteDomain() = %q, want %q", got, want)
		}
	}
}
