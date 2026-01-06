package cmd

import (
	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/infra/db/pg"
	leader "github.com/webitel/im-thread-service/infra/discovery/consul"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
	grpchandler "github.com/webitel/im-thread-service/internal/handler/grpc"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/store/postgres"
	"go.uber.org/fx"
)

func NewApp(cfg *config.Config) *fx.App {
	return fx.New(
		fx.Provide(
			func() *config.Config { return cfg },
			ProvideLogger,
			ProvideWatermillLogger,
			ProvideSD,
			ProvidePubSub,
			ProvideNewDBConnection,
			pg.ProvidePgxPool,
		),
		postgres.Module,
		leader.Module,
		service.Module,
		grpcsrv.Module,
		grpchandler.Module,
	)
}
