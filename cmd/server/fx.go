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
	"github.com/webitel/im-thread-service/internal/service/decorators"
	"github.com/webitel/im-thread-service/internal/store"
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
		webiteldi.Module,
		storeBridgeModule,
		service.Module,
		serviceToHandlerBridgeModule,
		grpchandler.Module,
		grpcsrv.Module,
	)
}

var storeBridgeModule = fx.Module(
	"storeToServiceBridge",
	fx.Provide(
		func(s store.ThreadPermissionStore) service.ThreadPermissionStore {
			return s
		},
		func(s store.MessageHistory) service.MessageHistoryStore {
			return s
		},
	),
)

var serviceToHandlerBridgeModule = fx.Module(
	"serviceToHandlerBridge",
	fx.Provide(
		func(s *service.ThreadManagementService) grpchandler.ThreadManagementService {
			return s
		},
		func(s *service.MessageService) grpchandler.MessageService {
			return s
		},
		func(s *decorators.MessageHistoryEnricher) grpchandler.MessageHistoryService {
			return s
		},
	),
)
