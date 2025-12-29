package pubsub

import (
	"errors"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/webitel/im-thread-service/infra/pubsub/factory"
)

type Provider interface {
	GetRouter() *message.Router
	GetFactory() factory.Factory
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
