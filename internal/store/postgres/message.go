package postgres

import (
	"context"
	"fmt"

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

// SaveMessage saves a message to the database.
//
//	Args:
//	- ctx: Context for operation cancellation.
//	- msg: Message to be saved.
//
//	Returns:
//	- *model.Message: Saved message.
//	- error: Error if any occurs.
func (m *messageStore) SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	const query = `
		insert into im_message.messages (
			domain_id, thread_id, sender_id, receiver_id, type, body, metadata
		)
		values (
			@DomainID, @ThreadID, @SenderID, @ReceiverID, @Type, @Body, @Metadata
		)
		returning
			id, thread_id, type, body, metadata, created_at, updated_at,
			jsonb_build_object('id', sender_id) as "from",
			jsonb_build_object('id', receiver_id) as "to"
	`

	args := pgx.NamedArgs{
		"DomainID":   msg.DomainID,
		"ThreadID":   msg.ThreadID,
		"SenderID":   msg.From.ID,
		"ReceiverID": msg.To.ID,
		"Type":       msg.Type,
		"Body":       msg.Text,
		"Metadata":   msg.Metadata,
	}

	rows, err := m.q.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("postgres.save_message: %w", err)
	}

	savedMessage, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, fmt.Errorf("postgres.save_message: %w", err)
	}

	return savedMessage, nil
}

// SaveImages saves message images to the database.
//
// Args:
//
//	ctx: The context of the request.
//	messageID: The ID of the message that the images belong to.
//	images: The images to be saved.
//
// Returns:
//
//	A slice of saved images with their IDs populated.
//	An error if the save operation fails.
func (m *messageStore) SaveImages(ctx context.Context, messageID uuid.UUID, images []*model.MessageImage) ([]*model.MessageImage, error) {
	if len(images) == 0 {
		return nil, nil
	}

	var (
		fileIDs    = make([]int64, len(images))
		mimes      = make([]string, len(images))
		thumbnails = make([]map[string]any, len(images))
		widths     = make([]int32, len(images))
		heights    = make([]int32, len(images))
	)

	for i, img := range images {
		fileIDs[i] = img.FileID
		mimes[i] = img.Mime
		thumbnails[i] = img.Thumbnails
		widths[i] = img.Width
		heights[i] = img.Height
	}

	const query = `
		insert into im_message.message_images (
			message_id, file_id, mime, thumbnails, width, height
		)
		select 
			@MessageID,
			u.file_id,
			u.mime,
			u.thumbnails,
			u.width,
			u.height
		from unnest(
			@FileIDs::int[],
			@Mimes::text[],
			@Thumbnails::jsonb[],
			@Widths::int[],
			@Heights::int[]
		) as u(file_id, mime, thumbnails, width, height)
		returning id, message_id, file_id, mime, thumbnails, width, height, created_at
	`

	args := pgx.NamedArgs{
		"MessageID":  messageID,
		"FileIDs":    fileIDs,
		"Mimes":      mimes,
		"Thumbnails": thumbnails,
		"Widths":     widths,
		"Heights":    heights,
	}

	rows, err := m.q.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	savedImages, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.MessageImage])
	if err != nil {
		return nil, err
	}

	return savedImages, nil
}

// SaveDocuments saves message documents to the database.
//
// Args:
//
//	ctx: The context of the request.
//	messageID: The ID of the message that the documents belong to.
//	docs: The documents to be saved.
//
// Returns:
//
//	A slice of saved documents with their IDs populated.
//	An error if the save operation fails.
func (m *messageStore) SaveDocuments(ctx context.Context, messageID uuid.UUID, docs []*model.MessageDocument) ([]*model.MessageDocument, error) {
	docsLen := len(docs)
	if docsLen == 0 {
		return nil, nil
	}

	var (
		fileIDs   = make([]int, docsLen)
		fileNames = make([]string, docsLen)
		mimes     = make([]string, docsLen)
		sizes     = make([]int, docsLen)
	)

	for i, doc := range docs {
		fileIDs[i] = int(doc.FileID)
		fileNames[i] = doc.Name
		mimes[i] = doc.Mime
		sizes[i] = int(doc.Size)
	}

	const query = `
		insert into im_message.message_documents (
			message_id, file_id, name, mime, size
		)
		select
			@MessageID,
			u.file_id,
			u.name,
			u.mime,
			u.size
		from unnest(
			@FileIDs::int[],
			@FileNames::text[],
			@Mimes::text[],
			@Sizes::int[]
		) as u(file_id, name, mime, size)
		returning id, message_id, file_id, name, mime, size, created_at
	`

	args := pgx.NamedArgs{
		"MessageID": messageID,
		"FileIDs":   fileIDs,
		"FileNames": fileNames,
		"Mimes":     mimes,
		"Sizes":     sizes,
	}

	rows, err := m.q.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	savedDocuments, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.MessageDocument])
	if err != nil {
		return nil, err
	}

	return savedDocuments, nil
}
