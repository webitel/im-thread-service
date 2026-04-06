package service

import (
	storageclient "github.com/webitel/im-thread-service/infra/webitel/storage"
	"github.com/webitel/im-thread-service/internal/adapter/pubsub"
	"github.com/webitel/im-thread-service/internal/service/decorators"
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
