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

// setReactionSQL applies the toggle/replace/remove semantics for one
// (message, reactor) pair in a single round-trip:
//
//   - guard  resolves the reactor's active dialog and enforces
//     can_react_messages, message existence, domain and not-deleted;
//   - claim  records the send_id in the idempotency ledger; a send_id already
//     present means this exact request was applied before, so del/ups are
//     skipped (dedup for at-least-once redelivery — survives a prior toggle-off
//     that deleted the row, and blocks reordered retries of an older request);
//   - del    removes the row when the emoji is empty or repeats the stored one
//     (toggle off);
//   - ups    inserts or replaces the row for any other non-empty emoji.
//
// del and ups are mutually exclusive by construction, so they never touch the
// same row within the statement.
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
claim as (
    insert into im_message.message_reaction_dedup (message_id, reactor_id, send_id)
    select @MessageID, @ReactorID, @SendID
    from guard g
    where @SendID <> ''
    on conflict (message_id, reactor_id, send_id) do nothing
    returning send_id
),
-- A keyed request already present in the ledger but not (re)claimed here was
-- applied by an earlier delivery: treat this delivery as a no-op.
dup as (
    select 1
    where @SendID <> ''
      and exists (select 1 from guard)
      and not exists (select 1 from claim)
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
      and (@Emoji = '' or r.emoji = @Emoji)
      and not exists (select 1 from dup)
    returning 'removed'::text as action, ''::text as emoji, now() as reacted_at
),
ups as (
    insert into im_message.message_reactions
        (id, message_id, thread_id, domain_id, reactor_id, reactor_member_id, emoji)
    select @ID, @MessageID, g.thread_id, @DomainID, @ReactorID, g.member_dialog_id, @Emoji
    from guard g
    where @Emoji <> ''
      and not exists (select 1 from prev p where p.emoji = @Emoji)
      and not exists (select 1 from dup)
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
// message and reports what changed. It is idempotent: an empty result, a repeat
// of the current emoji handled as toggle-off, and a send_id already applied are
// all resolved without a spurious second event.
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
		"SendID":    r.IdempotencyKey,
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
