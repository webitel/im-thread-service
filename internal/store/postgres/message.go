package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type messageStore struct {
	db Querier
}

func NewMessageStore(q Querier) store.MessageStore {
	return &messageStore{db: q}
}

func validateMessageForSave(msg *model.Message, operationID string) error {
	if msg == nil {
		return errors.InvalidArgument("message cannot be nil", errors.WithID(operationID))
	}

	if msg.ThreadID == uuid.Nil {
		return errors.InvalidArgument("message thread id cannot be nil", errors.WithID(operationID))
	}

	if msg.DomainID <= 0 {
		return errors.InvalidArgument("message domain id must be greater than zero", errors.WithID(operationID))
	}

	if msg.Type == model.MessageTypeSystem {
		return nil
	}

	if msg.Member == nil {
		return errors.InvalidArgument("message member cannot be nil", errors.WithID(operationID))
	}

	if msg.Member.ID == uuid.Nil {
		return errors.InvalidArgument("message member id cannot be nil", errors.WithID(operationID))
	}

	return nil
}

func replyToArg(msg *model.Message) *uuid.UUID {
	if msg.ReplyTo == nil {
		return nil
	}

	id := msg.ReplyTo.MessageID

	return &id
}

func (m *messageStore) SaveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	if err := validateMessageForSave(msg, "postgres.message.save_message"); err != nil {
		return nil, err
	}

	const query = `
		insert into im_message.messages (
			domain_id, thread_id, sender_id, member_id, type, body, metadata, origin_sender, reply_to
		)
		values (
			@DomainID, @ThreadID, @SenderID, @MemberID, @Type, @Body, @Metadata, @OriginSender, @ReplyTo
		)
		returning
			id, domain_id, thread_id, member_id, type, body, metadata, created_at, updated_at,
			jsonb_build_object('id', sender_id) as "from"
	`

	args := pgx.NamedArgs{
		"DomainID":     msg.DomainID,
		"ThreadID":     msg.ThreadID,
		"SenderID":     msg.GetSender(),
		"MemberID":     msg.Member.ID,
		"Type":         msg.Type,
		"Body":         msg.Body,
		"Metadata":     msg.Metadata,
		"OriginSender": msg.GetOriginSender(),
		"ReplyTo":      replyToArg(msg),
	}

	rows, err := m.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal("executing save message query", errors.WithCause(err), errors.WithID("postgres.message.save.query"))
	}

	savedMessage, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, errors.Internal("collecting saved message", errors.WithCause(err), errors.WithID("postgres.message.save.collecting"))
	}

	return savedMessage, nil
}

func (m *messageStore) GetReplyPreview(ctx context.Context, id uuid.UUID, domainID int32) (*model.ReplyToPreview, error) {
	const query = `
		select
			m.id as message_id,
			m.thread_id,
			m.sender_id,
			m.type,
			left(coalesce(m.body, ''), 256) as body,
			(extract(epoch from m.created_at) * 1000)::bigint as created_at,
			coalesce(
				(select jsonb_build_object('kind', 'document', 'name', d.name, 'mime', d.mime)
					from im_message.message_documents d
					where d.message_id = m.id
					order by d.created_at
					limit 1),
				(select jsonb_build_object('kind', 'image', 'mime', i.mime)
					from im_message.message_images i
					where i.message_id = m.id
					order by i.created_at
					limit 1)
			) as attachment
		from im_message.messages m
		where m.id = @ID and m.domain_id = @DomainID
	`

	args := pgx.NamedArgs{
		"ID":       id,
		"DomainID": domainID,
	}

	rows, err := m.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal("executing reply preview query", errors.WithCause(err), errors.WithID("postgres.message.reply_preview.query"))
	}

	preview, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.ReplyToPreview])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, errors.Internal("collecting reply preview", errors.WithCause(err), errors.WithID("postgres.message.reply_preview.collecting"))
	}

	return preview, nil
}

func (m *messageStore) EditMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	if msg == nil {
		return nil, errors.InvalidArgument("message cannot be nil", errors.WithID("postgres.message.edit_message"))
	}

	if msg.ID == uuid.Nil {
		return nil, errors.InvalidArgument("message id cannot be nil", errors.WithID("postgres.message.edit_message"))
	}

	if msg.From.ID == uuid.Nil {
		return nil, errors.InvalidArgument("editor id cannot be nil", errors.WithID("postgres.message.edit_message"))
	}

	const query = `
		update im_message.messages m
		set body = @Body,
		    edited = true,
		    updated_at = now()
		where m.id = @ID
		  and (m.sender_id = @EditorID or m.origin_sender = @EditorID)
		  and exists (
			select 1
			from im_thread.thread_dialog td
			where td.thread_id = m.thread_id
			  and td.member_id = @EditorID
			  and td.deleted_at is null
		  )
		returning
			id, domain_id, thread_id, member_id, type, body, metadata,
			created_at, updated_at, edited
	`

	args := pgx.NamedArgs{
		"ID":       msg.ID,
		"EditorID": msg.From.ID,
		"Body":     msg.Body,
	}

	rows, err := m.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal("executing edit message query", errors.WithCause(err), errors.WithID("postgres.message.edit.query"))
	}

	edited, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.Forbidden(
				"message cannot be edited: not found, not authored by the editor, or the chat is closed",
				errors.WithCause(err),
				errors.WithID("postgres.message.edit.not_allowed"),
			)
		}

		return nil, errors.Internal("collecting edited message", errors.WithCause(err), errors.WithID("postgres.message.edit.collecting"))
	}

	return edited, nil
}

func (m *messageStore) SaveDocuments(ctx context.Context, messageID uuid.UUID, docs []*model.MessageDocument) ([]*model.MessageDocument, error) {
	docsLen := len(docs)
	if docsLen == 0 {
		return nil, nil
	}

	var (
		fileIDs   = make([]int64, docsLen)
		fileNames = make([]string, docsLen)
		mimes     = make([]string, docsLen)
		sizes     = make([]int64, docsLen)
	)

	for i, doc := range docs {
		fileIDs[i] = doc.FileID
		fileNames[i] = doc.Name
		mimes[i] = doc.Mime
		sizes[i] = doc.Size
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
	if err := validateMessageForSave(msg, "postgres.message.save_message_location"); err != nil {
		return nil, err
	}

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
			(thread_id, domain_id, sender_id, member_id, type, body, metadata, reply_to)
			values (@ThreadID, @DomainID, @SenderID, @MemberID, @Type, @Body, @Metadata, @ReplyTo)
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
			jsonb_build_object('id', m.sender_id) as "from",
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
		"MemberID":  msg.Member.ID,
		"Type":      msg.Type,
		"Body":      msg.Body,
		"Metadata":  msg.Metadata,
		"ReplyTo":   replyToArg(msg),
		"Address":   msg.Location.Address,
		"Latitude":  msg.Location.Latitude,
		"Longitude": msg.Location.Longitude,
		"Name":      msg.Location.Name,
	}

	return queryobject.CompactSQL(query), args
}

func (m *messageStore) SaveMessageContact(ctx context.Context, msg *model.Message) (*model.Message, error) {
	if err := validateMessageForSave(msg, "postgres.message.save_message_contact"); err != nil {
		return nil, err
	}

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
			(thread_id, domain_id, sender_id, member_id, type, body, metadata, reply_to)
			values (@ThreadID, @DomainID, @SenderID, @MemberID, @Type, @Body, @Metadata, @ReplyTo)
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
			m.member_id as member_id,
			jsonb_build_object('id', m.sender_id) as "from",
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
		"MemberID": msg.Member.ID,
		"Type":     msg.Type,
		"Body":     msg.Body,
		"Metadata": msg.Metadata,
		"ReplyTo":  replyToArg(msg),
		"Phone":    msg.Contact.PhoneNumber,
		"Name":     msg.Contact.Name,
		"Email":    msg.Contact.Email,
	}

	return queryobject.CompactSQL(query), args
}

func (m *messageStore) SaveSystemMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	if err := validateMessageForSave(msg, "postgres.message.save_system_message"); err != nil {
		return nil, err
	}

	sql, args := prepareSaveSystemMessageQuery(msg)

	rows, err := m.db.Query(ctx, sql, args)
	if err != nil {
		return nil, errors.Internal(
			"querying insert system message",
			errors.WithCause(err),
			errors.WithID("postgres.message.save_system_message"),
		)
	}

	saved, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, errors.Internal(
			"collecting saved system message",
			errors.WithCause(err),
			errors.WithID("postgres.message.save_system_message"),
		)
	}

	return saved, nil
}

func prepareSaveSystemMessageQuery(msg *model.Message) (string, pgx.NamedArgs) {
	memberID := uuid.Nil
	if msg.Member != nil {
		memberID = msg.Member.ID
	}

	query := `
		with msg_ins as (
			insert into im_message.messages
			(thread_id, domain_id, sender_id, member_id, type, body, metadata)
			values (@ThreadID, @DomainID, @SenderID, @MemberID, @Type, @Body, @Metadata)
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
			m.member_id,
			jsonb_build_object('id', m.sender_id) as "from",
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
		"MemberID":       memberID,
		"Type":           msg.Type,
		"Body":           msg.Body,
		"Metadata":       msg.Metadata,
		"SystemType":     msg.System.Type,
		"SystemMetadata": msg.System.Metadata,
	}

	return queryobject.CompactSQL(query), args
}

func (m *messageStore) SaveInteractiveMessage(ctx context.Context, msg *model.Message) (*model.Message, error) {
	if err := validateMessageForSave(msg, "postgres.message.save_interactive_message"); err != nil {
		return nil, err
	}

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
			(thread_id, domain_id, sender_id, member_id, type, body, metadata, interactive)
			values (@ThreadID, @DomainID, @SenderID, @MemberID, @Type, @Body, @Metadata, @Interactive)
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
			m.member_id as member_id,
			jsonb_build_object('id', m.sender_id) as "from",
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
		"MemberID":           msg.Member.ID,
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
