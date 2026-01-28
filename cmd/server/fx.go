package server

import (
	"github.com/webitel/im-thread-service/config"
	leader "github.com/webitel/im-thread-service/infra/discovery/consul"
	"github.com/webitel/im-thread-service/infra/pubsub"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
	"github.com/webitel/im-thread-service/infra/tls"
	webiteldi "github.com/webitel/im-thread-service/infra/webitel/di"
	grpchandler "github.com/webitel/im-thread-service/internal/handler/grpc"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/store/postgres"
	"github.com/webitel/webitel-go-kit/infra/discovery"
	"go.uber.org/fx"
)

func NewApp(cfg *config.Config) *fx.App {
	return fx.New(
		fx.Provide(
			func() *config.Config { return cfg },
			ProvideLogger,
			ProvideWatermillLogger,
			ProvideSD,
		),
		fx.Invoke(func(discovery discovery.DiscoveryProvider) error { return nil }),
		tls.Module,
		pubsub.Module,
		postgres.Module,
		leader.Module,
		service.Module,
		grpchandler.Module,
		grpcsrv.Module,
		webiteldi.Module,
	)
}
