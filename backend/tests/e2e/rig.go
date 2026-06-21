//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/menta2k/iris/backend/internal/biz"
)

const (
	sinkContainer   = "iris-e2e-sink"
	kumodContainer  = "iris-e2e-kumod"
	rspamdContainer = "iris-e2e-rspamd"
	redisContainer  = "iris-redis" // the dev-compose redis the rig reuses
	sinkCtrlPort    = "18025"      // host port → sink HTTP control/query

	// The rig manages its own network with an explicit subnet: a user-configured
	// subnet is required to assign kumod a fixed --ip (the dev compose network
	// has none). The subnet sits in 172.16.0.0/12, which the generated policy's
	// defaultRelayHosts trusts, so the injector sidecar can relay.
	e2eNetwork = "iris-e2e"
	e2eSubnet  = "172.31.71.0/24"
	kumodIP    = "172.31.71.50"
)

// network is the Docker network the rig attaches to.
func network() string { return e2eNetwork }

// capturedMsg mirrors the sink's JSON: what kumod actually delivered. EHLO is
// the egress source's announced name, which identifies the VMTA that won the
// route.
type capturedMsg struct {
	EHLO     string   `json:"ehlo"`
	MailFrom string   `json:"mailFrom"`
	Rcpts    []string `json:"rcpts"`
	Data     string   `json:"data"`
}

// rig is a running kumod + sink pair wired so kumod loads an iris-generated
// policy and delivers to the sink. The harness shim points kumod's resolver at
// inline .test zones (no DNS server) so delivery is deterministic.
type rig struct {
	t         *testing.T
	staticIP  string // kumod's fixed IP: used for both the listener bind and egress
	sinkIP    string
	redisIP   string
	policyTmp string
}

// startRig builds the sink/injector binaries, launches the sink, renders the
// given snapshot, wraps it in the resolver shim, and boots kumod. It registers
// cleanup. The snapshot's listener and VMTA IPs must equal rig.staticIP, which
// startRig computes up-front and the test bakes into the snapshot — so call
// e2eStaticIP() first.
// startRig boots the rig with the given snapshot. extraFiles are written into
// the mounted policy dir (relative path → content) before kumod starts — used
// to drop DKIM private keys at the paths the generated policy references.
func startRig(t *testing.T, snap biz.ConfigSnapshot, extraFiles ...map[string]string) *rig {
	t.Helper()
	r := &rig{t: t, staticIP: kumodIP}

	ensureNetwork(t)
	sinkBin := buildLinux(t, "github.com/menta2k/iris/backend/tests/e2e/cmd/sink")
	dockerRM(sinkContainer, kumodContainer)
	t.Cleanup(func() { dockerRM(sinkContainer, kumodContainer) })

	// Sink: SMTP on the network, control API published to the host.
	dockerRun(t,
		"-d", "--name", sinkContainer, "--network", network(),
		"-p", sinkCtrlPort+":8025", "-v", sinkBin+":/sink:ro",
		"alpine", "/sink")
	r.sinkIP = containerIP(t, sinkContainer)
	r.redisIP = containerIP(t, "iris-redis")
	waitHTTP(t, "http://127.0.0.1:"+sinkCtrlPort+"/healthz")

	// Render the iris policy and wrap it in the resolver shim.
	rendered, err := biz.RenderKumoConfig(snap)
	if err != nil {
		t.Fatalf("render snapshot: %v", err)
	}
	if !rendered.Valid {
		t.Fatalf("snapshot policy failed lint: %v", rendered.LintIssues)
	}
	files := map[string]string{
		"iris_generated.lua": rendered.Content,
		"init.lua":           r.shim(),
	}
	for _, m := range extraFiles {
		for k, v := range m {
			files[k] = v
		}
	}
	r.policyTmp = writePolicy(t, files)

	// kumod with a fixed IP so the snapshot's listener/egress addresses are
	// bindable and the injector can reach the listener.
	dockerRun(t,
		"-d", "--name", kumodContainer, "--network", network(),
		"--ip", r.staticIP, "-v", r.policyTmp+":/policy:ro",
		kumoImage(), "kumod", "--policy", "/policy/init.lua", "--user", "kumod")
	r.waitReady()
	if os.Getenv("IRIS_E2E_DEBUG") != "" {
		state, _ := exec.Command("docker", "inspect", kumodContainer,
			"-f", "{{.State.Status}} exit={{.State.ExitCode}}").CombinedOutput()
		t.Logf("kumod state: %s", strings.TrimSpace(string(state)))
		t.Logf("kumod logs:\n%s", r.kumodLogs())
	}
	return r
}

// shim is the harness-only init.lua: it configures kumod's resolver with inline
// .test zones (so the recipient domain's MX resolves to the sink and the redis
// host resolves to the redis container) then loads the iris policy under test.
// configure_resolver runs at top level; the generated policy owns kumo.on('init').
func (r *rig) shim() string {
	return fmt.Sprintf(`local kumo = require 'kumo'
kumo.dns.configure_resolver { Test = { zones = {
[[
$ORIGIN sink.test.
@ 30 IN MX 10 mx.sink.test.
mx 30 IN A %s
]],
[[
$ORIGIN test.
iris-redis 30 IN A %s
]],
} } }
dofile('/policy/iris_generated.lua')
`, r.sinkIP, r.redisIP)
}

// startRspamdStub runs the fake rspamd /checkv2 endpoint as a container on the
// rig network and returns its base URL (for ConfigSnapshot.RspamdURL). Call it
// before building the snapshot so the URL can be baked into the policy. kumod
// reaches it by container IP, so no DNS is involved.
func startRspamdStub(t *testing.T) string {
	t.Helper()
	ensureNetwork(t)
	bin := buildLinux(t, "github.com/menta2k/iris/backend/tests/e2e/cmd/rspamdstub")
	dockerRM(rspamdContainer)
	t.Cleanup(func() { dockerRM(rspamdContainer) })
	dockerRun(t,
		"-d", "--name", rspamdContainer, "--network", network(),
		"-v", bin+":/rspamd:ro", "alpine", "/rspamd")
	return "http://" + containerIP(t, rspamdContainer) + ":11334"
}

// inject submits one message through kumod's reception path via a throwaway
// injector sidecar on the network. headers are raw "Name: Value" lines.
func (r *rig) inject(to string, headers ...string) {
	r.injectAs("", to, headers...)
}

// injectAs is inject with an explicit envelope sender / From header (so the
// message can be attributed to a DKIM signing domain). An empty from uses the
// injector default.
func (r *rig) injectAs(from, to string, headers ...string) {
	r.injectFull(from, to, "", headers...)
}

// injectFull submits a message with an explicit sender, body, and headers — used
// to feed a complete ARF report through kumod's reception path. An empty body
// uses the injector default.
func (r *rig) injectFull(from, to, body string, headers ...string) {
	r.t.Helper()
	if out, err := r.tryInject(from, to, body, headers...); err != nil {
		r.t.Fatalf("inject to %s: %v\n%s", to, err, out)
	}
}

// tryInject runs the injector and returns its output and error instead of
// failing the test — used to assert a message is *rejected* (e.g. by rspamd in
// enforce mode, where the injector sees a 5xx).
func (r *rig) tryInject(from, to, body string, headers ...string) (string, error) {
	r.t.Helper()
	injectBin := buildLinux(r.t, "github.com/menta2k/iris/backend/tests/e2e/cmd/inject")
	args := []string{
		"run", "--rm", "--network", network(), "-v", injectBin + ":/inject:ro",
		"alpine", "/inject", "-addr", r.staticIP + ":2525", "-to", to,
	}
	if from != "" {
		args = append(args, "-from", from)
	}
	if body != "" {
		args = append(args, "-body", body)
	}
	for _, h := range headers {
		args = append(args, "-header", h)
	}
	out, err := exec.Command("docker", args...).CombinedOutput()
	return string(out), err
}

// programBounce tells the sink to answer recipients containing match with the
// given SMTP code at the given stage ("rcpt" or "data"), so a delivery attempt
// produces a bounce.
func (r *rig) programBounce(match, stage string, code int, text string) {
	r.t.Helper()
	body := fmt.Sprintf(`{"match":%q,"stage":%q,"code":%d,"text":%q}`, match, stage, code, text)
	resp, err := http.Post("http://127.0.0.1:"+sinkCtrlPort+"/behavior", "application/json", strings.NewReader(body))
	if err != nil {
		r.t.Fatalf("program sink behavior: %v", err)
	}
	resp.Body.Close()
}

// sinkMessages returns every message the sink has captured so far.
func (r *rig) sinkMessages() []capturedMsg {
	r.t.Helper()
	resp, err := http.Get("http://127.0.0.1:" + sinkCtrlPort + "/messages")
	if err != nil {
		r.t.Fatalf("query sink: %v", err)
	}
	defer resp.Body.Close()
	var msgs []capturedMsg
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		r.t.Fatalf("decode sink messages: %v", err)
	}
	return msgs
}

// waitForSink polls until at least n messages are captured or it times out,
// returning the captured set.
func (r *rig) waitForSink(n int, timeout time.Duration) []capturedMsg {
	r.t.Helper()
	deadline := time.Now().Add(timeout)
	var last []capturedMsg
	for time.Now().Before(deadline) {
		last = r.sinkMessages()
		if len(last) >= n {
			return last
		}
		time.Sleep(300 * time.Millisecond)
	}
	r.t.Fatalf("timed out waiting for %d sink messages; got %d. kumod logs:\n%s", n, len(last), r.kumodLogs())
	return last
}

func (r *rig) waitReady() {
	r.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(r.kumodLogs(), "initialization complete") {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	r.t.Fatalf("kumod did not become ready. logs:\n%s", r.kumodLogs())
}

func (r *rig) kumodLogs() string {
	out, _ := exec.Command("docker", "logs", kumodContainer).CombinedOutput()
	return string(out)
}

// --- docker / build helpers ---

func buildLinux(t *testing.T, pkg string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), filepath.Base(pkg))
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, b)
	}
	// The container user must be able to read+exec the mounted binary.
	_ = os.Chmod(out, 0o755)
	_ = os.Chmod(filepath.Dir(out), 0o755)
	return out
}

func dockerRun(t *testing.T, args ...string) {
	t.Helper()
	full := append([]string{"run"}, args...)
	if out, err := exec.Command("docker", full...).CombinedOutput(); err != nil {
		t.Fatalf("docker run %v: %v\n%s", args, err, out)
	}
}

func dockerRM(names ...string) {
	for _, n := range names {
		_ = exec.Command("docker", "rm", "-f", n).Run()
	}
}

func containerIP(t *testing.T, name string) string {
	t.Helper()
	format := fmt.Sprintf("{{(index .NetworkSettings.Networks %q).IPAddress}}", network())
	out, err := exec.Command("docker", "inspect", name, "-f", format).CombinedOutput()
	ip := strings.TrimSpace(string(out))
	if err != nil || net.ParseIP(ip) == nil {
		t.Fatalf("inspect %s ip: %v (%q)", name, err, ip)
	}
	return ip
}

// ensureNetwork creates the rig's dedicated network (with an explicit subnet so
// kumod can take a fixed IP) if it does not already exist, and connects the
// dev-compose redis to it so kumod's log hook can reach Redis on the same
// network. Both operations are idempotent across test runs.
func ensureNetwork(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "network", "inspect", e2eNetwork).Run(); err != nil {
		if out, err := exec.Command("docker", "network", "create",
			"--subnet", e2eSubnet, e2eNetwork).CombinedOutput(); err != nil {
			t.Fatalf("create network %s: %v\n%s", e2eNetwork, err, out)
		}
	}
	// Attach redis (ignore the error when it is already connected).
	_ = exec.Command("docker", "network", "connect", e2eNetwork, redisContainer).Run()
}

func writePolicy(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "iris-e2e-policy-*")
	if err != nil {
		t.Fatalf("temp policy dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod policy dir: %v", err)
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if d := filepath.Dir(path); d != dir {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", d, err)
			}
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func waitHTTP(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}
