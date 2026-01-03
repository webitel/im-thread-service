package postgres

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/webitel/im-thread-service/infra/db/pg"
	"github.com/webitel/im-thread-service/internal/domain/events"
	"github.com/webitel/im-thread-service/internal/store"
)

type outboxStore struct {
	db     *pg.PgxDB
	config sql.PublisherConfig
	logger watermill.LoggerAdapter
}

func NewOutboxStore(db *pg.PgxDB, logger watermill.LoggerAdapter) store.OutboxStore {
	return &outboxStore{
		db:     db,
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

func (o *outboxStore) Publish(ctx context.Context, topic string, event events.Outboxer) error {
	tx, ok := o.db.Tx(ctx)
	if !ok {
		return fmt.Errorf("outbox publish: transaction required")
	}

	ev, err := event.ToOutbox()
	if err != nil {
		return fmt.Errorf("outbox publish: %w", err)
	}
	publisher, err := sql.NewPublisher(sql.TxFromPgx(tx), o.config, o.logger)
	if err != nil {
		return err
	}

	msg := message.NewMessage(ev.ID.String(), ev.Payload)
	for k, v := range ev.Metadata {
		msg.Metadata.Set(k, v)
	}

	return publisher.Publish(topic, msg)
}

func (o *outboxStore) MarkAsPublished(ctx context.Context, id string) error {
	_, err := o.db.Executor(ctx).Exec(ctx, `
        UPDATE im_message.messages_outbox 
        SET published_at = now() 
        WHERE uuid = $1`,
		id,
	)
	return err
}

func (o *outboxStore) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	query := `
        DELETE FROM im_message.messages_outbox
        WHERE created_at < now() - ($1 * interval '1 day')
          AND "offset" <= (
              SELECT COALESCE(offset_value, 0)
              FROM im_message.watermill_offsets
              WHERE consumer_group = 'im-thread-outbox-forwarder'
                AND topic = 'im.messages'
          )
    `

	result, err := o.db.Executor(ctx).Exec(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("outbox cleanup failed: %w", err)
	}

	return result.RowsAffected(), nil
}
