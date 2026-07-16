package worker

import "testing"

func TestIsTLSHandshakeFailure(t *testing.T) {
	cases := []struct {
		name string
		diag string
		want bool
	}{
		{
			name: "escom.bg DHE-only handshake failure",
			diag: "KumoMTA internal: failed to connect to any candidate hosts: All failures are related to OpportunisticInsecure STARTTLS. Consider setting enable_tls=Disabled for this site. mail.escom.bg./195.24.89.4:25: EHLO after OpportunisticInsecure STARTTLS handshake status: failed: received fatal alert: HandshakeFailure",
			want: true,
		},
		{name: "kumomta suggests disable", diag: "consider setting enable_tls=Disabled for this site", want: true},
		{name: "generic starttls handshake failure", diag: "STARTTLS ... received fatal alert: HandshakeFailure", want: true},
		// Must NOT trigger on unrelated deferrals:
		{name: "greylist 4xx", diag: "451 4.7.1 Greylisted, please try again later", want: false},
		{name: "rate limit", diag: "421 4.7.0 Too many connections", want: false},
		{name: "unplumbed source (proxy bind, different bug)", diag: "All failures are related to having an unplumbed source address ... bind 185.117.188.5 for vmta-02 failed: Cannot assign requested address (os error 99)", want: false},
		{name: "empty", diag: "", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTLSHandshakeFailure(c.diag); got != c.want {
				t.Fatalf("isTLSHandshakeFailure(%q) = %v, want %v", c.diag, got, c.want)
			}
		})
	}
}
