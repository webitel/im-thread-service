package pg

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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

	const (
		maxAttempts = 5
		delay       = 2 * time.Second
	)

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := dbpool.Ping(ctx); err == nil {
			if attempt > 1 {
				logger.Info("Database connection established", slog.Int("attempts", attempt))
			}
			return &PgxDB{
				master: dbpool,
				logger: logger,
			}, nil
		} else {
			lastErr = err
			logger.Warn("Failed to ping database, retrying...",
				slog.Int("attempt", attempt),
				slog.Int("max_attempts", maxAttempts),
				slog.String("error", err.Error()),
			)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}

	return nil, fmt.Errorf("database unreachable after %d attempts: %v", maxAttempts, lastErr)
}

func (d *PgxDB) Master() *pgxpool.Pool {
	return d.master
}

func (d *PgxDB) Tx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(TxKey{}).(pgx.Tx)
	return tx, ok
}

func ProvidePgxPool(db *PgxDB) *pgxpool.Pool {
	return db.Master()
}

func (d *PgxDB) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := d.master.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback(context.Background())
			d.logger.Error("Transaction panic recovered, rollback executed",
				slog.Any("panic", r),
			)
			panic(r)
		}
	}()

	ctxWithTx := context.WithValue(ctx, TxKey{}, tx)
	err = fn(ctxWithTx)
	if err != nil {
		d.logger.Warn("Transaction rollback due to error", "error", err)
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("rollback error: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	d.logger.Debug("Transaction committed successfully")
	return nil
}

func (d *PgxDB) Executor(ctx context.Context) Querier {
	if tx, ok := ctx.Value(TxKey{}).(pgx.Tx); ok {
		return tx
	}
	return d.master
}
