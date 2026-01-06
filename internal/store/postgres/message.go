package postgres

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type messageStore struct {
	// q is a querier that can be either a connection pool or a transaction
	q Querier
}

// NewMessageStore returns a new message store, given a querier.
// The querier is used to execute queries against the database.
func NewMessageStore(q Querier) store.MessageStore {
	return &messageStore{
		q: q,
	}
}

// Interface guard
var (
	_ store.MessageStore = (*messageStore)(nil)
)

// SaveMessage inserts a new message into the database.
// It uses pgxscan to map the returning row into the Message model.
func (m *messageStore) SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	args := pgx.NamedArgs{
		"thread_id":   msg.ThreadId,
		"sender_id":   msg.From.Id,
		"receiver_id": msg.To.Id,
		"body":        msg.Text,
		"type":        msg.Type,
	}

	const query = `
        INSERT INTO im_message.messages (thread_id, sender_id, receiver_id, body, type)
        VALUES (@thread_id, @sender_id, @receiver_id, @body, @type)
        RETURNING 
            id, thread_id, body, type, metadata, created_at, updated_at,
            sender_id   AS "from.id", 
            receiver_id AS "to.id"`

	var saved model.Message
	// Use the querier m.q instead of db.Executor(ctx)
	if err := pgxscan.Get(ctx, m.q, &saved, query, args); err != nil {
		return nil, fmt.Errorf("save_message: %w", err)
	}
	return &saved, nil
}
