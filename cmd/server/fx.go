package server

import (
	"go.uber.org/fx"

	"github.com/webitel/webitel-go-kit/infra/discovery"
	"github.com/webitel/webitel-go-kit/infra/profiler"

	"github.com/webitel/im-thread-service/config"
	leader "github.com/webitel/im-thread-service/infra/discovery/consul"
	"github.com/webitel/im-thread-service/infra/pubsub"
	grpcsrv "github.com/webitel/im-thread-service/infra/server/grpc"
	"github.com/webitel/im-thread-service/infra/tls"
	webiteldi "github.com/webitel/im-thread-service/infra/webitel/di"
	imcontact "github.com/webitel/im-thread-service/infra/webitel/im-contact"
	grpchandler "github.com/webitel/im-thread-service/internal/handler/grpc"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/service/decorators"
	"github.com/webitel/im-thread-service/internal/store"
	"github.com/webitel/im-thread-service/internal/store/postgres"
)

func NewApp(cfg *config.Config) *fx.App {
	return fx.New(MainModule(cfg))
}

func MainModule(cfg *config.Config) fx.Option {
	return fx.Options(
		fx.Provide(
			func() *config.Config { return cfg },
			ProvideLogger,
			ProvideWatermillLogger,
			ProvideSD,
			ProvideProfiler,
			ProvideTypingConfig,
			ProvideRedisClient,
			ProvideTypingRateLimiter,
		),
		fx.Invoke(func(_ discovery.DiscoveryProvider) error { return nil }),
		tls.Module,
		pubsub.Module,

		postgres.Module,
		leader.Module,
		storeBridgeModule,

		webiteldi.Module,
		grpcClientsBridgeModule,

		service.Module,
		serviceToHandlerBridgeModule,

		grpchandler.Module,
		grpcsrv.Module,
		profiler.Module,
	)
}

var storeBridgeModule = fx.Module(
	"storeToServiceBridge",
	fx.Provide(
		func(s store.MessageHistory) service.MessageHistoryStore {
			return s
		},
	),
)

var grpcClientsBridgeModule = fx.Module(
	"clientsToServiceBridge",
	fx.Provide(
		func(c *imcontact.Client) service.ThreadPrivacyChecker {
			return c
		},
		func(c *imcontact.Client) service.ContactInfoProvider {
			return c
		},
	),
)

var serviceToHandlerBridgeModule = fx.Module(
	"serviceToHandlerBridge",
	fx.Provide(
		func(s *service.ThreadManagementService) service.ThreadManager {
			return s
		},

		func(s *service.ThreadManagementService) service.BotController {
			return s
		},

		func(s *service.ThreadManagementService) grpchandler.ThreadManagementService {
			return s
		},

		func(s *service.ThreadPermissionService) grpchandler.ThreadPermissionManagementService {
			return s
		},
		func(s *service.MessageService) grpchandler.MessageService {
			return s
		},

		provideTypingBus,
		func(s *service.MessageStatusService) grpchandler.MessageStatusReporter {
			return s
		},
		func(s *decorators.MessageHistoryEnricher) grpchandler.MessageHistoryService {
			return s
		},
	),
)
