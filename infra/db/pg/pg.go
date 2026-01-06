package pg

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

func ProvidePgxPool(db *PgxDB) *pgxpool.Pool {
	return db.Master()
}
