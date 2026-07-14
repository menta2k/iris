package biz

import (
	"strings"
	"testing"
	"time"
)

func validNode() *MTANode {
	return &MTANode{
		Name:      "node1",
		AgentURL:  "https://10.0.0.5:8447",
		ProxyHost: "10.0.0.5",
		ProxyPort: 1080,
	}
}

func TestMTANodeValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(n *MTANode)
		wantErr string // DomainError reason; "" = valid
	}{
		{"valid full", func(n *MTANode) {}, ""},
		{"valid local node without agent or proxy", func(n *MTANode) {
			n.AgentURL, n.ProxyHost, n.ProxyPort = "", "", 0
		}, ""},
		{"defaults status to active", func(n *MTANode) { n.Status = "" }, ""},
		{"name required", func(n *MTANode) { n.Name = "  " }, "MTA_NODE_NAME_REQUIRED"},
		{"name too long", func(n *MTANode) { n.Name = strings.Repeat("a", 129) }, "MTA_NODE_NAME_TOO_LONG"},
		{"name not dns safe", func(n *MTANode) { n.Name = "node one!" }, "MTA_NODE_NAME_INVALID"},
		{"agent url must be https", func(n *MTANode) { n.AgentURL = "http://10.0.0.5:8447" }, "MTA_NODE_AGENT_URL_SCHEME"},
		{"agent url no path", func(n *MTANode) { n.AgentURL = "https://10.0.0.5:8447/agent" }, "MTA_NODE_AGENT_URL_PATH"},
		{"agent url garbage", func(n *MTANode) { n.AgentURL = "https://" }, "MTA_NODE_AGENT_URL_INVALID"},
		{"proxy host without port", func(n *MTANode) { n.ProxyPort = 0 }, "MTA_NODE_PROXY_PARTIAL"},
		{"proxy port without host", func(n *MTANode) { n.ProxyHost = "" }, "MTA_NODE_PROXY_PARTIAL"},
		{"proxy host not an ip", func(n *MTANode) { n.ProxyHost = "node2.internal" }, "MTA_NODE_PROXY_HOST_INVALID"},
		{"proxy host unspecified", func(n *MTANode) { n.ProxyHost = "0.0.0.0" }, "MTA_NODE_PROXY_HOST_INVALID"},
		{"proxy host public rejected", func(n *MTANode) { n.ProxyHost = "203.0.113.9" }, "MTA_NODE_PROXY_HOST_PUBLIC"},
		{"proxy host cgnat allowed", func(n *MTANode) { n.ProxyHost = "100.94.3.7" }, ""},
		{"proxy host ula allowed", func(n *MTANode) { n.ProxyHost = "fd00::7" }, ""},
		{"proxy host loopback allowed", func(n *MTANode) { n.ProxyHost = "127.0.0.1" }, ""},
		{"status invalid", func(n *MTANode) { n.Status = "offline" }, "MTA_NODE_STATUS_INVALID"},
		{"status draining valid", func(n *MTANode) { n.Status = MTANodeStatusDraining }, ""},
		{"notes too long", func(n *MTANode) { n.Notes = strings.Repeat("x", 2001) }, "MTA_NODE_NOTES_TOO_LONG"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := validNode()
			tc.mutate(n)
			err := n.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			de, ok := err.(*DomainError)
			if !ok {
				t.Fatalf("Validate() = %v (%T), want DomainError %s", err, err, tc.wantErr)
			}
			if de.Reason != tc.wantErr {
				t.Fatalf("Validate() reason = %s, want %s", de.Reason, tc.wantErr)
			}
		})
	}
}

func TestMTANodeHelpers(t *testing.T) {
	n := validNode()
	if got := n.ProxyEndpoint(); got != "10.0.0.5:1080" {
		t.Fatalf("ProxyEndpoint() = %q", got)
	}
	n.ProxyHost, n.ProxyPort = "", 0
	if got := n.ProxyEndpoint(); got != "" {
		t.Fatalf("ProxyEndpoint() = %q, want empty", got)
	}
	if n.Local() != false {
		t.Fatalf("Local() with agent_url should be false")
	}
	n.AgentURL = ""
	if !n.Local() {
		t.Fatalf("Local() without agent_url should be true")
	}

	tok := &MTANodeEnrollToken{ExpiresAt: time.Now().Add(-time.Minute)}
	if !tok.Expired(time.Now()) {
		t.Fatalf("Expired() should be true for past expiry")
	}
}
