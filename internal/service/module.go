package service

import (
	"github.com/webitel/im-thread-service/internal/adapter/pubsub"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"service",

	fx.Provide(
		// Domain services
		fx.Annotate(
			NewMessageService,
			fx.As(new(Messager)),
		),

		fx.Annotate(
			NewThreadService,
			fx.As(new(ThreadManager)),
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
	),

	fx.Invoke(
		pubsub.RegisterOutboxForwarder,
	),
)
