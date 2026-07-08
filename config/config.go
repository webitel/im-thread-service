package config

import (
	"log/slog"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"

	"github.com/webitel/webitel-go-kit/appconfig"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type Config struct {
	Service        ServiceConfig        `mapstructure:"service"`
	Log            appconfig.Log        `mapstructure:"log"`
	Postgres       appconfig.Postgres   `mapstructure:"postgres"`
	Redis          appconfig.Redis      `mapstructure:"redis"`
	Consul         appconfig.Consul     `mapstructure:"consul"`
	Pubsub         appconfig.Pubsub     `mapstructure:"pubsub"`
	Profiler       appconfig.Profiler   `mapstructure:"profiler"`
	LeaderElection LeaderElectionConfig `mapstructure:"leader_election"`
}

type ServiceConfig struct {
	Addr       string             `mapstructure:"addr"`
	Connection appconfig.GRPCConn `mapstructure:"conn"`
}

// LeaderElectionConfig tunes the Consul session-lock used to elect a single
// active outbox-forwarder node. Mirrors the loop_wait/ttl/retry_timeout model
// used by Patroni for DCS-backed leader locks.
type LeaderElectionConfig struct {
	// TTL is the Consul session TTL: how long a session is considered valid
	// after its last renewal before the lock is automatically released.
	// Consul enforces a 10s floor by default (session_ttl_min).
	TTL time.Duration `mapstructure:"ttl"`
	// RenewInterval is how often the leader renews its session heartbeat.
	// Must leave room for at least one missed renewal within the TTL window.
	RenewInterval time.Duration `mapstructure:"renew_interval"`
	// ErrCooldown is how long to back off after a transient Consul error
	// (session create/acquire failure) before retrying.
	ErrCooldown time.Duration `mapstructure:"err_cooldown"`
	// BlockingWait caps how long a single Consul blocking (long-poll) query
	// is held open before being reissued.
	BlockingWait time.Duration `mapstructure:"blocking_wait"`
	// LockDelay is Consul's anti-flapping grace period enforced after a lock is
	// released (by any means — explicit release or session invalidation) before
	// it can be re-acquired by anyone. Per Consul's session API, this must be
	// greater than zero (Consul rejects/ignores zero and falls back to its own
	// 15s default). Kept low here because our forwarder is idempotent
	// (offset-tracked in Postgres), so we don't need Consul's full default grace period.
	LockDelay time.Duration `mapstructure:"lock_delay"`
}

// LoadServerConfig loads the full configuration required by the gRPC server.
func LoadServerConfig() (*Config, error) {
	loader := appconfig.NewLoader(appconfig.Sections{
		Log:      true,
		Postgres: true,
		Redis:    true,
		Consul:   true,
		Pubsub:   true,
		Profiler: true,
	})
	loader.RegisterFlags(pflag.CommandLine)
	registerServiceFlags()
	pflag.Parse()

	cfg := &Config{}
	if err := loader.Load(pflag.CommandLine, cfg); err != nil {
		return nil, err
	}

	loader.Watch(func(e fsnotify.Event) {
		slog.Info("config file changed", "name", e.Name)

		newCfg := &Config{}
		if err := loader.Viper().Unmarshal(newCfg); err != nil {
			slog.Error("config reload: unmarshal failed", "error", err)

			return
		}

		if err := newCfg.validate(); err != nil {
			slog.Error("config reload: validation failed", "error", err)

			return
		}

		*cfg = *newCfg

		slog.Info("config reloaded")
	})

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadMigrateConfig loads the minimal configuration required by the migrate command.
func LoadMigrateConfig() (*Config, error) {
	loader := appconfig.NewLoader(appconfig.Sections{
		Log:      true,
		Postgres: true,
	})
	loader.RegisterFlags(pflag.CommandLine)
	pflag.Parse()

	cfg := &Config{}
	if err := loader.Load(pflag.CommandLine, cfg); err != nil {
		return nil, err
	}

	if cfg.Postgres.DSN == "" {
		return nil, errors.New("config: postgres.dsn is required", errors.WithID("config.config.load_migrate_config.dsn"))
	}

	return cfg, nil
}

func registerServiceFlags() {
	pflag.String("service.addr", "localhost:8080", "gRPC listen address")
	appconfig.RegisterGRPCConnFlags(pflag.CommandLine, "service.conn", true)

	pflag.Duration("leader_election.ttl", 10*time.Second,
		"Consul session TTL for the outbox-forwarder leader lock (10s Consul floor)")
	pflag.Duration("leader_election.renew_interval", 5*time.Second,
		"how often the leader renews its Consul session heartbeat")
	pflag.Duration("leader_election.err_cooldown", 5*time.Second,
		"backoff after a transient Consul error before retrying")
	pflag.Duration("leader_election.blocking_wait", 10*time.Second,
		"max duration a Consul blocking query is held open before being reissued; "+
			"acts as a safety-net upper bound on failover latency if a change notification is ever missed")
	pflag.Duration("leader_election.lock_delay", time.Second,
		"Consul post-release anti-flapping grace period; must be > 0 per Consul's session API")
}

func (c *Config) validate() error {
	if c.Service.Addr == "" {
		return errors.New("config: service.addr is required")
	}

	if err := appconfig.ValidateGRPCConn("service.conn", c.Service.Connection); err != nil {
		return err
	}

	if c.Log.Level == "" {
		c.Log.Level = "info"
	}

	if c.Postgres.DSN == "" {
		return errors.New("config: postgres.dsn is required (use --postgres.dsn or POSTGRES_DSN env)")
	}

	if c.Redis.Addr == "" {
		return errors.New("config: redis.addr is required")
	}

	if c.Consul.Addr == "" {
		return errors.New("config: consul.addr is required")
	}

	if c.Pubsub.URL == "" {
		return errors.New("config: pubsub.url is required (use --pubsub.url or PUBSUB_URL env)")
	}

	if !strings.HasPrefix(c.Pubsub.URL, "amqp://") && !strings.HasPrefix(c.Pubsub.URL, "amqps://") {
		return errors.New("config: pubsub.url must start with amqp:// or amqps://")
	}

	if c.LeaderElection.TTL < 10*time.Second {
		return errors.New("config: leader_election.ttl must be at least 10s (Consul session_ttl_min floor)")
	}

	// Mirrors Patroni's loop_wait/ttl safety rule: renewal must fit at least
	// twice inside the TTL window, so a single missed heartbeat doesn't cost leadership.
	if c.LeaderElection.RenewInterval*2 > c.LeaderElection.TTL {
		return errors.New("config: leader_election.renew_interval must be at most half of leader_election.ttl")
	}

	if c.LeaderElection.ErrCooldown <= 0 {
		return errors.New("config: leader_election.err_cooldown must be positive")
	}

	if c.LeaderElection.BlockingWait <= 0 {
		return errors.New("config: leader_election.blocking_wait must be positive")
	}

	// Consul's session API rejects/ignores a zero LockDelay (falls back to its own 15s default),
	// so this must be explicitly positive to have any effect.
	if c.LeaderElection.LockDelay <= 0 {
		return errors.New("config: leader_election.lock_delay must be positive (Consul requires > 0)")
	}

	return nil
}
