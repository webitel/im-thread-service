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
	// [SQL_SUBSCRIBER_SETUP]
	// Configure subscriber to poll events from the Postgres outbox table.
	// Uses pgxpool for connection management.
	return sql.NewSubscriber(
		sql.BeginnerFromPgx(pool),
		sql.SubscriberConfig{
			// [CONSUMER_GROUP]
			// Identifies this instance in messages_offsets table to track progress
			ConsumerGroup: "im-thread-outbox-forwarder",

			// [SCHEMA_MAPPING]
			// Point Watermill to our custom schema and table naming convention
			SchemaAdapter: sql.DefaultPostgreSQLSchema{
				GenerateMessagesTableName: func(_ string) string {
					return "im_message.messages_outbox"
				},
			},

			// [OFFSETS_MANAGEMENT]
			// Store ack progress in messages_offsets to ensure At-Least-Once delivery
			OffsetsAdapter: sql.DefaultPostgreSQLOffsetsAdapter{
				GenerateMessagesOffsetsTableName: func(_ string) string {
					return "im_message.messages_offsets"
				},
			},

			// Schema is managed via golang-migrate/goose, disable auto-init
			InitializeSchema: false,
		},
		logger,
	)
}
