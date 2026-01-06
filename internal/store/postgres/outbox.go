package postgres

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/events"
	"github.com/webitel/im-thread-service/internal/store"
)

type outboxStore struct {
	// q is a querier that can be either a connection pool or a transaction
	q      Querier
	config sql.PublisherConfig
	logger watermill.LoggerAdapter
}

// NewOutboxStore returns a new outbox store, given a querier and a logger.
// The querier is used to execute queries or provide a transaction for the watermill publisher.
func NewOutboxStore(q Querier, logger watermill.LoggerAdapter) store.OutboxStore {
	return &outboxStore{
		q:      q,
		logger: logger,
		config: sql.PublisherConfig{
			SchemaAdapter: sql.DefaultPostgreSQLSchema{
				GenerateMessagesTableName: func(topic string) string {
					return "im_message.messages_outbox"
				},
			},
		},
	}
}

// Interface guard
var (
	_ store.OutboxStore = (*outboxStore)(nil)
)

// Publish adds an event to the outbox table within the current transaction.
// It fails if the querier is not an active pgx.Tx.
func (o *outboxStore) Publish(ctx context.Context, topic string, event events.Outboxer) error {
	// Watermill SQL publisher requires a transaction to ensure atomicity.
	tx, ok := o.q.(pgx.Tx)
	if !ok {
		return fmt.Errorf("outbox publish: transaction required (querier is not pgx.Tx)")
	}

	ev, err := event.ToOutbox()
	if err != nil {
		return fmt.Errorf("outbox publish: %w", err)
	}

	// sql.TxFromPgx converts pgx.Tx to *sql.Tx compatible with watermill
	publisher, err := sql.NewPublisher(sql.TxFromPgx(tx), o.config, o.logger)
	if err != nil {
		return fmt.Errorf("failed to create outbox publisher: %w", err)
	}

	msg := message.NewMessage(ev.ID.String(), ev.Payload)
	for k, v := range ev.Metadata {
		msg.Metadata.Set(k, v)
	}

	return publisher.Publish(topic, msg)
}

// Cleanup removes processed messages from the outbox table based on retention days.
func (o *outboxStore) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	const query = `
        DELETE FROM im_message.messages_outbox
        WHERE created_at < now() - ($1 * interval '1 day')
          AND "offset" <= (
              SELECT COALESCE(offset_value, 0)
              FROM im_message.messages_offsets
              WHERE consumer_group = 'im-thread-outbox-forwarder'
                AND topic = 'im.messages'
          )
    `

	result, err := o.q.Exec(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("outbox cleanup failed: %w", err)
	}

	return result.RowsAffected(), nil
}
