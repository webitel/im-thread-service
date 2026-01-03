package postgres

import (
	"context"
	"encoding/json"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/webitel/im-thread-service/infra/db/pg"
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

// func (o *outboxStore) Add(ctx context.Context, r store.OutboxRecord) error {
// 	tx, _ := o.db.Tx(ctx)

// 	publisher, err := sql.NewPublisher(
// 		sql.BeginnerFromPgx(dbExecutor),
// 		o.config,
// 		o.logger,
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to create watermill publisher: %w", err)
// 	}

// 	msg := message.NewMessage(r.ID.String(), r.Payload)
// 	for k, v := range r.Metadata {
// 		msg.Metadata.Set(k, v)
// 	}

// 	return publisher.Publish(r.Topic, msg)
// }

func (o *outboxStore) Add(ctx context.Context, r store.OutboxRecord) error {
	exec := o.db.Executor(ctx)

	meta, _ := json.Marshal(r.Metadata)

	_, err := exec.Exec(ctx, `
        INSERT INTO im_message.messages_outbox (uuid, payload, metadata)
        VALUES ($1, $2, $3)
    `, r.ID, r.Payload, meta)

	return err
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

func (o *outboxStore) Cleanup(ctx context.Context, limit int) (int64, error) {
	cmd, err := o.db.Executor(ctx).Exec(ctx, `
		DELETE FROM im_message.messages_outbox
		WHERE published_at < now() - interval '10 minutes'
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}
