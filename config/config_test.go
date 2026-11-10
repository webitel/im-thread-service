package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webitel/webitel-go-kit/appconfig/secret"

	"github.com/webitel/im-thread-service/config"
)

// startAsSystemdUnit makes Load* see what a systemd unit with
// LoadCredential*= sees: no secret env vars, only $CREDENTIALS_DIRECTORY
// holding the given credentials. It also resets the global flag set, since
// the loaders register into pflag.CommandLine and parse os.Args.
func startAsSystemdUnit(t *testing.T, creds map[string]string) string {
	t.Helper()

	for _, env := range []string{
		"CONFIG_FILE",
		"POSTGRES_DSN", "POSTGRES_DSN_FILE",
		"REDIS_PASSWORD", "REDIS_PASSWORD_FILE",
		"PUBSUB_URL", "PUBSUB_URL_FILE",
		"SERVICE_CONN_KEY", "SERVICE_CONN_CLIENT_KEY",
	} {
		t.Setenv(env, "")
		os.Unsetenv(env)
	}

	dir := t.TempDir()
	for name, value := range creds {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0o400))
	}

	t.Setenv("CREDENTIALS_DIRECTORY", dir)

	args, flags := os.Args, pflag.CommandLine
	os.Args = []string{"server"}
	pflag.CommandLine = pflag.NewFlagSet("server", pflag.ContinueOnError)

	t.Cleanup(func() { os.Args, pflag.CommandLine = args, flags })

	return dir
}

func TestLoadServerConfigSecretsFromCredentials(t *testing.T) {
	dir := startAsSystemdUnit(t, map[string]string{
		"postgres.dsn":     "postgres://db.example.com/im_test",
		"redis.password":   "test-redis-password",
		"pubsub.url":       "amqp://mq.example.com/",
		"service.conn.key": "test-only-key",
	})
	t.Setenv("SERVICE_CONN_CA", "/etc/test/ca.crt")
	t.Setenv("SERVICE_CONN_CERT", "/etc/test/service.crt")

	cfg, err := config.LoadServerConfig()
	require.NoError(t, err)
	assert.Equal(t, "postgres://db.example.com/im_test", cfg.Postgres.DSN)
	assert.Equal(t, "test-redis-password", cfg.Redis.Password)
	assert.Equal(t, "amqp://mq.example.com/", cfg.Pubsub.URL)
	assert.Equal(t, filepath.Join(dir, "service.conn.key"), cfg.Service.Connection.Key)
}

func TestLoadServerConfigPlainSecretShadowingCredentialFails(t *testing.T) {
	startAsSystemdUnit(t, map[string]string{"postgres.dsn": "postgres://db.example.com/new"})
	t.Setenv("POSTGRES_DSN", "postgres://db.example.com/old")

	_, err := config.LoadServerConfig()
	require.ErrorIs(t, err, secret.ErrConflict)
}
