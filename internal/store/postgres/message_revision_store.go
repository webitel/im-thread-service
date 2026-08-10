package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type messageRevisionStore struct {
	db Querier
}

func NewMessageRevisionStore(db Querier) store.MessageRevisionStore {
	return &messageRevisionStore{db: db}
}

func (s *messageRevisionStore) Search(ctx context.Context, messageID uuid.UUID, domainID int32, callerID uuid.UUID) ([]*model.MessageRevision, error) {
	if messageID == uuid.Nil {
		return nil, errors.InvalidArgument("message id cannot be nil", errors.WithID("postgres.message_revision.search"))
	}

	if callerID == uuid.Nil {
		return nil, errors.InvalidArgument("caller id cannot be nil", errors.WithID("postgres.message_revision.search"))
	}

	const visibilityQuery = `
		select 1
		from im_message.messages m
		where m.id = @MessageID
		  and m.domain_id = @DomainID
		  and exists (
			select 1
			from im_thread.thread_dialog td
			where td.thread_id = m.thread_id
			  and td.member_id = @CallerID
			  and td.deleted_at is null
		  )
	`

	visibilityArgs := pgx.NamedArgs{
		"MessageID": messageID,
		"DomainID":  domainID,
		"CallerID":  callerID,
	}

	var visible int

	if err := s.db.QueryRow(ctx, visibilityQuery, visibilityArgs).Scan(&visible); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrMessageNotVisible
		}

		return nil, errors.Internal(
			"executing message visibility query",
			errors.WithCause(err),
			errors.WithID("postgres.message_revision.search.visibility"),
		)
	}

	const query = `
		select r.message_id, r.domain_id, r.revision_no, r.action, r.body, r.changed_by, r.changed_at
		from im_message.message_revisions r
		where r.message_id = @MessageID
		order by r.revision_no
	`

	rows, err := s.db.Query(ctx, query, pgx.NamedArgs{"MessageID": messageID})
	if err != nil {
		return nil, errors.Internal(
			"executing message revisions query",
			errors.WithCause(err),
			errors.WithID("postgres.message_revision.search.query"),
		)
	}

	revisions, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.MessageRevision])
	if err != nil {
		return nil, errors.Internal(
			"collecting message revisions",
			errors.WithCause(err),
			errors.WithID("postgres.message_revision.search.collecting"),
		)
	}

	return revisions, nil
}
