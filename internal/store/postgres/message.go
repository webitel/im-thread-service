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
	// [QUERIER]
	// Supports both pgxpool (standalone) and pgx.Tx (within UoW)
	q Querier
}

func NewMessageStore(q Querier) store.MessageStore {
	return &messageStore{
		q: q,
	}
}

var _ store.MessageStore = (*messageStore)(nil)

func (m *messageStore) SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	// 1. Prepare arguments for the master record (im_message.messages)
	// According to schema: thread_id, sender_id, receiver_id, type, body, metadata
	args := pgx.NamedArgs{
		"thread_id":   msg.ThreadID,
		"sender_id":   msg.From.ID,
		"receiver_id": msg.To.ID,
		"type":        msg.Type, // SMALLINT (MessageType enum)
		"body":        msg.Text,
		"metadata":    msg.Metadata,
	}

	const query = `
        INSERT INTO im_message.messages (thread_id, sender_id, receiver_id, type, body, metadata)
        VALUES (@thread_id, @sender_id, @receiver_id, @type, @body, @metadata)
        RETURNING 
            id, thread_id, type, body, metadata, created_at, updated_at,
            sender_id   AS "from.id", 
            receiver_id AS "to.id"`

	var saved model.Message

	// 2. Execute main message insertion
	// Since we use Unit of Work, m.q is the active transaction
	if err := pgxscan.Get(ctx, m.q, &saved, query, args); err != nil {
		return nil, fmt.Errorf("save_message.messages: %w", err)
	}

	// 3. Handle image attachments (detail records)
	if len(msg.Images) > 0 {
		saved.Images = make([]*model.MessageImage, 0, len(msg.Images))

		for _, img := range msg.Images {
			// Link the attachment to the newly generated message ID
			imgArgs := pgx.NamedArgs{
				"message_id": saved.ID,
				"file_id":    img.FileID,
				"name":       img.Name,
				"mime":       img.Mime,
				"thumbnails": img.Thumbnails,
				"width":      img.Width,
				"height":     img.Height,
			}

			const imgQuery = `
                INSERT INTO im_message.message_images (message_id, file_id, name, mime, thumbnails, width, height)
                VALUES (@message_id, @file_id, @name, @mime, @thumbnails, @width, @height)
                RETURNING id, message_id, file_id, name, mime, thumbnails, width, height, created_at`

			var savedImg model.MessageImage
			if err := pgxscan.Get(ctx, m.q, &savedImg, imgQuery, imgArgs); err != nil {
				return nil, fmt.Errorf("save_message.message_images: %w", err)
			}

			saved.Images = append(saved.Images, &savedImg)
		}
	}

	return &saved, nil
}
