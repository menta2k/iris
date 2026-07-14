package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/menta2k/iris/backend/internal/agent"
	"github.com/menta2k/iris/backend/internal/clusterca"
	"github.com/menta2k/iris/backend/internal/conf"
	"github.com/menta2k/iris/backend/internal/data"
)

// agentVersion is stamped into health reports; kept simple until a build-time
// version is threaded through ldflags.
const agentVersion = "iris-agent/1"

// buildClusterAgentClient builds the mTLS HTTP client iris uses to reach node
// agents, or nil when the cluster section is not configured (single-node mode,
// where remote nodes are refused with a clear error).
func buildClusterAgentClient(cfg conf.Cluster, timeout time.Duration) (*http.Client, error) {
	if !cfg.Enabled() {
		if cfg.CACert != "" || cfg.ClientCert != "" || cfg.ClientKey != "" {
			return nil, fmt.Errorf("cluster.ca_cert, client_cert, and client_key must all be set together")
		}
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
	if err != nil {
		return nil, fmt.Errorf("load cluster client certificate: %w", err)
	}
	caRaw, err := os.ReadFile(cfg.CACert)
	if err != nil {
		return nil, fmt.Errorf("read cluster CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caRaw) {
		return nil, fmt.Errorf("cluster CA %s contains no certificates", cfg.CACert)
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
			RootCAs:      pool,
		}},
	}, nil
}

// runAgent runs the iris-agent daemon: the mTLS control-plane endpoint on a
// KumoMTA cluster node. It reuses the kumomta: config section for kumod
// control (config path, reload/restart, base URL) and the agent: section for
// its listener + TLS material.
func runAgent(ctx context.Context, cfg *conf.Config, log *slog.Logger) int {
	if cfg.KumoMTA.Stub {
		log.Error("agent requires kumomta.stub=false (it manages a real kumod)")
		return 1
	}
	agentCfg := cfg.Agent
	if agentCfg.StatePath == "" && cfg.KumoMTA.ConfigPath != "" {
		agentCfg.StatePath = filepath.Join(filepath.Dir(cfg.KumoMTA.ConfigPath), "iris-agent-state.json")
	}

	kumo := data.NewFileKumoMTA(cfg.KumoMTA)
	kumo.DisableNodePrelude() // the agent writes the prelude from each bundle's NodeName
	configDir := ""
	if cfg.KumoMTA.ConfigPath != "" {
		configDir = filepath.Dir(cfg.KumoMTA.ConfigPath)
	}
	srv, err := agent.New(agentCfg, kumo, cfg.KumoMTA.BaseURL, configDir, agentVersion, log)
	if err != nil {
		log.Error("agent startup failed", "error", err.Error())
		return 1
	}
	log.Info("starting iris-agent", "listen", agentCfg.Listen, "config_path", cfg.KumoMTA.ConfigPath)
	if err := agent.Run(ctx, agentCfg, srv); err != nil && !strings.Contains(err.Error(), "Server closed") {
		log.Error("agent terminated", "error", err.Error())
		return 1
	}
	return 0
}

// runClusterCommand dispatches `iris cluster <subcommand>`.
func runClusterCommand(cfg *conf.Config, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: iris cluster {init-ca|issue-cert} [flags]")
		return 2
	}
	switch args[0] {
	case "init-ca":
		fs := flag.NewFlagSet("cluster init-ca", flag.ExitOnError)
		dir := fs.String("dir", "cluster-ca", "directory to create the CA in")
		cn := fs.String("cn", "iris-cluster-ca", "CA common name")
		_ = fs.Parse(args[1:])
		if err := clusterca.InitCA(*dir, *cn); err != nil {
			fmt.Fprintln(os.Stderr, "init-ca:", err)
			return 1
		}
		fmt.Printf("cluster CA created in %s (ca.crt world-readable, ca.key 0600)\n", *dir)
		fmt.Println("Distribute ca.crt to every node; keep ca.key only on the iris host.")
		return 0
	case "issue-cert":
		fs := flag.NewFlagSet("cluster issue-cert", flag.ExitOnError)
		caDir := fs.String("ca-dir", "cluster-ca", "directory holding ca.crt/ca.key")
		outDir := fs.String("out", ".", "directory to write <name>.crt/<name>.key into")
		name := fs.String("name", "", "certificate name (node name, or e.g. iris-control-plane)")
		sans := fs.String("sans", "", "comma-separated DNS names and/or IPs the cert must cover (the agent URL host)")
		server := fs.Bool("server", false, "issue a server certificate (for an agent); omit for the iris client cert")
		_ = fs.Parse(args[1:])
		if *name == "" {
			fmt.Fprintln(os.Stderr, "issue-cert: -name is required")
			return 2
		}
		opts := clusterca.IssueOptions{CommonName: *name, Server: *server}
		for _, san := range strings.Split(*sans, ",") {
			san = strings.TrimSpace(san)
			if san == "" {
				continue
			}
			if ip := net.ParseIP(san); ip != nil {
				opts.IPs = append(opts.IPs, ip)
			} else {
				opts.DNSNames = append(opts.DNSNames, san)
			}
		}
		if *server && len(opts.IPs) == 0 && len(opts.DNSNames) == 0 {
			fmt.Fprintln(os.Stderr, "issue-cert: a server certificate needs -sans covering the agent URL host")
			return 2
		}
		fp, err := clusterca.IssueCert(*caDir, *outDir, *name, opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "issue-cert:", err)
			return 1
		}
		fmt.Printf("issued %s/%s.crt (+ .key, 0600)\nsha256 fingerprint: %s\n", *outDir, *name, fp)
		if *server {
			fmt.Println("Copy the .crt/.key and ca.crt to the node, reference them in its agent: config,")
			fmt.Println("and record the fingerprint on the node entry in iris (cert pinning).")
		}
		return 0
	case "enroll":
		fs := flag.NewFlagSet("cluster enroll", flag.ExitOnError)
		irisURL := fs.String("iris-url", "", "iris admin base URL, e.g. https://iris.internal:8080")
		name := fs.String("name", "", "this node's name as registered in iris")
		token := fs.String("token", "", "single-use bootstrap token issued in iris")
		sans := fs.String("sans", "", "comma-separated DNS names/IPs the cert must cover (the agent URL host)")
		out := fs.String("out", "/etc/iris/cluster", "directory to write agent.crt/agent.key/ca.crt into")
		serverCA := fs.String("iris-ca", "", "CA bundle to verify the iris HTTPS endpoint (recommended)")
		insecure := fs.Bool("insecure", false, "skip iris TLS verification (bootstrap relies on the token alone)")
		_ = fs.Parse(args[1:])
		opts := clusterca.EnrollOptions{
			IrisURL: *irisURL, NodeName: *name, Token: *token,
			OutDir: *out, ServerCA: *serverCA, Insecure: *insecure,
		}
		for _, san := range strings.Split(*sans, ",") {
			san = strings.TrimSpace(san)
			if san == "" {
				continue
			}
			if ip := net.ParseIP(san); ip != nil {
				opts.IPs = append(opts.IPs, ip)
			} else {
				opts.DNSNames = append(opts.DNSNames, san)
			}
		}
		certPath, keyPath, caPath, err := clusterca.Enroll(opts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "enroll:", err)
			return 1
		}
		fmt.Printf("enrolled: %s, %s (0600), %s\n", certPath, keyPath, caPath)
		fmt.Println("Reference these in the node's agent: config section, then start `iris agent`.")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown cluster subcommand %q (want init-ca, issue-cert, or enroll)\n", args[0])
		return 2
	}
}
