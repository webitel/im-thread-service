package config

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"github.com/webitel/webitel-go-kit/appconfig"
)

type Config struct {
	Service  ServiceConfig      `mapstructure:"service"`
	Log      appconfig.Log      `mapstructure:"log"`
	Postgres appconfig.Postgres `mapstructure:"postgres"`
	Redis    appconfig.Redis    `mapstructure:"redis"`
	Consul   appconfig.Consul   `mapstructure:"consul"`
	Pubsub   appconfig.Pubsub   `mapstructure:"pubsub"`
	Profiler appconfig.Profiler `mapstructure:"profiler"`
}

type ServiceConfig struct {
	ID         string             `mapstructure:"id"`
	Addr       string             `mapstructure:"addr"`
	Connection appconfig.GRPCConn `mapstructure:"conn"`
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
		return nil, fmt.Errorf("config: postgres.dsn is required")
	}

	return cfg, nil
}

func registerServiceFlags() {
	pflag.String("service.id", "", "Service instance ID (required)")
	pflag.String("service.addr", "localhost:8080", "gRPC listen address")
	pflag.Bool("service.conn.verify_certs", true, "Verify TLS certificates on outbound gRPC connections")
	pflag.String("service.conn.ca", "", "CA certificate path")
	pflag.String("service.conn.cert", "", "Server certificate path")
	pflag.String("service.conn.key", "", "Server certificate key path")
	pflag.String("service.conn.client.ca", "", "Client CA certificate path")
	pflag.String("service.conn.client.cert", "", "Client certificate path")
	pflag.String("service.conn.client.key", "", "Client certificate key path")
}

func (c *Config) validate() error {
	if c.Service.ID == "" {
		return fmt.Errorf("config: service.id is required (use --service.id or SERVICE_ID env)")
	}
	if c.Service.Addr == "" {
		return fmt.Errorf("config: service.addr is required")
	}
	if err := appconfig.ValidateGRPCConn("service.conn", c.Service.Connection); err != nil {
		return err
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Postgres.DSN == "" {
		return fmt.Errorf("config: postgres.dsn is required (use --postgres.dsn or POSTGRES_DSN env)")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("config: redis.addr is required")
	}
	if c.Consul.Addr == "" {
		return fmt.Errorf("config: consul.addr is required")
	}
	if c.Pubsub.URL == "" {
		return fmt.Errorf("config: pubsub.url is required (use --pubsub.url or PUBSUB_URL env)")
	}
	if !strings.HasPrefix(c.Pubsub.URL, "amqp://") && !strings.HasPrefix(c.Pubsub.URL, "amqps://") {
		return fmt.Errorf("config: pubsub.url must start with amqp:// or amqps://")
	}
	return nil
}
