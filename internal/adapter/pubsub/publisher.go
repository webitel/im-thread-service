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
	// [RABBITMQ_PUB_SETUP]
	// Initialize RabbitMQ publisher using the internal infra factory.
	// Events are routed through a topic exchange for flexible consumption.
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

// Publish sends messages to the pre-configured RabbitMQ exchange
func (r *rabbitPublisher) Publish(topic string, messages ...*message.Message) error {
	return r.publisher.Publish(topic, messages...)
}

// Close gracefully shuts down the RabbitMQ connection
func (r *rabbitPublisher) Close() error { return r.publisher.Close() }
