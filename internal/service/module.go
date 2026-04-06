package service

import (
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
	"github.com/webitel/im-thread-service/internal/adapter/pubsub"
<<<<<<< HEAD
	"github.com/webitel/im-thread-service/internal/service/decorators"
=======
	"github.com/webitel/im-thread-service/internal/store/postgres"
>>>>>>> 72cd28b ([WMSG-219] feat(thread.variables): add thread variables support)
	"go.uber.org/fx"
)

var Module = fx.Module(
	"service",

	fx.Provide(
		NewMessageService,
		NewThreadService,
		NewThreadPermissionService,
		NewMessageHistory,
		NewMediaProcessor,

		func(base *MessageHistoryService, storageServiceClient *storageclient.Client) *decorators.MessageHistoryEnricher {
			return decorators.NewMessageHistoryEnricher(base, storageServiceClient)
		},
		pubsub.NewOutboxSubscriber,
		pubsub.NewRabbitPublisher,
	),

	fx.Invoke(
		pubsub.RegisterOutboxForwarder,
	),
)
