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

	// Bump each recipient's denormalized unread counter. Content messages only —
	// system messages are never counted as unread; the sender is not among the
	// recipients, so own messages never bump.
	if msg.Type == model.MessageTypeSystem {
		return nil
	}

	const bumpQuery = `
		update im_thread.thread_dialog
		set unread_count = unread_count + 1
		where thread_id = @ThreadID
		  and member_id = any(@MemberIDs::uuid[])
		  and deleted_at is null
	`

	bumpArgs := pgx.NamedArgs{
		"ThreadID":  msg.ThreadID,
		"MemberIDs": recipientIDs,
	}

	if _, err := s.db.Exec(ctx, bumpQuery, bumpArgs); err != nil {
		return errors.Internal("bumping unread counters", errors.WithCause(err), errors.WithID("postgres.message_status.bump_unread"))
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

	changes, err := s.collectChanges(ctx, query, args, "postgres.message_status.mark_read")
	if err != nil {
		return nil, err
	}

	// Advance each member's read horizon and refresh the denormalized unread
	// counter in the same transaction.
	if err := s.advanceReadHorizon(ctx, receipts); err != nil {
		return nil, err
	}

	return changes, nil
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

// ReadUnread returns the denormalized unread_count per thread for the member,
// read straight from thread_dialog (O(1) per row, no message scan). Threads
// with no active row for the member are omitted.
func (s *messageStatusStore) ReadUnread(ctx context.Context, domainID int32, memberID uuid.UUID, threadIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	if memberID == uuid.Nil || len(threadIDs) == 0 {
		return nil, nil
	}

	const query = `
		select thread_id, unread_count
		from im_thread.thread_dialog
		where member_id = @MemberID
		  and (@DomainID <= 0 or domain_id = @DomainID)
		  and thread_id = any(@ThreadIDs::uuid[])
		  and deleted_at is null
	`

	args := pgx.NamedArgs{
		"MemberID":  memberID,
		"DomainID":  domainID,
		"ThreadIDs": threadIDs,
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal("reading unread counts", errors.WithCause(err), errors.WithID("postgres.message_status.read_unread"))
	}

	type unreadRow struct {
		ThreadID uuid.UUID `db:"thread_id"`
		Unread   int64     `db:"unread_count"`
	}

	counts, err := pgx.CollectRows(rows, pgx.RowToStructByName[unreadRow])
	if err != nil {
		return nil, errors.Internal("collecting unread counts", errors.WithCause(err), errors.WithID("postgres.message_status.read_unread"))
	}

	result := make(map[uuid.UUID]int64, len(counts))
	for _, c := range counts {
		result[c.ThreadID] = c.Unread
	}

	return result, nil
}

// UnreadSummary returns the member's denormalized unread totals across the
// threads they are still an active participant of (thread_dialog not
// soft-deleted): the number of chats with unread messages and the total unread
// message count. Read straight from the denormalized counters.
func (s *messageStatusStore) UnreadSummary(ctx context.Context, domainID int32, memberID uuid.UUID) (model.UnreadSummary, error) {
	if memberID == uuid.Nil {
		return model.UnreadSummary{}, nil
	}

	const query = `
		select
			count(*) filter (where unread_count > 0) as unread_chats,
			coalesce(sum(unread_count), 0) as unread_messages
		from im_thread.thread_dialog
		where member_id = @MemberID
		  and (@DomainID <= 0 or domain_id = @DomainID)
		  and deleted_at is null
	`

	args := pgx.NamedArgs{
		"MemberID": memberID,
		"DomainID": domainID,
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return model.UnreadSummary{}, errors.Internal("querying unread summary", errors.WithCause(err), errors.WithID("postgres.message_status.unread_summary"))
	}

	summary, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[model.UnreadSummary])
	if err != nil {
		return model.UnreadSummary{}, errors.Internal("collecting unread summary", errors.WithCause(err), errors.WithID("postgres.message_status.unread_summary"))
	}

	return summary, nil
}

// advanceReadHorizon moves each member's read horizon
// (thread_dialog.last_read_message_id) forward to the receipt's up-to boundary —
// monotonically, never backward — and refreshes the denormalized unread_count
// from the new horizon: content messages after it that the member did not send.
// Mirrors Telegram's read_inbox_max_id + dialog unread_count. Runs in the same
// transaction as the MarkRead status upsert.
func (s *messageStatusStore) advanceReadHorizon(ctx context.Context, receipts []*model.ReadReceipt) error {
	if len(receipts) == 0 {
		return nil
	}

	var (
		threadIDs = make([]uuid.UUID, len(receipts))
		memberIDs = make([]uuid.UUID, len(receipts))
		upTos     = make([]uuid.UUID, len(receipts))
	)

	for i, r := range receipts {
		threadIDs[i] = r.ThreadID
		memberIDs[i] = r.MemberID
		upTos[i] = r.UpToMessageID
	}

	// The horizon CASE is repeated in SET and in the count subquery: the UPDATE
	// target (td) is only in scope of SET/WHERE, not of a lateral FROM item.
	const query = `
		update im_thread.thread_dialog td
		set last_read_message_id = case
				when td.last_read_message_id is null or r.up_to > td.last_read_message_id
				then r.up_to else td.last_read_message_id end,
		    unread_count = coalesce((
		        select count(*)
		        from im_message.messages m
		        where m.thread_id = td.thread_id
		          and m.sender_id <> td.member_id
		          and m.type <> @SystemType
		          and m.id > case
		                when td.last_read_message_id is null or r.up_to > td.last_read_message_id
		                then r.up_to else td.last_read_message_id end
		    ), 0)
		from unnest(@ThreadIDs::uuid[], @MemberIDs::uuid[], @UpTos::uuid[])
			as r(thread_id, member_id, up_to)
		where td.thread_id = r.thread_id
		  and td.member_id = r.member_id
		  and td.deleted_at is null
	`

	args := pgx.NamedArgs{
		"ThreadIDs":  threadIDs,
		"MemberIDs":  memberIDs,
		"UpTos":      upTos,
		"SystemType": int(model.MessageTypeSystem),
	}

	if _, err := s.db.Exec(ctx, query, args); err != nil {
		return errors.Internal("advancing read horizon", errors.WithCause(err), errors.WithID("postgres.message_status.advance_read_horizon"))
	}

	return nil
}

// ReconcileUnread recomputes unread_count for every active dialog straight from
// the read horizon (optionally scoped to a domain). A drift safety net for a
// periodic job, not the hot path. Returns the number of rows updated.
func (s *messageStatusStore) ReconcileUnread(ctx context.Context, domainID int32) (int64, error) {
	const query = `
		update im_thread.thread_dialog td
		set unread_count = coalesce((
			select count(*)
			from im_message.messages m
			where m.thread_id = td.thread_id
			  and m.sender_id <> td.member_id
			  and m.type <> @SystemType
			  and (td.last_read_message_id is null or m.id > td.last_read_message_id)
		), 0)
		where (@DomainID <= 0 or td.domain_id = @DomainID)
		  and td.deleted_at is null
	`

	tag, err := s.db.Exec(ctx, query, pgx.NamedArgs{
		"DomainID":   domainID,
		"SystemType": int(model.MessageTypeSystem),
	})
	if err != nil {
		return 0, errors.Internal("reconciling unread counts", errors.WithCause(err), errors.WithID("postgres.message_status.reconcile_unread"))
	}

	return tag.RowsAffected(), nil
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
