package server

import (
	"github.com/webitel/im-thread-service/config"
	leader "github.com/webitel/im-thread-service/infra/discovery/consul"
	"github.com/webitel/im-thread-service/infra/pubsub"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
	"github.com/webitel/im-thread-service/infra/tls"
	webiteldi "github.com/webitel/im-thread-service/infra/webitel/di"
	grpchandler "github.com/webitel/im-thread-service/internal/handler/grpc"
	"github.com/webitel/im-thread-service/internal/service/di"
	"github.com/webitel/im-thread-service/internal/store/postgres"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	"github.com/webitel/webitel-go-kit/infra/profiler"
	"go.uber.org/fx"
)

func NewApp(cfg *config.Config) *fx.App {
	return fx.New(
		fx.Provide(
			func() *config.Config { return cfg },
			ProvideLogger,
			ProvideWatermillLogger,
			ProvideSD,
			ProvideProfiler,
		),
		fx.Invoke(func(discovery discovery.DiscoveryProvider) error { return nil }),
		tls.Module,
		pubsub.Module,
		postgres.Module,
		leader.Module,
		webiteldi.Module,
		di.ServiceModule,
		grpchandler.Module,
		grpcsrv.Module,
		profiler.Module,
	)
}
