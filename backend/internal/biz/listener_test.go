package biz

import "testing"

func TestListenerValidate(t *testing.T) {
	valid := func() Listener {
		return Listener{Name: "mx-1", IPAddress: "203.0.113.10", Port: 25, Hostname: "mta1.example.com"}
	}
	l := valid()
	if err := l.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.ListenAddr() != "203.0.113.10:25" {
		t.Fatalf("unexpected listen addr: %q", l.ListenAddr())
	}

	tests := []struct {
		name    string
		mutate  func(*Listener)
		wantErr string
	}{
		{"missing name", func(l *Listener) { l.Name = "" }, "LISTENER_NAME_REQUIRED"},
		{"bad ip", func(l *Listener) { l.IPAddress = "nope" }, "LISTENER_IP_INVALID"},
		{"wildcard ip", func(l *Listener) { l.IPAddress = "0.0.0.0" }, "LISTENER_IP_WILDCARD"},
		{"bad port", func(l *Listener) { l.Port = 70000 }, "LISTENER_PORT_RANGE"},
		{"missing hostname", func(l *Listener) { l.Hostname = "" }, "LISTENER_HOSTNAME_REQUIRED"},
		{"bad hostname", func(l *Listener) { l.Hostname = "not a host" }, "LISTENER_HOSTNAME_INVALID"},
		{"tls without paths", func(l *Listener) { l.TLSEnabled = true }, "LISTENER_TLS_PATHS_REQUIRED"},
		{"bad relay host", func(l *Listener) { l.RelayHosts = []string{"not-a-cidr!"} }, "LISTENER_RELAY_HOST_INVALID"},
		{"bad role", func(l *Listener) { l.Role = "weird" }, "LISTENER_ROLE_INVALID"},
		{"inbound with relay", func(l *Listener) {
			l.Role = ListenerRoleInbound
			l.RelayHosts = []string{"10.0.0.0/8"}
		}, "LISTENER_INBOUND_RELAY_FORBIDDEN"},
		{"submission without relay", func(l *Listener) { l.Role = ListenerRoleSubmission }, "LISTENER_SUBMISSION_RELAY_REQUIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := valid()
			tt.mutate(&l)
			assertReason(t, l.Validate(), tt.wantErr)
		})
	}
}

func TestListenerRoleDefaultsAndSubmission(t *testing.T) {
	// Role defaults to inbound when unset.
	l := Listener{Name: "mx", IPAddress: "203.0.113.10", Port: 25, Hostname: "mta.example.com"}
	if err := l.Validate(); err != nil {
		t.Fatalf("default-role listener rejected: %v", err)
	}
	if l.Role != ListenerRoleInbound {
		t.Fatalf("role = %q, want inbound default", l.Role)
	}
	// A submission listener with a relay allowlist is valid.
	s := Listener{Name: "submit", IPAddress: "203.0.113.11", Port: 587, Hostname: "submit.example.com",
		Role: ListenerRoleSubmission, RelayHosts: []string{"10.1.111.0/24"}}
	if err := s.Validate(); err != nil {
		t.Fatalf("submission listener with relay rejected: %v", err)
	}
}

func TestListenerTLSPathsAccepted(t *testing.T) {
	l := Listener{Name: "mx-tls", IPAddress: "203.0.113.10", Port: 587, Hostname: "mta.example.com",
		TLSEnabled: true, TLSCertPath: "/etc/tls/cert.pem", TLSKeyPath: "/etc/tls/key.pem"}
	if err := l.Validate(); err != nil {
		t.Fatalf("valid TLS listener rejected: %v", err)
	}
}
