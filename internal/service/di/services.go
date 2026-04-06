package di

import (
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
	"github.com/webitel/im-thread-service/internal/adapter/pubsub"
	"github.com/webitel/im-thread-service/internal/service"
	"github.com/webitel/im-thread-service/internal/service/decorators"
	"github.com/webitel/im-thread-service/internal/store/postgres"
	"go.uber.org/fx"
)

// var Module = fx.Module(
// 	"service",
// 	fx.Provide(
// 		fx.Annotate(
// 			service.NewThreadService,
// 			fx.As(new(service.ThreadManager)),
// 		),
// 		fx.Annotate(
// 			service.NewMessageHistory,
// 			fx.As(new(service.MessageHistorySearcher)),
// 		),
// 	),
// 	fx.Decorate(func(logger *slog.Logger, base service.ThreadManager, imContactsClient *imcontact.Client) service.ThreadManager {
// 		return decorators.NewThreadWithCanSendDecorator(logger, base, *imContactsClient)
// 	}),
// )

var ServiceModule = fx.Module(
	"service",

	fx.Provide(
		fx.Annotate(
			service.NewMessageService,
			fx.As(new(service.Messager)),
		),

		fx.Annotate(
			service.NewThreadService,
			fx.As(new(service.ThreadManager)),
			fx.As(new(service.ThreadProvisioner)),
			fx.As(new(service.ThreadSearcher)),
		),

		fx.Annotate(
			service.NewMessageHistory,
			fx.ResultTags(`name:"base_message_history"`),
			fx.As(new(service.MessageHistorySearcher)),
		),

		fx.Annotate(
			func(
				base service.MessageHistorySearcher,
				storage *storageclient.Client,
			) service.MessageHistorySearcher {
				return decorators.NewMessageHistoryEnricher(base, storage)
			},
			fx.ParamTags(`name:"base_message_history"`),
			fx.ResultTags(`name:"enriched_message_history"`),
			fx.As(new(service.MessageHistorySearcher)),
		),

		fx.Annotate(
			service.NewMediaProcessor,
			fx.As(new(service.MediaProcessor)),
		),

		// PubSub infrastructure
		fx.Annotate(
			pubsub.NewOutboxSubscriber,
			fx.As(new(pubsub.OutboxSubscriber)),
		),

		fx.Annotate(
			pubsub.NewRabbitPublisher,
			fx.As(new(pubsub.EventPublisher)),
		),

		fx.Annotate(
			postgres.NewThreadVariablesStore,
			fx.As(new(service.ThreadVariablesStore)),
		),
	),

	fx.Invoke(
		pubsub.RegisterOutboxForwarder,
	),
)

var DecoratorModule = fx.Module(
	"decorators",
	fx.Decorate(
		func(
			base service.MessageHistorySearcher,
			storage *storageclient.Client,
		) service.MessageHistorySearcher {
			return decorators.NewMessageHistoryEnricher(base, storage)
		},
	),
)
