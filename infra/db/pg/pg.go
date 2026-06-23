package pg

import (
	"context"
	"fmt"

	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/webitel-go-kit/infra/pgw"
	"github.com/webitel/webitel-go-kit/infra/pgw/verifier/goose"
)

const (
	defaultMigrationTableName = "im_thread_schema_version"
	lastMigrationVersion      = "20260603120007"
)

func New(ctx context.Context, conf *config.Config) (*pgw.PoolManager, error) {
	migrationVerifier, err := goose.NewGooseMigrationVerifier(defaultMigrationTableName, lastMigrationVersion)
	if err != nil {
		return nil, fmt.Errorf("create migration verifier: %v", err)
	}

	return pgw.NewPoolManager(
		ctx,
		pgw.WithMigrationVerifier(migrationVerifier),
		pgw.WithPrimaryConfig(pgw.PrimaryConfig{DSN: conf.Postgres.DSN}),
	)

}
