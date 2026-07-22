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

type messageStatusStore struct {
	db Querier
}

func NewMessageStatusStore(q Querier) store.MessageStatusStore {
	return &messageStatusStore{db: q}
}

func (s *messageStatusStore) InsertSent(ctx context.Context, msg *model.Message, recipientIDs []uuid.UUID) error {
	if msg == nil || len(recipientIDs) == 0 {
		return nil
	}

	const query = `
		insert into im_message.message_statuses (domain_id, thread_id, message_id, member_id, status)
		select @DomainID, @ThreadID, @MessageID, u.member_id, 1
		from unnest(@MemberIDs::uuid[]) as u(member_id)
		on conflict (message_id, member_id) do nothing
	`

	args := pgx.NamedArgs{
		"DomainID":  msg.DomainID,
		"ThreadID":  msg.ThreadID,
		"MessageID": msg.ID,
		"MemberIDs": recipientIDs,
	}

	if _, err := s.db.Exec(ctx, query, args); err != nil {
		return errors.Internal("inserting sent statuses", errors.WithCause(err), errors.WithID("postgres.message_status.insert_sent"))
	}

	return nil
}

// MarkDelivered performs a monotonic batch upsert to DELIVERED.
// Allowed transitions: none -> delivered (late receipt for a message that
// predates status tracking), sent -> delivered, failed -> delivered (retry).
// Receipts are validated against im_message.messages: unknown messages and
// the sender itself are ignored, thread/domain ids are taken from the message.
func (s *messageStatusStore) MarkDelivered(ctx context.Context, receipts []*model.StatusReceipt) ([]*model.StatusChange, error) {
	receipts = dedupStatusReceipts(receipts)
	if len(receipts) == 0 {
		return nil, nil
	}

	const query = `
		insert into im_message.message_statuses as ms
			(domain_id, thread_id, message_id, member_id, status, delivered_at, via)
		select m.domain_id, m.thread_id, m.id, u.member_id, 2, u.confirmed_at, nullif(u.via, '')
		from unnest(@MessageIDs::uuid[], @MemberIDs::uuid[], @ConfirmedAts::timestamptz[], @Vias::text[])
			as u(message_id, member_id, confirmed_at, via)
		join im_message.messages m on m.id = u.message_id
		where m.sender_id <> u.member_id
		and exists (
			select 1 from im_thread.thread_dialog td
			where td.thread_id = m.thread_id and td.member_id = u.member_id
		)
		on conflict (message_id, member_id) do update set
			status = excluded.status,
			delivered_at = coalesce(ms.delivered_at, excluded.delivered_at),
			failed_at = null,
			error = null,
			via = coalesce(excluded.via, ms.via),
			updated_at = now()
		where ms.status in (1, 4)
		returning ms.domain_id, ms.thread_id, ms.message_id, ms.member_id, ms.status, ms.via, ms.error, ms.updated_at
	`

	var (
		messageIDs   = make([]uuid.UUID, len(receipts))
		memberIDs    = make([]uuid.UUID, len(receipts))
		confirmedAts = make([]time.Time, len(receipts))
		vias         = make([]string, len(receipts))
	)

	for i, r := range receipts {
		messageIDs[i] = r.MessageID
		memberIDs[i] = r.MemberID
		confirmedAts[i] = confirmedAtOrNow(r.At)
		vias[i] = r.Via
	}

	args := pgx.NamedArgs{
		"MessageIDs":   messageIDs,
		"MemberIDs":    memberIDs,
		"ConfirmedAts": confirmedAts,
		"Vias":         vias,
	}

	return s.collectChanges(ctx, query, args, "postgres.message_status.mark_delivered")
}

// MarkRead performs a monotonic bulk upsert to READ with read-up-to
// semantics: for every receipt, all messages of the thread up to (and
// including) UpToMessageID that were not sent by the recipient are marked
// as read. Message ids are UUIDv7, so id order matches creation order.
// Allowed transitions: none/sent/delivered -> read, failed -> read
// (a read receipt implies the message reached the recipient).
func (s *messageStatusStore) MarkRead(ctx context.Context, receipts []*model.ReadReceipt) ([]*model.StatusChange, error) {
	receipts = dedupReadReceipts(receipts)
	if len(receipts) == 0 {
		return nil, nil
	}

	const query = `
		insert into im_message.message_statuses as ms
			(domain_id, thread_id, message_id, member_id, status, read_at, via)
		select m.domain_id, m.thread_id, m.id, u.member_id, 3, u.confirmed_at, nullif(u.via, '')
		from unnest(@ThreadIDs::uuid[], @MemberIDs::uuid[], @UpToMessageIDs::uuid[], @ConfirmedAts::timestamptz[], @Vias::text[])
			as u(thread_id, member_id, up_to_message_id, confirmed_at, via)
		join im_message.messages m
			on m.thread_id = u.thread_id
			and m.id <= u.up_to_message_id
			and m.sender_id <> u.member_id
		where exists (
			select 1 from im_thread.thread_dialog td
			where td.thread_id = u.thread_id and td.member_id = u.member_id
		)
		and not exists (
			select 1 from im_message.message_statuses cur
			where cur.message_id = m.id and cur.member_id = u.member_id and cur.status = 3
		)
		on conflict (message_id, member_id) do update set
			status = excluded.status,
			read_at = coalesce(ms.read_at, excluded.read_at),
			failed_at = null,
			error = null,
			via = coalesce(excluded.via, ms.via),
			updated_at = now()
		where ms.status < 3 or ms.status = 4
		returning ms.domain_id, ms.thread_id, ms.message_id, ms.member_id, ms.status, ms.via, ms.error, ms.updated_at
	`

	var (
		threadIDs      = make([]uuid.UUID, len(receipts))
		memberIDs      = make([]uuid.UUID, len(receipts))
		upToMessageIDs = make([]uuid.UUID, len(receipts))
		confirmedAts   = make([]time.Time, len(receipts))
		vias           = make([]string, len(receipts))
	)

	for i, r := range receipts {
		threadIDs[i] = r.ThreadID
		memberIDs[i] = r.MemberID
		upToMessageIDs[i] = r.UpToMessageID
		confirmedAts[i] = confirmedAtOrNow(r.At)
		vias[i] = r.Via
	}

	args := pgx.NamedArgs{
		"ThreadIDs":      threadIDs,
		"MemberIDs":      memberIDs,
		"UpToMessageIDs": upToMessageIDs,
		"ConfirmedAts":   confirmedAts,
		"Vias":           vias,
	}

	return s.collectChanges(ctx, query, args, "postgres.message_status.mark_read")
}

// MarkFailed performs a monotonic batch upsert to FAILED with provider
// error details. Only sent -> failed is allowed: a failure receipt for a
// message that already reached the recipient is ignored.
func (s *messageStatusStore) MarkFailed(ctx context.Context, receipts []*model.StatusReceipt) ([]*model.StatusChange, error) {
	receipts = dedupStatusReceipts(receipts)
	if len(receipts) == 0 {
		return nil, nil
	}

	const query = `
		insert into im_message.message_statuses as ms
			(domain_id, thread_id, message_id, member_id, status, failed_at, error, via)
		select m.domain_id, m.thread_id, m.id, u.member_id, 4, u.confirmed_at, u.error, nullif(u.via, '')
		from unnest(@MessageIDs::uuid[], @MemberIDs::uuid[], @ConfirmedAts::timestamptz[], @Errors::jsonb[], @Vias::text[])
			as u(message_id, member_id, confirmed_at, error, via)
		join im_message.messages m on m.id = u.message_id
		where m.sender_id <> u.member_id
		and exists (
			select 1 from im_thread.thread_dialog td
			where td.thread_id = m.thread_id and td.member_id = u.member_id
		)
		on conflict (message_id, member_id) do update set
			status = excluded.status,
			failed_at = excluded.failed_at,
			error = excluded.error,
			via = coalesce(excluded.via, ms.via),
			updated_at = now()
		where ms.status = 1
		returning ms.domain_id, ms.thread_id, ms.message_id, ms.member_id, ms.status, ms.via, ms.error, ms.updated_at
	`

	var (
		messageIDs   = make([]uuid.UUID, len(receipts))
		memberIDs    = make([]uuid.UUID, len(receipts))
		confirmedAts = make([]time.Time, len(receipts))
		errs         = make([]map[string]any, len(receipts))
		vias         = make([]string, len(receipts))
	)

	for i, r := range receipts {
		messageIDs[i] = r.MessageID
		memberIDs[i] = r.MemberID
		confirmedAts[i] = confirmedAtOrNow(r.At)
		errs[i] = r.Error
		vias[i] = r.Via
	}

	args := pgx.NamedArgs{
		"MessageIDs":   messageIDs,
		"MemberIDs":    memberIDs,
		"ConfirmedAts": confirmedAts,
		"Errors":       errs,
		"Vias":         vias,
	}

	return s.collectChanges(ctx, query, args, "postgres.message_status.mark_failed")
}

func (s *messageStatusStore) collectChanges(ctx context.Context, query string, args pgx.NamedArgs, operationID string) ([]*model.StatusChange, error) {
	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal("executing status upsert", errors.WithCause(err), errors.WithID(operationID))
	}

	changes, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.StatusChange])
	if err != nil {
		return nil, errors.Internal("collecting status changes", errors.WithCause(err), errors.WithID(operationID))
	}

	return changes, nil
}

func confirmedAtOrNow(at time.Time) time.Time {
	if at.IsZero() {
		return time.Now().UTC()
	}

	return at
}

// dedupStatusReceipts drops repeated (message_id, member_id) pairs, keeping
// the first occurrence: an upsert cannot affect the same row twice within
// one statement.
func dedupStatusReceipts(receipts []*model.StatusReceipt) []*model.StatusReceipt {
	type key struct{ messageID, memberID uuid.UUID }

	seen := make(map[key]struct{}, len(receipts))
	out := make([]*model.StatusReceipt, 0, len(receipts))

	for _, r := range receipts {
		if r == nil {
			continue
		}

		k := key{r.MessageID, r.MemberID}
		if _, ok := seen[k]; ok {
			continue
		}

		seen[k] = struct{}{}

		out = append(out, r)
	}

	return out
}

// dedupReadReceipts collapses receipts of the same (thread_id, member_id)
// into a single one with the greatest UpToMessageID: ranges overlap, and an
// upsert cannot affect the same row twice within one statement.
func dedupReadReceipts(receipts []*model.ReadReceipt) []*model.ReadReceipt {
	type key struct{ threadID, memberID uuid.UUID }

	best := make(map[key]*model.ReadReceipt, len(receipts))
	order := make([]key, 0, len(receipts))

	for _, r := range receipts {
		if r == nil {
			continue
		}

		k := key{r.ThreadID, r.MemberID}

		cur, ok := best[k]
		if !ok {
			best[k] = r
			order = append(order, k)

			continue
		}

		// UUIDv7 ids are time-ordered, so byte comparison picks the latest.
		if greaterUUID(r.UpToMessageID, cur.UpToMessageID) {
			best[k] = r
		}
	}

	out := make([]*model.ReadReceipt, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}

	return out
}

func greaterUUID(a, b uuid.UUID) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}

	return false
}
