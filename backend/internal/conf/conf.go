// Package conf defines the application configuration structs and a loader that
// reads YAML and applies environment-variable overrides for secrets.
package conf

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root application configuration.
type Config struct {
	Server  Server   `yaml:"server"`
	Data    Data     `yaml:"data"`
	Auth    Auth     `yaml:"auth"`
	KumoMTA External `yaml:"kumomta"`
	Rspamd  External `yaml:"rspamd"`
	Log     Log      `yaml:"log"`
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

// Redis holds the Redis Streams connection settings.
type Redis struct {
	Addr         string        `yaml:"addr"`
	Password     string        `yaml:"password"`
	DB           int           `yaml:"db"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	ConsumerName string        `yaml:"consumer_name"`
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
		Log:     Log{Level: "info"},
	}
}

func (c *Config) applyEnv() {
	if v := os.Getenv("IRIS_DATABASE_DSN"); v != "" {
		c.Data.Database.DSN = v
	}
	if v := os.Getenv("IRIS_REDIS_ADDR"); v != "" {
		c.Data.Redis.Addr = v
	}
	if v := os.Getenv("IRIS_REDIS_PASSWORD"); v != "" {
		c.Data.Redis.Password = v
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
}

func (c *Config) validate() error {
	if c.Data.Database.DSN == "" {
		return fmt.Errorf("database dsn must be set")
	}
	if c.Server.HTTP.Addr == "" && c.Server.GRPC.Addr == "" {
		return fmt.Errorf("at least one server endpoint must be configured")
	}
	return nil
}
