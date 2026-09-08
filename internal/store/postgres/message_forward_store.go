package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

const forwardSourceFields = `m.id, m.thread_id, m.sender_id, m.type, m.body, m.metadata, m.created_at,
	m.documents, m.images, m.location, m.contact, m.interactive`

func (m *messageStore) LoadForwardSources(
	ctx context.Context,
	ids []uuid.UUID,
	callerID uuid.UUID,
	domainID int32,
) (*model.MessageForwardSources, error) {
	if len(ids) == 0 {
		return nil, errors.InvalidArgument("message ids cannot be empty", errors.WithID("postgres.message.load_forward_sources"))
	}

	if callerID == uuid.Nil {
		return nil, errors.InvalidArgument("caller id cannot be nil", errors.WithID("postgres.message.load_forward_sources"))
	}

	query := queryobject.CompactSQL(`
		select ` + forwardSourceFields + `,
			case
				when d.dialog_id is null then @NotFound::smallint
				when m.deleted_at is not null then @AlreadyDeleted::smallint
				when d.dialog_deleted_at is not null then @ChatClosed::smallint
				when m.type = @SystemType then @NotAllowed::smallint
				else @Forwardable::smallint
			end as reason
		from ` + queryobject.MessageHistoryView + ` m
		left join lateral (
			select td.id as dialog_id, td.deleted_at as dialog_deleted_at
			from im_thread.thread_dialog td
			where td.thread_id = m.thread_id
			  and td.member_id = @CallerID
			order by td.deleted_at nulls first
			limit 1
		) d on true
		where m.id = any(@IDs)
		  and m.domain_id = @DomainID
		order by m.created_at, m.id
	`)

	args := pgx.NamedArgs{
		"IDs":            ids,
		"CallerID":       callerID,
		"DomainID":       domainID,
		"SystemType":     int16(model.MessageTypeSystem),
		"Forwardable":    int16(model.MessageSkipUnspecified),
		"NotFound":       int16(model.MessageSkipNotFound),
		"AlreadyDeleted": int16(model.MessageSkipAlreadyDeleted),
		"ChatClosed":     int16(model.MessageSkipChatClosed),
		"NotAllowed":     int16(model.MessageSkipNotAllowed),
	}

	rows, err := m.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"executing load forward sources query",
			errors.WithCause(err),
			errors.WithID("postgres.message.load_forward_sources.query"),
		)
	}

	classified, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, errors.Internal(
			"collecting forward sources",
			errors.WithCause(err),
			errors.WithID("postgres.message.load_forward_sources.collecting"),
		)
	}

	sources, skipped := splitSkipOutcome(ids, classified)

	return &model.MessageForwardSources{Sources: sources, Skipped: skipped}, nil
}

func (m *messageStore) CopyAttachments(ctx context.Context, sourceID, targetID uuid.UUID) error {
	if sourceID == uuid.Nil || targetID == uuid.Nil {
		return errors.InvalidArgument(
			"source and target message ids are required",
			errors.WithID("postgres.message.copy_attachments"),
		)
	}

	const query = `
		with docs as (
			insert into im_message.message_documents (message_id, file_id, name, mime, size)
			select @TargetID, d.file_id, d.name, d.mime, d.size
			from im_message.message_documents d
			where d.message_id = @SourceID
			returning 1
		),
		imgs as (
			insert into im_message.message_images (message_id, file_id, mime, thumbnails, width, height)
			select @TargetID, i.file_id, i.mime, i.thumbnails, i.width, i.height
			from im_message.message_images i
			where i.message_id = @SourceID
			returning 1
		)
		select
			(select count(*) from docs) as documents,
			(select count(*) from imgs) as images
	`

	args := pgx.NamedArgs{
		"SourceID": sourceID,
		"TargetID": targetID,
	}

	if _, err := m.db.Exec(ctx, query, args); err != nil {
		return errors.Internal(
			"copying message attachments",
			errors.WithCause(err),
			errors.WithID("postgres.message.copy_attachments.query"),
		)
	}

	return nil
}
