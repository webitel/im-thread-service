package pubsub

import (
	"github.com/ThreeDotsLabs/watermill/message"
	infrapubsub "github.com/webitel/im-thread-service/infra/pubsub"
	"github.com/webitel/im-thread-service/infra/pubsub/factory"
)

type EventPublisher interface {
	message.Publisher
}

type rabbitPublisher struct {
	publisher message.Publisher
}

func NewRabbitPublisher(p infrapubsub.Provider) (EventPublisher, error) {
	pub, err := p.GetFactory().BuildPublisher(&factory.PublisherConfig{
		Exchange: factory.ExchangeConfig{
			Name:    "im_message.events",
			Type:    "topic",
			Durable: true,
		},
	})
	if err != nil {
		return nil, err
	}

	return &rabbitPublisher{publisher: pub}, nil
}

func (r *rabbitPublisher) Publish(topic string, messages ...*message.Message) error {
	return r.publisher.Publish(topic, messages...)
}

func (r *rabbitPublisher) Close() error { return r.publisher.Close() }
