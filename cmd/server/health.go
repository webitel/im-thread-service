package server

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/webitel/webitel-go-kit/infra/health"

	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
)

func registerHealth(
	h *health.Registry,
	srv *grpcsrv.Server,
	pool *pgxpool.Pool,
	rdb *redis.Client,
) {
	h.Critical("grpc", health.ListenerCheck(srv.Listener()))
	h.Informational("postgres", pool.Ping)
	h.Informational("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })
}
