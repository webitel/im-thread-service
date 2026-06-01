package pubsub

import (
	"context"
	"errors"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"go.uber.org/fx"

	"github.com/webitel/im-thread-service/config"
	"github.com/webitel/im-thread-service/infra/pubsub/factory"
	"github.com/webitel/im-thread-service/infra/pubsub/factory/amqp"
)

type Provider interface {
	GetRouter() *message.Router
	GetFactory() factory.Factory
}

var Module = fx.Module("pubsub",
	fx.Provide(
		fx.Annotate(
			ProvidePubSub,
		),
	),
)

func ProvidePubSub(cfg *config.Config, l *slog.Logger, lc fx.Lifecycle) (Provider, error) {
	var (
		pubsubConfig  = cfg.Pubsub
		loggerAdapter = watermill.NewSlogLogger(l)
		pubsubFactory factory.Factory
		err           error
	)

	switch pubsubConfig.Driver {
	case "amqp":
		pubsubFactory, err = amqp.NewFactory(pubsubConfig.URL, loggerAdapter)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("pubsub driver not supported")
	}

	router, err := message.NewRouter(message.RouterConfig{}, loggerAdapter)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			return router.Close()
		},
		OnStart: func(ctx context.Context) error {
			go func() {
				if err := router.Run(ctx); err != nil {
					slog.Error("[BROKER] running router", "error", err)
				}
			}()

			return nil
		},
	})

	return NewDefaultProvider(router, pubsubFactory)
}

type DefaultProvider struct {
	router  *message.Router
	factory factory.Factory
}

func NewDefaultProvider(router *message.Router, factory factory.Factory) (Provider, error) {
	if router == nil {
		return nil, errors.New("router is required")
	}

	if factory == nil {
		return nil, errors.New("factory is required")
	}

	return &DefaultProvider{
		router:  router,
		factory: factory,
	}, nil
}

func (p *DefaultProvider) GetRouter() *message.Router {
	return p.router
}

func (p *DefaultProvider) GetFactory() factory.Factory {
	return p.factory
}
