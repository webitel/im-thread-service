package postgres

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/infra/db/pg"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type messageStore struct {
	db *pg.PgxDB
}

func NewMessageStore(db *pg.PgxDB) store.MessageStore {
	return &messageStore{
		db: db,
	}
}

// Interface guard
var (
	_ store.MessageStore = (*messageStore)(nil)
)

func (m *messageStore) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.WithTx(ctx, fn)
}

func (m *messageStore) SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	args := pgx.NamedArgs{
		"thread_id": msg.ThreadId, "sender_id": msg.From.Id,
		"receiver_id": msg.To.Id, "body": msg.Text, "type": msg.Type,
	}

	const query = `
        INSERT INTO im_message.messages (thread_id, sender_id, receiver_id, body, type)
        VALUES (@thread_id, @sender_id, @receiver_id, @body, @type)
        RETURNING 
            id, thread_id, body, type, metadata, created_at, updated_at,
            sender_id   AS "from.id", 
            receiver_id AS "to.id"`

	var saved model.Message
	if err := pgxscan.Get(ctx, m.db.Executor(ctx), &saved, query, args); err != nil {
		return nil, fmt.Errorf("save_message: %w", err)
	}
	return &saved, nil
}
