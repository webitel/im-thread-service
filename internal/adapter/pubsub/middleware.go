package pubsub

import (
	"context"
	"log"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/webitel/im-thread-service/internal/store"
)

func OutboxMarkAsPublished(outbox store.OutboxStore) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			events, err := h(msg)
			if err != nil {
				return nil, err
			}

			if markErr := outbox.MarkAsPublished(context.Background(), msg.UUID); markErr != nil {
				log.Printf("failed to mark outbox message %s as published: %v", msg.UUID, markErr)
			}

			return events, nil
		}
	}
}
