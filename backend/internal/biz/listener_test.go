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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := valid()
			tt.mutate(&l)
			assertReason(t, l.Validate(), tt.wantErr)
		})
	}
}

func TestListenerTLSPathsAccepted(t *testing.T) {
	l := Listener{Name: "mx-tls", IPAddress: "203.0.113.10", Port: 587, Hostname: "mta.example.com",
		TLSEnabled: true, TLSCertPath: "/etc/tls/cert.pem", TLSKeyPath: "/etc/tls/key.pem"}
	if err := l.Validate(); err != nil {
		t.Fatalf("valid TLS listener rejected: %v", err)
	}
}
