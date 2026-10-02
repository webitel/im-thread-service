package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"

	healthfx "github.com/webitel/webitel-go-kit/infra/health/fx"

	"github.com/webitel/im-thread-service/config"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
)

func TestHealth_Lifecycle(t *testing.T) {
	cfg := &config.Config{Health: config.HealthConfig{Addr: freeAddr(t)}}

	srv, err := grpcsrv.New("127.0.0.1:0")
	if err != nil {
		t.Fatalf("new grpc server: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}

	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})

	t.Cleanup(func() {
		if err := rdb.Close(); err != nil {
			t.Errorf("close redis: %v", err)
		}
	})

	app := fx.New(
		fx.NopLogger,
		fx.Supply(cfg, srv, pool, rdb, slog.New(slog.NewTextHandler(io.Discard, nil))),
		healthfx.Module(healthfx.Config{HTTPAddr: cfg.Health.Addr}),
		fx.Invoke(func(lc fx.Lifecycle) {
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					go func() {
						if err := srv.Listen(); err != nil && !errors.Is(err, net.ErrClosed) {
							t.Errorf("grpc listen: %v", err)
						}
					}()

					return nil
				},
				OnStop: func(context.Context) error { return srv.Shutdown() },
			})
		}),
		fx.Invoke(registerHealth),
		healthfx.Shutdown(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := app.Start(ctx); err != nil {
		t.Fatalf("start app: %v", err)
	}

	waitReadyz(t, "http://"+cfg.Health.Addr+"/readyz")

	if err := app.Stop(context.Background()); err != nil {
		t.Fatalf("stop app: %v", err)
	}
}

func waitReadyz(t *testing.T, url string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for {
		resp, err := http.Get(url)
		if err == nil {
			if cerr := resp.Body.Close(); cerr != nil {
				t.Errorf("close body: %v", cerr)
			}

			if resp.StatusCode == http.StatusOK {
				return
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("GET %s never returned 200 while only informational checks fail: last err %v", url, err)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}

	addr := l.Addr().String()

	if err := l.Close(); err != nil {
		t.Fatalf("release free port: %v", err)
	}

	return addr
}
