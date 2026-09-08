package service

import (
	"go.uber.org/fx"

	"github.com/webitel/im-thread-service/config"
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
	"github.com/webitel/im-thread-service/internal/adapter/pubsub"
	"github.com/webitel/im-thread-service/internal/service/decorators"
)

var Module = fx.Module(
	"service",

	fx.Provide(
		func(cfg *config.Config) ReactionCapabilities {
			return NewStaticReactionCapabilities(cfg.Reactions.AllowedEmoji)
		},
		fx.Annotate(newBaseRPCProvidersAdapter, fx.As(new(ProvidersAdapter))),
		NewMessageService,
		NewCommandService,
		NewMessageStatusService,
		NewThreadService,
		NewThreadPermissionService,
		NewThreadTagService,
		NewMessageHistory,
		NewMediaProcessor,
		fx.Annotate(NewDirectThreadCreatorGuard, fx.As(new(DirectThreadCreatorGuarder))),
		fx.Annotate(
			NewDirectThreadCreator,
			fx.As(new(ThreadCreator)),
			fx.ResultTags(`group:"thread_creators"`),
		),

		fx.Annotate(
			NewThreadCreatorsFactory,
			fx.As(new(ThreadCreatorsFactoryProvider)),
			fx.ParamTags(`group:"thread_creators"`),
		),

		func(base *MessageHistoryService, storageServiceClient *storageclient.Client) *decorators.MessageHistoryEnricher {
			return decorators.NewMessageHistoryEnricher(base, storageServiceClient)
		},
		pubsub.NewOutboxSubscriber,
		pubsub.NewRabbitPublisher,
		NewThreadVariables,
	),

	fx.Invoke(
		pubsub.RegisterOutboxForwarder,
	),
)
