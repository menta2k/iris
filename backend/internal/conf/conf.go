// Package conf defines the application configuration structs and a loader that
// reads YAML and applies environment-variable overrides for secrets.
package conf

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root application configuration.
type Config struct {
	Server    Server    `yaml:"server"`
	Data      Data      `yaml:"data"`
	Auth      Auth      `yaml:"auth"`
	KumoMTA   External  `yaml:"kumomta"`
	Rspamd    External  `yaml:"rspamd"`
	Injection Injection `yaml:"injection"`
	Cluster   Cluster   `yaml:"cluster"`
	Agent     Agent     `yaml:"agent"`
	Log       Log       `yaml:"log"`
}

// Injection configures the GreenArrow-compatible mail-injection API. For
// security it runs on its OWN HTTP listener (separate port from the admin API),
// authenticated by a body-level username/password rather than the admin JWT, so
// it can be firewalled independently and never exposes the admin surface.
type Injection struct {
	// Enabled turns the separate injection listener on. Off by default.
	Enabled bool `yaml:"enabled"`
	// Addr is the injection listener's own address, e.g. ":8025". It MUST differ
	// from the admin HTTP/gRPC ports.
	Addr string `yaml:"addr"`
	// Path is the route the injection handler answers on (POST). The caller
	// points its API URL at http(s)://host:<port><path>.
	Path string `yaml:"path"`
	// Timeout bounds request handling.
	Timeout time.Duration `yaml:"timeout"`
	// Username/Password authenticate the injection request body (GreenArrow
	// compatibility). Required when Enabled; supply via env in production.
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// MailClassHeader is a comma-separated list of the header(s) the injected
	// message's `mailclass` field is stamped into, so HTTP-injected mail is
	// classified by the same routing rules as SMTP mail. Empty defaults to
	// "X-Mail-Class"; set to your rules' convention, e.g.
	// "X-GreenArrow-MailClass,X-GreenArrow".
	MailClassHeader string `yaml:"mailclass_header"`
	// TLS serves the injection listener over HTTPS. Provide the certificate
	// EITHER as an iris/ACME-managed cert by domain (TLSCertDomain) OR as an
	// explicit key pair (TLSCertFile + TLSKeyFile). When TLS is true but no
	// usable certificate is available the process refuses to start — the
	// listener never silently falls back to plaintext.
	TLS           bool   `yaml:"tls"`
	TLSCertDomain string `yaml:"tls_cert_domain"`
	TLSCertFile   string `yaml:"tls_cert_file"`
	TLSKeyFile    string `yaml:"tls_key_file"`
}

// Server holds HTTP and gRPC transport configuration.
type Server struct {
	HTTP Endpoint `yaml:"http"`
	GRPC Endpoint `yaml:"grpc"`
}

// Endpoint describes a single network listener.
type Endpoint struct {
	Addr    string        `yaml:"addr"`
	Timeout time.Duration `yaml:"timeout"`
}

// Data holds storage configuration for TimescaleDB and Redis.
type Data struct {
	Database Database `yaml:"database"`
	Redis    Redis    `yaml:"redis"`
}

// Database holds the TimescaleDB/PostgreSQL connection settings.
type Database struct {
	DSN             string        `yaml:"dsn"`
	MaxConns        int32         `yaml:"max_conns"`
	MinConns        int32         `yaml:"min_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
	MigrateOnStart  bool          `yaml:"migrate_on_start"`
}

// Redis holds the Redis connection settings. It supports a single node, a
// Redis Cluster, or a Sentinel-managed failover set:
//   - single: set `addr` (or a one-element `addrs`).
//   - cluster: set `addrs` to the seed nodes (or one seed + `cluster: true`).
//     A single-node client cannot follow MOVED/ASK slot redirections, so a
//     Redis Cluster REQUIRES cluster mode.
//   - sentinel: set `master_name` plus the sentinel `addrs`.
type Redis struct {
	// Addr is a single host:port (back-compat). Addrs, when non-empty, takes
	// precedence and lists the cluster/sentinel seed nodes.
	Addr  string   `yaml:"addr"`
	Addrs []string `yaml:"addrs"`
	// Cluster forces the cluster client even with a single seed address (needed
	// when a Redis Cluster is fronted by one endpoint). Multiple addrs also
	// implies cluster unless MasterName is set.
	Cluster bool `yaml:"cluster"`
	// MasterName, when set, selects Sentinel failover mode (addrs = sentinels).
	MasterName   string        `yaml:"master_name"`
	Password     string        `yaml:"password"`
	DB           int           `yaml:"db"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	ConsumerName string        `yaml:"consumer_name"`
}

// SeedAddrs returns the effective, normalized address list: Addrs when set,
// else Addr. Every entry is additionally split on commas and trimmed, so a
// comma-separated list placed in a single field (a common mistake, and what
// IRIS_REDIS_ADDR / a single `addr` naturally holds) is expanded into separate
// host:port seeds rather than dialed as one bad address.
func (r Redis) SeedAddrs() []string {
	raw := r.Addrs
	if len(raw) == 0 && r.Addr != "" {
		raw = []string{r.Addr}
	}
	var out []string
	for _, entry := range raw {
		for _, a := range strings.Split(entry, ",") {
			if a = strings.TrimSpace(a); a != "" {
				out = append(out, a)
			}
		}
	}
	return out
}

// IsCluster reports whether a Redis Cluster client is required. A multi-seed
// list (however it was supplied — `addrs`, or commas in a single field)
// implies cluster unless Sentinel (MasterName) is selected.
func (r Redis) IsCluster() bool {
	return r.MasterName == "" && (r.Cluster || len(r.SeedAddrs()) > 1)
}

// Auth holds authentication and session configuration.
type Auth struct {
	SessionTTL   time.Duration `yaml:"session_ttl"`
	SessionToken string        `yaml:"session_token_secret"`
	MFARequired  bool          `yaml:"mfa_required"`
	DevBypass    bool          `yaml:"dev_bypass"`
}

// External describes a bounded outbound integration (KumoMTA or Rspamd).
type External struct {
	BaseURL string        `yaml:"base_url"`
	Timeout time.Duration `yaml:"timeout"`
	Stub    bool          `yaml:"stub"`
	// ConfigPath is where the generated KumoMTA policy is written (KumoMTA only).
	ConfigPath string `yaml:"config_path"`
	// ReloadCommand, when set, is executed to reload KumoMTA after a config
	// write (e.g. "kcli reload" or "systemctl reload kumomta"). When empty and
	// ReloadURL is set, an HTTP POST to ReloadURL is used instead.
	ReloadCommand string `yaml:"reload_command"`
	// ReloadURL, when set, is POSTed to in order to reload KumoMTA (its admin
	// HTTP API), e.g. http://localhost:8000/api/admin/bump-config-epoch/v1.
	// A reload (config-epoch bump) re-evaluates runtime callbacks but does NOT
	// re-run kumo.on('init').
	ReloadURL string `yaml:"reload_url"`
	// RestartCommand is executed when a config change touches the init block
	// (listeners, spool, log hook) — which a reload cannot pick up — e.g.
	// "systemctl restart kumomta". RestartURL is an HTTP alternative. When
	// neither is set, Apply falls back to a reload and flags that a manual
	// restart is required.
	RestartCommand string `yaml:"restart_command"`
	RestartURL     string `yaml:"restart_url"`
	// Mode controls inbound rspamd spam filtering in the generated policy
	// (Rspamd only): "" / "off" (disabled), "tag" (scan + X-Spam headers,
	// never reject) or "enforce" (honor rspamd's reject/greylist verdict).
	Mode string `yaml:"mode"`
	// LogStreamRedisURL is the Redis URL embedded in the generated KumoMTA
	// policy's log_hook (KumoMTA only). It is the address KumoMTA reaches Redis
	// at (e.g. "redis://redis:6379" in docker), which may differ from the
	// backend's own Redis address. Empty derives "redis://<redis.addr>".
	LogStreamRedisURL string `yaml:"log_stream_redis_url"`
}

// Cluster configures iris's client side of the KumoMTA cluster control plane:
// the mTLS material used to reach each node's iris-agent. All three paths must
// be set to enable remote-node management; the CA and certificates are created
// with `iris cluster init-ca` / `iris cluster issue-cert`.
type Cluster struct {
	// CACert verifies agent server certificates.
	CACert string `yaml:"ca_cert"`
	// ClientCert/ClientKey authenticate iris to the agents.
	ClientCert string `yaml:"client_cert"`
	ClientKey  string `yaml:"client_key"`
	// CADir holds the cluster CA (ca.crt/ca.key, from `iris cluster init-ca`).
	// Setting it enables online agent enrollment (token -> CSR -> signed cert).
	CADir string `yaml:"ca_dir"`
}

// Enabled reports whether cluster mTLS is fully configured.
func (c Cluster) Enabled() bool {
	return c.CACert != "" && c.ClientCert != "" && c.ClientKey != ""
}

// Agent configures the iris-agent daemon (`iris agent`) that manages the
// co-located KumoMTA on a cluster node. The kumod control settings (config
// path, reload/restart, base URL) come from the regular `kumomta:` section of
// the same config file.
type Agent struct {
	// Listen is the mTLS listener address, e.g. ":8447". It must only be
	// reachable on the private cluster network.
	Listen string `yaml:"listen"`
	// CACert verifies the iris control plane's client certificate; Cert/Key are
	// this agent's server credentials, issued by the same cluster CA.
	CACert string `yaml:"ca_cert"`
	Cert   string `yaml:"cert"`
	Key    string `yaml:"key"`
	// StatePath persists the applied bundle checksum/generation across agent
	// restarts. Empty defaults to "<config_path dir>/iris-agent-state.json".
	StatePath string `yaml:"state_path"`
}

// Log holds structured-logging configuration.
type Log struct {
	Level string `yaml:"level"`
}

// Load reads configuration from the given YAML path and applies environment
// overrides. A missing path returns the built-in defaults with overrides.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read config %s: %w", path, err)
			}
		} else if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	cfg.applyEnv()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Default returns a configuration suitable for local development.
func Default() *Config {
	return &Config{
		Server: Server{
			HTTP: Endpoint{Addr: ":8080", Timeout: 30 * time.Second},
			GRPC: Endpoint{Addr: ":9090", Timeout: 30 * time.Second},
		},
		Data: Data{
			Database: Database{
				DSN:             "postgres://iris:iris@localhost:5432/iris?sslmode=disable",
				MaxConns:        10,
				MinConns:        2,
				ConnMaxLifetime: time.Hour,
				MigrateOnStart:  true,
			},
			Redis: Redis{
				Addr:         "localhost:6379",
				DB:           0,
				DialTimeout:  5 * time.Second,
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 3 * time.Second,
				ConsumerName: "iris-1",
			},
		},
		Auth: Auth{
			SessionTTL:  12 * time.Hour,
			MFARequired: true,
			DevBypass:   false,
		},
		KumoMTA: External{BaseURL: "http://localhost:8000", Timeout: 10 * time.Second, Stub: true, ConfigPath: "/opt/kumomta/etc/policy/iris_generated.lua"},
		Rspamd:  External{BaseURL: "http://localhost:11334", Timeout: 10 * time.Second, Stub: true},
		Injection: Injection{
			Enabled: false,
			Addr:    ":8025",
			Path:    "/api/inject",
			Timeout: 30 * time.Second,
		},
		Log: Log{Level: "info"},
	}
}

func (c *Config) applyEnv() {
	if v := os.Getenv("IRIS_DATABASE_DSN"); v != "" {
		c.Data.Database.DSN = v
	}
	if v := os.Getenv("IRIS_REDIS_ADDR"); v != "" {
		c.Data.Redis.Addr = v
	}
	// Comma-separated seed nodes for a Redis Cluster / Sentinel set.
	if v := os.Getenv("IRIS_REDIS_ADDRS"); v != "" {
		parts := strings.Split(v, ",")
		addrs := parts[:0]
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				addrs = append(addrs, p)
			}
		}
		c.Data.Redis.Addrs = addrs
	}
	if v := os.Getenv("IRIS_REDIS_CLUSTER"); v == "1" || v == "true" {
		c.Data.Redis.Cluster = true
	}
	if v := os.Getenv("IRIS_REDIS_MASTER_NAME"); v != "" {
		c.Data.Redis.MasterName = v
	}
	if v := os.Getenv("IRIS_REDIS_PASSWORD"); v != "" {
		c.Data.Redis.Password = v
	}
	if v := os.Getenv("IRIS_INJECTION_MAILCLASS_HEADER"); v != "" {
		c.Injection.MailClassHeader = v
	}
	if v := os.Getenv("IRIS_HTTP_ADDR"); v != "" {
		c.Server.HTTP.Addr = v
	}
	if v := os.Getenv("IRIS_GRPC_ADDR"); v != "" {
		c.Server.GRPC.Addr = v
	}
	if v := os.Getenv("IRIS_SESSION_SECRET"); v != "" {
		c.Auth.SessionToken = v
	}
	if v := os.Getenv("IRIS_LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("IRIS_AUTH_DEV_BYPASS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Auth.DevBypass = b
		}
	}
	if v := os.Getenv("IRIS_INJECTION_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Injection.Enabled = b
		}
	}
	if v := os.Getenv("IRIS_INJECTION_ADDR"); v != "" {
		c.Injection.Addr = v
	}
	if v := os.Getenv("IRIS_INJECTION_PATH"); v != "" {
		c.Injection.Path = v
	}
	if v := os.Getenv("IRIS_INJECTION_USERNAME"); v != "" {
		c.Injection.Username = v
	}
	if v := os.Getenv("IRIS_INJECTION_PASSWORD"); v != "" {
		c.Injection.Password = v
	}
	if v := os.Getenv("IRIS_INJECTION_TLS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Injection.TLS = b
		}
	}
}

func (c *Config) validate() error {
	if c.Data.Database.DSN == "" {
		return fmt.Errorf("database dsn must be set")
	}
	if c.Server.HTTP.Addr == "" && c.Server.GRPC.Addr == "" {
		return fmt.Errorf("at least one server endpoint must be configured")
	}
	if c.Injection.Enabled {
		if c.Injection.Addr == "" {
			return fmt.Errorf("injection.addr must be set when injection is enabled")
		}
		if c.Injection.Addr == c.Server.HTTP.Addr || c.Injection.Addr == c.Server.GRPC.Addr {
			return fmt.Errorf("injection.addr (%s) must differ from the admin HTTP/gRPC ports", c.Injection.Addr)
		}
		if c.Injection.Username == "" || c.Injection.Password == "" {
			return fmt.Errorf("injection.username and injection.password are required when injection is enabled")
		}
		if c.Injection.Path == "" {
			c.Injection.Path = "/api/inject"
		}
		if c.Injection.TLS {
			hasFiles := c.Injection.TLSCertFile != "" && c.Injection.TLSKeyFile != ""
			oneFile := (c.Injection.TLSCertFile != "") != (c.Injection.TLSKeyFile != "")
			if oneFile {
				return fmt.Errorf("injection: tls_cert_file and tls_key_file must both be set")
			}
			if !hasFiles && c.Injection.TLSCertDomain == "" {
				return fmt.Errorf("injection.tls is enabled but no certificate is configured (set tls_cert_domain or tls_cert_file+tls_key_file)")
			}
		}
	}
	return nil
}
