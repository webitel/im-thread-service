package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type messageReactionStore struct {
	db Querier
}

func NewMessageReactionStore(q Querier) store.MessageReactionStore {
	return &messageReactionStore{db: q}
}

// reactionOutcome mirrors the single row the SetReaction statement returns.
type reactionOutcome struct {
	Allowed   bool       `db:"allowed"`
	ThreadID  *uuid.UUID `db:"thread_id"` // resolved from the message; nil when not allowed
	Action    string     `db:"action"`
	Emoji     string     `db:"emoji"`
	ReactedAt int64      `db:"reacted_at"` // epoch millis, 0 when nothing is stored
}

// setReactionSQL applies declarative set/replace/remove semantics for one
// (message, reactor) pair in a single round-trip. It is idempotent by
// construction: the request carries the desired end state, so an at-least-once
// redelivery converges to the same result with no dedup ledger and no send_id.
//
//   - guard  resolves the reactor's active dialog and enforces
//     can_react_messages, message existence, domain and not-deleted;
//   - del    removes the row only when the emoji is empty (an explicit clear);
//   - ups    inserts or replaces the row for a non-empty emoji that differs
//     from the stored one.
//
// An emoji equal to the one already stored matches neither del nor ups, so the
// outcome is 'unchanged' — a no-op, NOT a toggle-off (toggling off is the
// client sending an empty emoji). del and ups are mutually exclusive by
// construction, so they never touch the same row within the statement.
const setReactionSQL = `
with guard as (
    select m.thread_id, td.id as member_dialog_id
    from im_message.messages m
    join im_thread.thread_dialog td
        on td.thread_id = m.thread_id
       and td.member_id = @ReactorID
       and td.deleted_at is null
    left join im_thread.thread_permission tp on tp.thread_dialog_id = td.id
    where m.id = @MessageID
      and m.domain_id = @DomainID
      and m.deleted_at is null
      and coalesce(tp.can_react_messages, true)
    order by td.id desc
    limit 1
),
prev as (
    select r.id, r.emoji, r.updated_at
    from im_message.message_reactions r
    where r.message_id = @MessageID and r.reactor_id = @ReactorID
),
del as (
    delete from im_message.message_reactions r
    using guard g
    where r.message_id = @MessageID
      and r.reactor_id = @ReactorID
      and @Emoji = ''
    returning 'removed'::text as action, ''::text as emoji, now() as reacted_at
),
ups as (
    insert into im_message.message_reactions
        (id, message_id, thread_id, domain_id, reactor_id, reactor_member_id, emoji)
    select @ID, @MessageID, g.thread_id, @DomainID, @ReactorID, g.member_dialog_id, @Emoji
    from guard g
    where @Emoji <> ''
      and not exists (select 1 from prev p where p.emoji = @Emoji)
    on conflict (message_id, reactor_id) do update
        set emoji = excluded.emoji,
            updated_at = now()
    returning 'set'::text as action, emoji, updated_at as reacted_at
),
outcome as (
    select action, emoji, reacted_at from del
    union all
    select action, emoji, reacted_at from ups
)
select
    exists (select 1 from guard) as allowed,
    (select g.thread_id from guard g) as thread_id,
    coalesce((select o.action from outcome o), 'unchanged') as action,
    coalesce((select o.emoji from outcome o), (select p.emoji from prev p), '') as emoji,
    coalesce(
        (select (extract(epoch from o.reacted_at) * 1000)::bigint from outcome o),
        (select (extract(epoch from p.updated_at) * 1000)::bigint from prev p),
        0
    ) as reacted_at
`

// SetReaction sets, replaces or clears the reactor's single reaction on a
// message and reports what changed. It is idempotent by construction: setting
// the already-stored emoji, or clearing an already-absent reaction, both settle
// as 'unchanged' so no spurious second event is emitted — no send_id needed.
func (s *messageReactionStore) SetReaction(ctx context.Context, r *model.Reaction) (*model.ReactionResult, error) {
	if r == nil {
		return nil, errors.InvalidArgument("reaction cannot be nil", errors.WithID("postgres.message_reaction.set"))
	}

	if r.MessageID == uuid.Nil {
		return nil, errors.InvalidArgument("message id cannot be nil", errors.WithID("postgres.message_reaction.set"))
	}

	if r.ReactorID == uuid.Nil {
		return nil, errors.InvalidArgument("reactor id cannot be nil", errors.WithID("postgres.message_reaction.set"))
	}

	args := pgx.NamedArgs{
		"ID":        uuid.Must(uuid.NewV7()),
		"MessageID": r.MessageID,
		"DomainID":  r.DomainID,
		"ReactorID": r.ReactorID,
		"Emoji":     r.Emoji,
	}

	rows, err := s.db.Query(ctx, setReactionSQL, args)
	if err != nil {
		return nil, errors.Internal("executing set reaction query", errors.WithCause(err), errors.WithID("postgres.message_reaction.set.query"))
	}

	out, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[reactionOutcome])
	if err != nil {
		return nil, errors.Internal("collecting reaction outcome", errors.WithCause(err), errors.WithID("postgres.message_reaction.set.collecting"))
	}

	if !out.Allowed {
		return nil, store.ErrReactionNotAllowed
	}

	res := &model.ReactionResult{
		Action:  model.ReactionAction(out.Action),
		Emoji:   out.Emoji,
		Changed: out.Action == string(model.ReactionActionSet) || out.Action == string(model.ReactionActionRemoved),
	}

	if out.ThreadID != nil {
		res.ThreadID = *out.ThreadID
	}

	if out.ReactedAt > 0 {
		res.ReactedAt = time.UnixMilli(out.ReactedAt).UTC()
	}

	return res, nil
}

const aggregateForMessageSQL = `
select emoji,
       cnt::int as count,
       to_jsonb(reactor_ids) as reactor_ids,
       last_ms as last_reacted_at
from (
    select mr.emoji,
           count(*) as cnt,
           (array_agg(mr.reactor_id order by mr.created_at))[1:12] as reactor_ids,
           min(mr.created_at) as first_at,
           (extract(epoch from max(mr.updated_at)) * 1000)::bigint as last_ms
    from im_message.message_reactions mr
    where mr.message_id = @MessageID
    group by mr.emoji
) e
order by e.first_at
`

// AggregateForMessage returns the per-emoji reaction aggregate for a message,
// ordered by first-reaction time (the read-model the history view exposes).
func (s *messageReactionStore) AggregateForMessage(ctx context.Context, messageID uuid.UUID) ([]*model.MessageReaction, error) {
	if messageID == uuid.Nil {
		return nil, errors.InvalidArgument("message id cannot be nil", errors.WithID("postgres.message_reaction.aggregate_for_message"))
	}

	args := pgx.NamedArgs{
		"MessageID": messageID,
	}

	rows, err := s.db.Query(ctx, aggregateForMessageSQL, args)
	if err != nil {
		return nil, errors.Internal("executing aggregate for message query", errors.WithCause(err), errors.WithID("postgres.message_reaction.aggregate_for_message.query"))
	}

	agg, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.MessageReaction])
	if err != nil {
		return nil, errors.Internal("collecting message reactions aggregate", errors.WithCause(err), errors.WithID("postgres.message_reaction.aggregate_for_message.collecting"))
	}

	if len(agg) == 0 {
		return nil, nil
	}

	return agg, nil
}
