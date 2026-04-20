package postgres

import (
	"context"

	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type messageStore struct {
	db Querier
}

func NewMessageStore(q Querier) store.MessageStore {
	return &messageStore{db: q}
}

func (m *messageStore) SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	const query = `
		insert into im_message.messages (
			domain_id, thread_id, sender_id, type, body, metadata
		)
		values (
			@DomainID, @ThreadID, @SenderID, @Type, @Body, @Metadata
		)
		returning
			id, domain_id, thread_id, type, body, metadata, created_at, updated_at,
			jsonb_build_object('id', sender_id) as "from"
	`

	args := pgx.NamedArgs{
		"DomainID": msg.DomainID,
		"ThreadID": msg.ThreadID,
		"SenderID": msg.From.ID,
		"Type":     msg.Type,
		"Body":     msg.Body,
		"Metadata": msg.Metadata,
	}

	rows, err := m.db.Query(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("postgres.save_message: %w", err)
	}

	savedMessage, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, fmt.Errorf("postgres.save_message: %w", err)
	}

	return savedMessage, nil
}

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

	rows, err := m.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	savedImages, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.MessageImage])
	if err != nil {
		return nil, err
	}

	return savedImages, nil
}

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

	rows, err := m.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	savedDocuments, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.MessageDocument])
	if err != nil {
		return nil, err
	}

	return savedDocuments, nil
}

func (m *messageStore) ReadMessage(ctx context.Context, read struct {
	DomainID  int32
	ThreadID  uuid.UUID
	MessageID uuid.UUID
	UserID    uuid.UUID
},
) error {
	const query = `
		INSERT INTO im_message.message_reads (domain_id, thread_id, message_id, user_id)
		VALUES (@DomainID, @ThreadID, @MessageID, @UserID)
		ON CONFLICT (message_id, user_id) DO NOTHING;
	`

	args := pgx.NamedArgs{
		"DomainID":  read.DomainID,
		"MessageID": read.MessageID,
		"ThreadID":  read.ThreadID,
		"UserID":    read.UserID,
	}

	_, err := m.db.Exec(ctx, query, args)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			// 23503 is the PostgreSQL code for foreign_key_violation
			if pgErr.Code == "23503" {
				return fmt.Errorf("read_message: message or thread not found: %w", err)
			}
		}
		return fmt.Errorf("read_message: failed to execute insert: %w", err)
	}

	return nil
}

func (m *messageStore) SaveMessageLocation(ctx context.Context, msg *model.Message) (*model.Message, error) {
	sql, args := prepareSaveMessageLocationQuery(msg)

	rows, err := m.db.Query(ctx, sql, args)
	if err != nil {
		return nil, errors.Internal("eror querying insert location", errors.WithCause(err))
	}

	saved, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, errors.Internal("eror collecting saved location", errors.WithCause(err))
	}

	return saved, nil
}

func prepareSaveMessageLocationQuery(msg *model.Message) (string, pgx.NamedArgs) {
	query := `
		with msg_ins as (
			insert into im_message.messages
			(thread_id, domain_id, sender_id, type, body, metadata)
			values (@ThreadID, @DomainID, @SenderID, @Type, @Body, @Metadata)
			returning *
		),
		location_ins as (
			insert into im_message.message_locations
			(message_id, address, latitude, longitude, name)
			values ((select id from msg_ins), @Address, @Latitude, @Longitude, @Name)
			returning *
		)
		select
			m.id,
			m.thread_id,
			m.domain_id,
			m.sender_id,
			m.type,
			m.body,
			m.metadata,
			m.created_at,
			m.updated_at,
			l.location as location
		from msg_ins m
		left join lateral (
			select to_jsonb(l) as location from location_ins l
		) l on true;
	`

	args := pgx.NamedArgs{
		"ThreadID":  msg.ThreadID,
		"DomainID":  msg.DomainID,
		"SenderID":  msg.From.ID,
		"Type":      msg.Type,
		"Body":      msg.Body,
		"Metadata":  msg.Metadata,
		"Address":   msg.Location.Address,
		"Latitude":  msg.Location.Latitude,
		"Longitude": msg.Location.Longitude,
		"Name":      msg.Location.Name,
	}

	return queryobject.CompactSQL(query), args
}

func (m *messageStore) SaveMessageContact(ctx context.Context, msg *model.Message) (*model.Message, error) {
	query, args := prepareSaveMessageContactQuery(msg)

	rows, err := m.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal("eror querying insert contact", errors.WithCause(err))
	}

	saved, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, errors.Internal("eror collecting saved contact", errors.WithCause(err))
	}

	return saved, nil
}

func prepareSaveMessageContactQuery(msg *model.Message) (string, pgx.NamedArgs) {
	query := `
		with msg_ins as (
			insert into im_message.messages
			(thread_id, domain_id, sender_id, type, body, metadata)
			values (@ThreadID, @DomainID, @SenderID, @Type, @Body, @Metadata)
			returning *
		),
		contact_ins as (
			insert into im_message.message_contacts
			(message_id, phone_number, name, email)
			values ((select id from msg_ins), @Phone, @Name, @Email)
			returning *
		)
		select
			m.id as id,
			m.thread_id as thread_id,
			m.domain_id as domain_id,
			m.sender_id as sender_id,
			m.type as type,
			m.body as body,
			m.metadata as metadata,
			m.created_at as created_at,
			m.updated_at as updated_at,
			c.contact as contact
		from msg_ins m
		left join lateral (
			select to_jsonb(c) as contact from contact_ins c
		) c on true;
	`
	args := pgx.NamedArgs{
		"ThreadID": msg.ThreadID,
		"DomainID": msg.DomainID,
		"SenderID": msg.From.ID,
		"Type":     msg.Type,
		"Body":     msg.Body,
		"Metadata": msg.Metadata,
		"Phone":    msg.Contact.PhoneNumber,
		"Name":     msg.Contact.Name,
		"Email":    msg.Contact.Email,
	}

	return queryobject.CompactSQL(query), args
}

func (m *messageStore) SaveSystemMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	sql, args := prepareSaveSystemMessageQuery(msg)

	rows, err := m.db.Query(ctx, sql, args)
	if err != nil {
		return nil, errors.Internal("error querying insert system message", errors.WithCause(err))
	}

	saved, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, errors.Internal("error collecting saved system message", errors.WithCause(err))
	}

	return saved, nil
}

func prepareSaveSystemMessageQuery(msg *model.Message) (string, pgx.NamedArgs) {
	query := `
		with msg_ins as (
			insert into im_message.messages
			(thread_id, domain_id, sender_id, type, body, metadata)
			values (@ThreadID, @DomainID, @SenderID, @Type, @Body, @Metadata)
			returning *
		),
		sys_ins as (
			insert into im_message.system_messages (message_id, type, metadata)
			values ((select id from msg_ins), @SystemType, @SystemMetadata)
			returning *
		)
		select
			m.id,
			m.thread_id,
			m.domain_id,
			m.sender_id,
			m.type,
			m.body,
			m.metadata,
			m.created_at,
			m.updated_at,
			sys.system as system
		from msg_ins m
		left join lateral (
			select to_jsonb(s) as system from sys_ins s
		) sys on true
	`

	args := pgx.NamedArgs{
		"ThreadID":       msg.ThreadID,
		"DomainID":       msg.DomainID,
		"SenderID":       msg.From.ID,
		"Type":           msg.Type,
		"Body":           msg.Body,
		"Metadata":       msg.Metadata,
		"SystemType":     msg.System.Type,
		"SystemMetadata": msg.System.Metadata,
	}

	return queryobject.CompactSQL(query), args
}

func (m *messageStore) SaveInteractiveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	sql, args := prepareSaveInteractiveMessageQuery(msg)

	rows, err := m.db.Query(ctx, sql, args)
	if err != nil {
		return nil, errors.Internal("eror querying insert interactive message", errors.WithCause(err))
	}

	saved, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, errors.Internal("eror collecting saved interactive message", errors.WithCause(err))
	}

	return saved, nil
}

func prepareSaveInteractiveMessageQuery(msg *model.Message) (string, pgx.NamedArgs) {
	query := `
		with msg_ins as (
			insert into im_message.messages
			(thread_id, domain_id, sender_id, type, body, metadata, interactive)
			values (@ThreadID, @DomainID, @SenderID, @Type, @Body, @Metadata, @Interactive)
			returning *
		),
		documents_ins as (
			insert into im_message.message_documents
			(message_id, file_id, mime, name, size)
			select
				msg.id,
				u.file_id,
				u.mime,
				u.file_name,
				u.file_size
			from  unnest (
				@DocumentsFileIDs::int[],
				@DocumentsMimes::text[],
				@DocumentsFileNames::text[],
				@DocumentsFileSizes::int[]
			) as u(file_id, mime, file_name, file_size)
			cross join msg_ins msg
			returning *
		),
		images_ins as (
			insert into im_message.message_images
			(message_id, file_id, mime, thumbnails, width, height)
			select
				msg.id,
				u.file_id,
				u.mime,
				u.thumbnails,
				u.width,
				u.height
			from unnest (
				@ImagesFileIDs::int[],
				@ImagesMimes::text[],
				@ImagesThumbnails::jsonb[],
				@ImagesWidths::int[],
				@ImagesHeights::int[]
			) as u(file_id, mime, thumbnails, width, height)
			cross join msg_ins msg
			returning *
		)
		select
			m.id as id,
			m.thread_id as thread_id,
			m.domain_id as domain_id,
			m.sender_id as sender_id,
			m.type as type,
			m.body as body,
			m.metadata as metadata,
			m.interactive as interactive,
			m.created_at as created_at,
			m.updated_at as updated_at,
			doc_ins.documents as documents,
			img_ins.images as images
		from msg_ins m
		left join lateral (
			select jsonb_agg(to_jsonb(doc)) as documents
			from documents_ins doc
			where doc.message_id = m.id
		) as doc_ins on true
		left join lateral (
			select jsonb_agg(to_jsonb(img)) as images
			from images_ins img
			where img.message_id = m.id
		) as img_ins on true;
	`

	var (
		documentsFileIDs []int
		documentsMimes   []string
		documentsNames   []string
		documentsSizes   []int
		imagesFileIDs    []int
		imagesMimes      []string
		imagesThumbnails []map[string]any
		imagesWidths     []int
		imagesHeights    []int
	)

	if msg.Documents != nil {
		for _, doc := range msg.Documents {
			documentsFileIDs = append(documentsFileIDs, int(doc.FileID))
			documentsMimes = append(documentsMimes, doc.Mime)
			documentsNames = append(documentsNames, doc.Name)
			documentsSizes = append(documentsSizes, int(doc.Size))
		}
	}
	if msg.Images != nil {
		for _, img := range msg.Images {
			imagesFileIDs = append(imagesFileIDs, int(img.FileID))
			imagesMimes = append(imagesMimes, img.Mime)
			imagesThumbnails = append(imagesThumbnails, img.Thumbnails)
			imagesWidths = append(imagesWidths, int(img.Width))
			imagesHeights = append(imagesHeights, int(img.Height))
		}
	}

	args := pgx.NamedArgs{
		"ThreadID":           msg.ThreadID,
		"DomainID":           msg.DomainID,
		"SenderID":           msg.From.ID,
		"Type":               msg.Type,
		"Body":               msg.Body,
		"Metadata":           msg.Metadata,
		"Interactive":        msg.Interactive,
		"DocumentsFileIDs":   documentsFileIDs,
		"DocumentsMimes":     documentsMimes,
		"DocumentsFileNames": documentsNames,
		"DocumentsFileSizes": documentsSizes,
		"ImagesFileIDs":      imagesFileIDs,
		"ImagesMimes":        imagesMimes,
		"ImagesThumbnails":   imagesThumbnails,
		"ImagesWidths":       imagesWidths,
		"ImagesHeights":      imagesHeights,
	}

	return queryobject.CompactSQL(query), args
}
