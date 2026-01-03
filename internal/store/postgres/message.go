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
		"thread_id": msg.ThreadId,
		"from_id":   msg.From.Id,
		"to_id":     msg.To.Id,
		"body":      msg.Text,
		"metadata":  msg.Metadata,
	}

	query := `
        INSERT INTO im_message.messages (thread_id, from_id, to_id, body, metadata)
        VALUES (@thread_id, @from_id, @to_id, @body, @metadata)
        RETURNING *
    `

	var saved model.Message
	if err := pgxscan.Get(ctx, m.db.Executor(ctx), &saved, query, args); err != nil {
		return nil, fmt.Errorf("failed to save message: %w", err)
	}
	return &saved, nil
}
