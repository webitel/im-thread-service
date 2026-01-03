package pg

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type TxKey struct{}

type PgxDB struct {
	master *pgxpool.Pool
	logger *slog.Logger
}

func New(ctx context.Context, logger *slog.Logger, dsn string) (*PgxDB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %v", err)
	}

	dbpool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %v", err)
	}

	if err := dbpool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %v", err)
	}

	return &PgxDB{
		master: dbpool,
		logger: logger,
	}, nil
}

func (d *PgxDB) Master() *pgxpool.Pool {
	return d.master
}

func (d *PgxDB) Tx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(TxKey{}).(pgx.Tx)
	return tx, ok
}

func (d *PgxDB) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return pgx.BeginFunc(ctx, d.master, func(tx pgx.Tx) error {
		ctxWithTx := context.WithValue(ctx, TxKey{}, tx)
		err := fn(ctxWithTx)
		if err != nil {
			d.logger.Warn("Transaction rollback due to error", "error", err)
		} else {
			d.logger.Info("Transaction commit")
		}
		return err
	})
}

func (d *PgxDB) Executor(ctx context.Context) Querier {
	if tx, ok := ctx.Value(TxKey{}).(pgx.Tx); ok {
		return tx
	}
	return d.master
}
