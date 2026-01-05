package config

import (
	"fmt"
	"log"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Config struct {
	Service  ServiceConfig  `mapstructure:"service"`
	Log      LogConfig      `mapstructure:"log"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Consul   ConsulConfig   `mapstructure:"consul"`
	Pubsub   PubsubConfig   `mapstructure:"pubsub"`
}

type ServiceConfig struct {
	Id        string `mapstructure:"id"`
	Address   string `mapstructure:"addr"`
	SecretKey string `mapstructure:"secret"`
}

type LogConfig struct {
	Level   string `mapstructure:"level"`
	JSON    bool   `mapstructure:"json"`
	Otel    bool   `mapstructure:"otel"`
	File    string `mapstructure:"file"`
	Console bool   `mapstructure:"console"`
}

type PostgresConfig struct {
	DSN string `mapstructure:"dsn"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type ConsulConfig struct {
	Address       string `mapstructure:"addr"`
	PublicAddress string `mapstructure:"grpc_addr"`
}

type PubsubConfig struct {
	URL    string `mapstructure:"broker_url"`
	Driver string `mapstructure:"broker_driver"`
}

func LoadConfig() (*Config, error) {
	defineFlags()
	pflag.Parse()

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		return nil, err
	}

	// Explicitly bind environment variables for nested structures
	viper.BindEnv("service.id", "SERVICE_ID")
	viper.BindEnv("service.addr", "SERVICE_ADDR")
	viper.BindEnv("service.secret", "SERVICE_SECRET")
	viper.BindEnv("postgres.dsn", "POSTGRES_DSN")
	viper.BindEnv("pubsub.broker_url", "PUBSUB_BROKER_URL")
	viper.BindEnv("pubsub.broker_driver", "PUBSUB_BROKER_DRIVER")

	cfg := &Config{}

	configFile := viper.GetString("config_file")
	if configFile != "" {
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unable to decode into struct: %v", err)
	}

	if configFile != "" {
		viper.OnConfigChange(func(e fsnotify.Event) {
			log.Printf("Config file changed: %s", e.Name)
			newCfg := &Config{}
			if err := viper.Unmarshal(newCfg); err == nil {
				if err := newCfg.validate(); err == nil {
					*cfg = *newCfg
				}
			}
		})
		viper.WatchConfig()
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func defineFlags() {
	pflag.String("config_file", "", "Configuration file path")

	pflag.String("service.id", "", "Service ID")
	pflag.String("service.addr", "localhost:8080", "Service address")
	pflag.String("service.secret", "", "Service secret key")

	pflag.String("log.level", "info", "Log level")
	pflag.Bool("log.json", false, "Enable JSON logging")
	pflag.Bool("log.console", true, "Enable console logging")

	pflag.String("postgres.dsn", "", "PostgreSQL connection string")
	pflag.String("redis.addr", "localhost:6379", "Redis server address")
	pflag.String("consul.addr", "localhost:8500", "Consul server address")

	pflag.String("pubsub.broker_url", "", "PubSub broker connection URL")
	pflag.String("pubsub.broker_driver", "amqp", "PubSub driver (e.g. amqp)")
}

func (c *Config) validate() error {
	if c.Service.Id == "" {
		return fmt.Errorf("config: service.id is required")
	}
	if c.Service.SecretKey == "" {
		return fmt.Errorf("config: service.secret is required")
	}
	if c.Postgres.DSN == "" {
		return fmt.Errorf("config: postgres.dsn is required")
	}
	if c.Pubsub.URL == "" {
		return fmt.Errorf("config: pubsub.broker_url is required")
	}
	if c.Pubsub.Driver == "" {
		return fmt.Errorf("config: pubsub.broker_driver is required")
	}

	if !strings.HasPrefix(c.Pubsub.URL, "amqp://") && !strings.HasPrefix(c.Pubsub.URL, "amqps://") {
		return fmt.Errorf("config: pubsub.broker_url must start with amqp:// or amqps://")
	}

	return nil
}
