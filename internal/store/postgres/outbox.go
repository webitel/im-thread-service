package postgres

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/events"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type outboxStore struct {
	// [QUERIER]
	// Supports both connection pool for cleanup and pgx.Tx for publishing
	q        Querier
	config   sql.PublisherConfig
	wmlogger watermill.LoggerAdapter
}

func NewOutboxStore(q Querier, wmlogger watermill.LoggerAdapter) store.OutboxStore {
	return &outboxStore{
		q:        q,
		wmlogger: wmlogger,
		config: sql.PublisherConfig{
			SchemaAdapter: sql.DefaultPostgreSQLSchema{
				GenerateMessagesTableName: func(topic string) string {
					return "im_message.messages_outbox"
				},
			},
		},
	}
}

var _ store.OutboxStore = (*outboxStore)(nil)

func (o *outboxStore) Publish(ctx context.Context, topic string, event events.Outboxer) error {
	// [ATOMICITY_CHECK]
	// Watermill SQL publisher MUST run within an active transaction
	// to ensure the business logic and event storage are committed together.
	tx, ok := o.q.(pgx.Tx)
	if !ok {
		return fmt.Errorf("outbox publish: transaction required (querier is not pgx.Tx)")
	}

	ev, err := event.ToOutbox()
	if err != nil {
		return fmt.Errorf("outbox publish: %w", err)
	}

	// [TX_ADAPTER]
	// Wrap pgx.Tx into a Watermill-compatible SQL transaction
	publisher, err := sql.NewPublisher(sql.TxFromPgx(tx), o.config, o.wmlogger)
	if err != nil {
		return fmt.Errorf("failed to create outbox publisher: %w", err)
	}

	msg := message.NewMessage(ev.ID.String(), ev.Payload)
	for k, v := range ev.Metadata {
		msg.Metadata.Set(k, v)
	}

	// [ADD_METADATA] Force store the target routing key in metadata
	msg.Metadata.Set("x-routing-key", topic)

	return publisher.Publish(topic, msg)
}

func (o *outboxStore) Cleanup(ctx context.Context, opt *model.OutboxCleanupOptions) (int64, error) {
	if opt.BatchSize <= 0 {
		opt.BatchSize = 5000
	}

	args := pgx.NamedArgs{
		"retention_days": opt.RetentionDays,
		"batch_size":     opt.BatchSize,
		"consumer_group": opt.ConsumerGroup,
		"topic":          opt.Topic,
	}

	// [CLEANUP_QUERY]
	// Using CTE and Tuple Comparison (transaction_id, offset) to safely delete
	// only those records that have been acknowledged by the forwarder.
	const query = `
        WITH to_delete AS (
            SELECT transaction_id, "offset"
            FROM im_message.messages_outbox
            WHERE 
                (transaction_id, "offset") <= (
                    SELECT last_processed_transaction_id, offset_acked
                    FROM im_message.messages_offsets
                    WHERE consumer_group = @consumer_group
                      AND topic = @topic
                    LIMIT 1
                )
                AND created_at < now() - (@retention_days * interval '1 day')
            LIMIT @batch_size
        )
        DELETE FROM im_message.messages_outbox
        USING to_delete
        WHERE im_message.messages_outbox.transaction_id = to_delete.transaction_id
          AND im_message.messages_outbox."offset" = to_delete."offset"`

	var totalDeleted int64
	for {
		// [BATCHED_DELETE]
		// Loop execution to prevent long-lived locks and WAL bloat
		result, err := o.q.Exec(ctx, query, args)
		if err != nil {
			return totalDeleted, fmt.Errorf("outbox cleanup: %w", err)
		}

		rowsAffected := result.RowsAffected()
		totalDeleted += rowsAffected

		if rowsAffected < int64(opt.BatchSize) {
			break
		}

		select {
		case <-ctx.Done():
			return totalDeleted, ctx.Err()
		default:
		}
	}

	return totalDeleted, nil
}
