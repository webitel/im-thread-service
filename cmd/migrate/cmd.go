package migrate

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/migrations"
)


type migrator struct {
	cfg *config.Config
	log *slog.Logger
}

func NewMigrator(cfg *config.Config, log *slog.Logger) *migrator {
	return &migrator{
		cfg: cfg,
		log: log,
	}
}

func (m *migrator) Run(ctx context.Context) error {
	conf, err := pgxpool.ParseConfig(m.cfg.Postgres.DSN)
	if err != nil {
		return err
	}

	db := stdlib.OpenDB(*conf.ConnConfig)
	defer db.Close()

	goose.SetLogger(newLogger(m.log))
	goose.SetVerbose(true)
	store, err := database.NewStore(database.DialectPostgres, "im_contact_schema_version")
	if err != nil {
		return err
	}

	noopDialect := goose.Dialect("")
	provider, err := goose.NewProvider(noopDialect, db, migrations.EmbedMigrations, goose.WithStore(store))
	if err != nil {
		return err
	}

	res, err := provider.Up(ctx)
	if err != nil {
		return err
	}

	for _, r := range res {
		if r.Error != nil {
			m.log.Error("unable to apply migration", "err", r.Error)
		} else {
			m.log.Info("applied migration")
		}
	}

	return nil
}

type migrateLogger struct {
	log *slog.Logger
}

func newLogger(log *slog.Logger) *migrateLogger {
	return &migrateLogger{log: log}
}

func (l *migrateLogger) Printf(format string, args ...any) {
	l.log.Info(fmt.Sprintf(format, args...))
}

func (l *migrateLogger) Fatalf(format string, args ...any) {
	l.log.Error(fmt.Sprintf(format, args...))
}