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

// Search replays a message's history, oldest first.
//
// Only overwritten bodies are stored, so the log is assembled from three
// sources: one entry per stored revision, one for the body still live, and one
// for the deletion. A revision holds the body a version carried and the stamp
// of the edit that replaced it, hence the lag(): a version's text comes from
// its own row while its stamp comes from the previous one.
//
// A visible message always yields at least the live entry, so an empty result
// means the caller may not see the message at all.
func (s *messageRevisionStore) Search(ctx context.Context, messageID uuid.UUID, domainID int32, callerID uuid.UUID) ([]*model.MessageChangeEntry, error) {
	if messageID == uuid.Nil {
		return nil, errors.InvalidArgument("message id cannot be nil", errors.WithID("postgres.message_revision.search"))
	}

	if callerID == uuid.Nil {
		return nil, errors.InvalidArgument("caller id cannot be nil", errors.WithID("postgres.message_revision.search"))
	}

	const query = `
		with msg as (
			select
				m.id,
				m.thread_id,
				coalesce(m.origin_sender, m.sender_id) as created_by,
				m.created_at,
				m.updated_at,
				coalesce(m.body, '') as body,
				m.edited,
				m.deleted_by,
				m.deleted_at
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
		),
		rev as (
			select
				r.version,
				(row_number() over w)::int as position,
				r.body,
				r.changed_by,
				r.changed_at,
				lag(r.changed_by) over w as prev_changed_by,
				lag(r.changed_at) over w as prev_changed_at
			from im_message.message_revisions r
			where r.message_id = (select id from msg)
			window w as (order by r.version)
		),
		tail as (
			select
				count(*)::int as n,
				(array_agg(changed_by order by version desc))[1] as last_changed_by,
				(array_agg(changed_at order by version desc))[1] as last_changed_at
			from rev
		),
		entries as (
			select
				r.position as version,
				case when r.position = 1 then @ActionCreated::smallint else @ActionEdited::smallint end as action,
				r.body,
				coalesce(r.prev_changed_by, m.created_by) as changed_by,
				coalesce(r.prev_changed_at, m.created_at) as changed_at
			from rev r
			cross join msg m

			union all

			select
				t.n + 1,
				case when t.n > 0 or m.edited then @ActionEdited::smallint else @ActionCreated::smallint end,
				m.body,
				coalesce(t.last_changed_by, m.created_by),
				case
					when t.n > 0 then t.last_changed_at
					when m.edited then m.updated_at
					else m.created_at
				end
			from msg m
			cross join tail t

			union all

			select
				t.n + 2,
				@ActionDeleted::smallint,
				m.body,
				coalesce(m.deleted_by, m.created_by),
				m.deleted_at
			from msg m
			cross join tail t
			where m.deleted_at is not null
		)
		select
			e.version,
			e.action,
			e.body,
			e.changed_at,
			jsonb_build_object('id', td.id, 'member_id', e.changed_by, 'member_role', coalesce(td.thread_role, 0)) as changed_by
		from entries e
		cross join msg m
		left join lateral (
			select td.id, td.thread_role
			from im_thread.thread_dialog td
			where td.thread_id = m.thread_id
			  and td.member_id = e.changed_by
			order by td.id desc
			limit 1
		) td on true
		order by e.version
	`

	args := pgx.NamedArgs{
		"MessageID":     messageID,
		"DomainID":      domainID,
		"CallerID":      callerID,
		"ActionCreated": int16(model.MessageRevisionActionCreated),
		"ActionEdited":  int16(model.MessageRevisionActionEdited),
		"ActionDeleted": int16(model.MessageRevisionActionDeleted),
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"executing message change log query",
			errors.WithCause(err),
			errors.WithID("postgres.message_revision.search.query"),
		)
	}

	entries, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.MessageChangeEntry])
	if err != nil {
		return nil, errors.Internal(
			"collecting message change log",
			errors.WithCause(err),
			errors.WithID("postgres.message_revision.search.collecting"),
		)
	}

	if len(entries) == 0 {
		return nil, store.ErrMessageNotVisible
	}

	return entries, nil
}
