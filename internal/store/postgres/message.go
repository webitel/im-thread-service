package postgres

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type messageStore struct {
	// [QUERIER]
	// Supports both standalone connection pool and active transaction (Unit of Work)
	q Querier
}

func NewMessageStore(q Querier) store.MessageStore {
	return &messageStore{q: q}
}

// [MESSAGE] Persistence implementation
// SAVES the core message entity including sender, receiver, and text content.
func (m *messageStore) SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	args := pgx.NamedArgs{
		"thread_id":   msg.ThreadID,
		"sender_id":   msg.From.ID,
		"receiver_id": msg.To.ID,
		"type":        msg.Type,
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
	if err := pgxscan.Get(ctx, m.q, &saved, query, args); err != nil {
		return nil, fmt.Errorf("postgres.save_message: %w", err)
	}

	return &saved, nil
}

// [IMAGE] Attachments implementation
// PERFORMS bulk-like insertion for image metadata associated with a specific MESSAGE_ID.
func (m *messageStore) SaveImages(ctx context.Context, messageID uuid.UUID, images []*model.MessageImage) ([]*model.MessageImage, error) {
	if len(images) == 0 {
		return nil, nil
	}

	savedImages := make([]*model.MessageImage, 0, len(images))
	for _, img := range images {
		args := pgx.NamedArgs{
			"message_id": messageID,
			"file_id":    img.FileID,
			"mime":       img.Mime,
			"thumbnails": img.Thumbnails,
			"width":      img.Width,
			"height":     img.Height,
		}

		const query = `
            INSERT INTO im_message.message_images (message_id, file_id, mime, thumbnails, width, height)
            VALUES (@message_id, @file_id, @mime, @thumbnails, @width, @height)
            RETURNING id, message_id, file_id, mime, thumbnails, width, height, created_at`

		var saved model.MessageImage
		if err := pgxscan.Get(ctx, m.q, &saved, query, args); err != nil {
			return nil, fmt.Errorf("postgres.save_images: %w", err)
		}
		savedImages = append(savedImages, &saved)
	}

	return savedImages, nil
}

// [DOCUMENT] Attachments implementation
// PERFORMS bulk-like insertion for document metadata associated with a specific MESSAGE_ID.
func (m *messageStore) SaveDocuments(ctx context.Context, messageID uuid.UUID, docs []*model.MessageDocument) ([]*model.MessageDocument, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	savedDocs := make([]*model.MessageDocument, 0, len(docs))
	for _, doc := range docs {
		args := pgx.NamedArgs{
			"message_id": messageID,
			"file_id":    doc.FileID,
			"name":       doc.Name,
			"mime":       doc.Mime,
			"size":       doc.Size,
		}

		const query = `
            INSERT INTO im_message.message_documents (message_id, file_id, name, mime, size)
            VALUES (@message_id, @file_id, @name, @mime, @size)
            RETURNING id, message_id, file_id, name, mime, size, created_at`

		var saved model.MessageDocument
		if err := pgxscan.Get(ctx, m.q, &saved, query, args); err != nil {
			return nil, fmt.Errorf("postgres.save_documents: %w", err)
		}
		savedDocs = append(savedDocs, &saved)
	}

	return savedDocs, nil
}
