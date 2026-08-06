package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

const forwardSourceFields = `id, thread_id, sender_id, type, body, metadata, created_at,
	documents, images, location, contact, interactive`

func (m *messageStore) LoadForwardSources(
	ctx context.Context,
	ids []uuid.UUID,
	callerID uuid.UUID,
	domainID int32,
) ([]*model.Message, error) {
	if len(ids) == 0 {
		return nil, errors.InvalidArgument("message ids cannot be empty", errors.WithID("postgres.message.load_forward_sources"))
	}

	if callerID == uuid.Nil {
		return nil, errors.InvalidArgument("caller id cannot be nil", errors.WithID("postgres.message.load_forward_sources"))
	}

	query := queryobject.CompactSQL(`
		select ` + forwardSourceFields + `
		from ` + queryobject.MessageHistoryView + ` m
		where m.id = any(@IDs)
		  and m.domain_id = @DomainID
		  and m.deleted_at is null
		  and m.type <> 4
		  and exists (
			select 1
			from im_thread.thread_dialog td
			where td.thread_id = m.thread_id
			  and td.member_id = @CallerID
			  and td.deleted_at is null
		  )
		order by m.created_at, m.id
	`)

	args := pgx.NamedArgs{
		"IDs":      ids,
		"CallerID": callerID,
		"DomainID": domainID,
	}

	rows, err := m.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"executing load forward sources query",
			errors.WithCause(err),
			errors.WithID("postgres.message.load_forward_sources.query"),
		)
	}

	sources, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.Message])
	if err != nil {
		return nil, errors.Internal(
			"collecting forward sources",
			errors.WithCause(err),
			errors.WithID("postgres.message.load_forward_sources.collecting"),
		)
	}

	return sources, nil
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
