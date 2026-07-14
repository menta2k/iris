// Package agent implements the iris-agent daemon: the mTLS control-plane
// endpoint running next to each KumoMTA node. It receives checksummed config
// bundles from iris, applies them through the same local file/reload mechanics
// the single-node adapter uses, reports health, and reverse-proxies admin API
// calls to the localhost-bound kumod HTTP listener.
package agent

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/menta2k/iris/backend/internal/agentapi"
	"github.com/menta2k/iris/backend/internal/biz"
	"github.com/menta2k/iris/backend/internal/conf"
	"github.com/menta2k/iris/backend/internal/netutil"
)

// maxBundleBytes bounds a staged config bundle (policy + shaping TOMLs).
const maxBundleBytes = 64 << 20

// Applier installs a rendered KumoMTA config on this node. Satisfied by
// data.FileKumoMTA (without cluster attachment), so the agent applies configs
// with exactly the same file/reload semantics as a single-node iris.
type Applier interface {
	ApplyConfig(ctx context.Context, rendered biz.RenderedConfig, restart bool) (string, string, error)
	ApplyServiceControl(ctx context.Context, op biz.ServiceOperation) (string, error)
	Status(ctx context.Context) (biz.KumoStatus, error)
}

// Server is the agent's HTTP handler plus its persisted apply state.
type Server struct {
	cfg     conf.Agent
	kumo    Applier
	log     *slog.Logger
	version string
	// kumodURL is the localhost kumod HTTP listener the /v1/kumod/* proxy
	// forwards to (from the kumomta: config section).
	kumodURL *url.URL
	// configDir is where the per-node identity prelude is written (the policy
	// directory); empty disables prelude writing.
	configDir string

	mu     sync.Mutex
	staged *agentapi.ConfigBundle
	state  persistedState
}

// persistedState survives agent restarts so replay protection and drift
// reporting keep working.
type persistedState struct {
	AppliedChecksum string `json:"applied_checksum"`
	Generation      int64  `json:"generation"`
}

// New constructs the agent server. version is the agent build version reported
// in health; kumodBaseURL may be empty to disable the admin proxy; configDir is
// the policy directory the node identity prelude is written into (empty
// disables it).
func New(cfg conf.Agent, kumo Applier, kumodBaseURL, configDir, version string, log *slog.Logger) (*Server, error) {
	s := &Server{cfg: cfg, kumo: kumo, log: log, version: version, configDir: configDir}
	if kumodBaseURL != "" {
		u, err := url.Parse(kumodBaseURL)
		if err != nil {
			return nil, fmt.Errorf("parse kumod base url: %w", err)
		}
		s.kumodURL = u
	}
	if err := s.loadState(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) statePath() string {
	if s.cfg.StatePath != "" {
		return s.cfg.StatePath
	}
	return "iris-agent-state.json"
}

func (s *Server) loadState() error {
	raw, err := os.ReadFile(s.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read agent state: %w", err)
	}
	if err := json.Unmarshal(raw, &s.state); err != nil {
		return fmt.Errorf("parse agent state %s: %w", s.statePath(), err)
	}
	return nil
}

func (s *Server) saveState() {
	raw, err := json.Marshal(s.state)
	if err != nil {
		s.log.Error("marshal agent state", "error", err.Error())
		return
	}
	if err := os.WriteFile(s.statePath(), raw, 0o600); err != nil {
		s.log.Error("write agent state", "path", s.statePath(), "error", err.Error())
	}
}

// Handler returns the agent's HTTP mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+agentapi.PathStage, s.handleStage)
	mux.HandleFunc("POST "+agentapi.PathActivate, s.handleActivate)
	mux.HandleFunc("GET "+agentapi.PathHealth, s.handleHealth)
	mux.HandleFunc("GET "+agentapi.PathIPs, s.handleIPs)
	if s.kumodURL != nil {
		mux.Handle(agentapi.PathKumodPrefix, s.kumodProxy())
	}
	return mux
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(agentapi.Error{Code: code, Message: msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// handleStage verifies and stages a config bundle in memory.
func (s *Server) handleStage(w http.ResponseWriter, r *http.Request) {
	var b agentapi.ConfigBundle
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBundleBytes)).Decode(&b); err != nil {
		writeErr(w, http.StatusBadRequest, "BUNDLE_DECODE", "invalid bundle: "+err.Error())
		return
	}
	if b.Checksum == "" || b.Policy.Content == "" {
		writeErr(w, http.StatusBadRequest, "BUNDLE_INCOMPLETE", "bundle checksum and policy are required")
		return
	}
	all := append([]agentapi.File{b.Policy}, b.Shaping...)
	all = append(all, b.TLSFiles...)
	for _, f := range all {
		if sum := sha256Hex(f.Content); sum != f.SHA256 {
			writeErr(w, http.StatusBadRequest, "BUNDLE_CHECKSUM_MISMATCH",
				fmt.Sprintf("file %s checksum mismatch (got %s, declared %s)", f.Name, sum, f.SHA256))
			return
		}
	}
	// TLS files land at operator-controlled absolute paths; reject anything that
	// is not an absolute, metacharacter-free path so a bundle can only write a
	// cert file where a listener would reference one.
	for _, f := range b.TLSFiles {
		if !biz.ValidTLSFilePath(f.Name) {
			writeErr(w, http.StatusBadRequest, "BUNDLE_TLS_PATH_INVALID",
				fmt.Sprintf("TLS file path %q is not an absolute, metacharacter-free path", f.Name))
			return
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Replay protection: a bundle must be newer than the last activated one.
	if b.Generation <= s.state.Generation {
		writeErr(w, http.StatusConflict, "BUNDLE_STALE",
			fmt.Sprintf("bundle generation %d is not newer than activated generation %d", b.Generation, s.state.Generation))
		return
	}
	s.staged = &b
	s.log.Info("staged config bundle", "checksum", b.Checksum, "generation", b.Generation)
	writeJSON(w, agentapi.StageReply{Staged: true, Checksum: b.Checksum})
}

// handleActivate applies the staged bundle (or, with an empty checksum, just
// triggers a reload of the current config).
func (s *Server) handleActivate(w http.ResponseWriter, r *http.Request) {
	var req agentapi.ActivateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "ACTIVATE_DECODE", "invalid request: "+err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if req.Checksum == "" {
		// Bare reload of whatever is currently installed.
		if _, err := s.kumo.ApplyServiceControl(r.Context(), biz.ServiceReload); err != nil {
			writeErr(w, http.StatusBadGateway, "RELOAD_FAILED", err.Error())
			return
		}
		writeJSON(w, agentapi.ActivateReply{Action: "reloaded", AppliedChecksum: s.state.AppliedChecksum})
		return
	}

	if s.staged == nil || s.staged.Checksum != req.Checksum {
		writeErr(w, http.StatusConflict, "ACTIVATE_NOT_STAGED",
			fmt.Sprintf("checksum %s is not staged", req.Checksum))
		return
	}

	rendered := biz.RenderedConfig{
		Content:      s.staged.Policy.Content,
		Checksum:     s.staged.Checksum,
		InitChecksum: s.staged.InitChecksum,
	}
	// Listener TLS cert/key files: the local applier writes each to its absolute
	// path (0640) so this node's kumod serves the centrally-issued cert.
	for _, f := range s.staged.TLSFiles {
		rendered.TLSFiles = append(rendered.TLSFiles, biz.TLSFile{Path: f.Name, Content: f.Content})
	}
	for _, f := range s.staged.Shaping {
		switch f.Name {
		case "iris-base.toml":
			rendered.ShapingBase = f.Content
		case "iris-warmup.toml":
			rendered.ShapingWarmup = f.Content
		case "iris-automation.toml":
			rendered.ShapingAutomation = f.Content
		default:
			writeErr(w, http.StatusBadRequest, "BUNDLE_UNKNOWN_FILE", "unexpected bundle file "+f.Name)
			return
		}
	}

	// Node identity prelude: written from the bundle's NodeName so this node's
	// log records carry its registry name. Outside the policy checksum by
	// design (the policy is identical on every node).
	if s.staged.NodeName != "" && s.configDir != "" {
		prelude := filepath.Join(s.configDir, biz.NodePreludeFile)
		if err := os.WriteFile(prelude, []byte(biz.NodePreludeContent(s.staged.NodeName)), 0o644); err != nil {
			writeErr(w, http.StatusInternalServerError, "PRELUDE_WRITE_FAILED", err.Error())
			return
		}
	}

	_, summary, err := s.kumo.ApplyConfig(r.Context(), rendered, req.Restart)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "ACTIVATE_FAILED", err.Error())
		return
	}
	s.state.AppliedChecksum = s.staged.Checksum
	s.state.Generation = s.staged.Generation
	s.staged = nil
	s.saveState()

	action := "reloaded"
	if req.Restart {
		action = "restarted"
	}
	if strings.Contains(summary, "MANUAL RESTART REQUIRED") {
		action = "reloaded — MANUAL RESTART REQUIRED for init changes (listeners/spool/log hook)"
	}
	s.log.Info("activated config bundle", "checksum", s.state.AppliedChecksum, "action", action)
	writeJSON(w, agentapi.ActivateReply{Action: action, AppliedChecksum: s.state.AppliedChecksum})
}

// handleIPs returns the node's assignable IP addresses for the UI's IP pickers.
func (s *Server) handleIPs(w http.ResponseWriter, r *http.Request) {
	ips, err := netutil.LocalIPs()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "IPS_FAILED", err.Error())
		return
	}
	writeJSON(w, agentapi.NodeIPs{IPs: ips})
}

// handleHealth reports agent and kumod state.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	st, _ := s.kumo.Status(r.Context())
	s.mu.Lock()
	h := agentapi.Health{
		Version:         s.version,
		AppliedChecksum: s.state.AppliedChecksum,
		Generation:      s.state.Generation,
		Kumo:            st.State,
	}
	s.mu.Unlock()
	writeJSON(w, h)
}

// kumodProxy reverse-proxies /v1/kumod/<path> to the localhost kumod listener,
// letting iris reach the admin API without kumod being network-exposed.
func (s *Server) kumodProxy() http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(s.kumodURL)
			pr.Out.URL.Path = "/" + strings.TrimPrefix(pr.In.URL.Path, agentapi.PathKumodPrefix)
			pr.Out.Host = s.kumodURL.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeErr(w, http.StatusBadGateway, "KUMOD_UNREACHABLE", err.Error())
		},
	}
	return proxy
}

// TLSConfig builds the agent's mTLS server configuration: it serves the
// cluster-CA-issued certificate and requires a client certificate signed by
// the same CA (the iris control plane).
func TLSConfig(cfg conf.Agent) (*tls.Config, error) {
	if cfg.CACert == "" || cfg.Cert == "" || cfg.Key == "" {
		return nil, fmt.Errorf("agent ca_cert, cert, and key are all required (the agent is mTLS-only)")
	}
	cert, err := tls.LoadX509KeyPair(cfg.Cert, cfg.Key)
	if err != nil {
		return nil, fmt.Errorf("load agent certificate: %w", err)
	}
	caRaw, err := os.ReadFile(cfg.CACert)
	if err != nil {
		return nil, fmt.Errorf("read cluster CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caRaw) {
		return nil, fmt.Errorf("cluster CA %s contains no certificates", cfg.CACert)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}, nil
}

// Run serves the agent until ctx is cancelled.
func Run(ctx context.Context, cfg conf.Agent, srv *Server) error {
	tlsCfg, err := TLSConfig(cfg)
	if err != nil {
		return err
	}
	listen := cfg.Listen
	if listen == "" {
		listen = ":8447"
	}
	httpSrv := &http.Server{
		Addr:              listen,
		Handler:           srv.Handler(),
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		// Certificate/key paths are in TLSConfig; ListenAndServeTLS args stay empty.
		errCh <- httpSrv.ListenAndServeTLS("", "")
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
