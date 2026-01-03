package pubsub

import (
	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxSubscriber interface {
	message.Subscriber
}

func NewOutboxSubscriber(
	pool *pgxpool.Pool,
	logger watermill.LoggerAdapter,
) (OutboxSubscriber, error) {
	return sql.NewSubscriber(
		sql.BeginnerFromPgx(pool),
		sql.SubscriberConfig{
			ConsumerGroup: "im-thread-outbox-forwarder",

			SchemaAdapter: sql.DefaultPostgreSQLSchema{
				GenerateMessagesTableName: func(topic string) string {
					return "im_message.messages_outbox"
				},
			},

			OffsetsAdapter: sql.DefaultPostgreSQLOffsetsAdapter{
				GenerateMessagesOffsetsTableName: func(topic string) string {
					return "im_message.watermill_offsets"
				},
			},
			InitializeSchema: false,
		},
		logger,
	)
}
