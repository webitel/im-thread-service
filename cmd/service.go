package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/webitel/im-thread-service/infra/server/grpc"
	"go.uber.org/fx"
)

func StartGrpcServer(lc fx.Lifecycle, srv *grpc.Server, log *slog.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				log.Info(fmt.Sprintf("listen grpc %s:%d", srv.Host(), srv.Port()))
				if err := srv.Listen(); err != nil {
					log.Error("grpc server error", err)
				}
			}()
			return nil
		},
	})
}
