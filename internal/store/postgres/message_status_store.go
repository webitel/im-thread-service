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

// MarkDelivered performs a monotonic batch upsert to DELIVERED using watermark
// semantics: every receipt advances the delivered horizon in thread_dialog and
// synthesizes one StatusChange per receipt with UpToMessageID set.
//
// For receipts with UpToMessageID set (ws, push from clients), use that boundary.
// For receipts with only MessageID set (provider/bot), resolve thread_id and seq
// from im_message.messages and use MessageID as the delivered-up-to boundary.
//
// Allowed transitions: none -> delivered (late receipt), sent -> delivered, failed -> delivered (retry).
// Receipts are validated against im_message.messages: unknown messages and
// the sender itself are ignored, thread/domain ids are taken from the message.
func (s *messageStatusStore) MarkDelivered(ctx context.Context, receipts []*model.StatusReceipt) ([]*model.StatusChange, error) {
	receipts = dedupStatusReceipts(receipts)
	if len(receipts) == 0 {
		return nil, nil
	}

	// Resolve ThreadID for receipts that lack it (provider/bot per-message receipts).
	if err := s.resolveThreadIDsForDelivery(ctx, receipts); err != nil {
		return nil, err
	}

	// Convert per-message receipts to watermark form: use MessageID as the UpToMessageID.
	for i, r := range receipts {
		if r.MessageID != uuid.Nil && r.UpToMessageID == uuid.Nil {
			receipts[i].UpToMessageID = r.MessageID
		}
	}

	// Resolve seq from message_id for receipts lacking UpToSeq (legacy/envelope path).
	if err := s.resolveSeqFromMessageID(ctx, receipts); err != nil {
		return nil, err
	}

	// Advance delivered horizon for all receipts (all are now watermark form).
	if err := s.advanceDeliveredHorizon(ctx, receipts); err != nil {
		return nil, err
	}

	// Synthesize StatusChange for each receipt.
	now := time.Now().UTC()

	allChanges := make([]*model.StatusChange, 0, len(receipts))
	for _, r := range receipts {
		allChanges = append(allChanges, &model.StatusChange{
			DomainID:      r.DomainID,
			ThreadID:      r.ThreadID,
			UpToMessageID: r.UpToMessageID,
			UpToSeq:       r.UpToSeq,
			MemberID:      r.MemberID,
			Status:        model.MessageDeliveryStatusDelivered,
			Via:           &r.Via,
			UpdatedAt:     now,
		})
	}

	return allChanges, nil
}

// MarkRead performs a monotonic bulk upsert to READ with read-up-to
// semantics: for every receipt, advances the read horizon and synthesizes
// one StatusChange per receipt with UpToMessageID and UpToSeq set. The watermark
// path bypasses per-message inserts, instead updating thread_dialog.last_read_seq
// and refreshing the denormalized unread counter.
// Allowed transitions: the read horizon is monotonic, never backward.
func (s *messageStatusStore) MarkRead(ctx context.Context, receipts []*model.ReadReceipt) ([]*model.StatusChange, error) {
	receipts = dedupReadReceipts(receipts)
	if len(receipts) == 0 {
		return nil, nil
	}

	// Resolve seq from message_id for receipts lacking UpToSeq (legacy/envelope path).
	if err := s.resolveReadSeqFromMessageID(ctx, receipts); err != nil {
		return nil, err
	}

	// Advance each member's read horizon and refresh the denormalized unread counter.
	if err := s.advanceReadHorizon(ctx, receipts); err != nil {
		return nil, err
	}

	// Synthesize StatusChange for each receipt, mirroring the watermark path in MarkDelivered.
	now := time.Now().UTC()

	allChanges := make([]*model.StatusChange, 0, len(receipts))
	for _, r := range receipts {
		allChanges = append(allChanges, &model.StatusChange{
			DomainID:      r.DomainID,
			ThreadID:      r.ThreadID,
			UpToMessageID: r.UpToMessageID,
			UpToSeq:       r.UpToSeq,
			MemberID:      r.MemberID,
			Status:        model.MessageDeliveryStatusRead,
			Via:           &r.Via,
			UpdatedAt:     now,
		})
	}

	return allChanges, nil
}

// MarkFailed performs a batch upsert to message_errors (ERRORS-ONLY table)
// with provider error details. Receipts are deduplicated by (message_id, member_id).
// Returns StatusChange rows for failed receipts for fan-out to clients.
func (s *messageStatusStore) MarkFailed(ctx context.Context, receipts []*model.StatusReceipt) ([]*model.StatusChange, error) {
	receipts = dedupStatusReceipts(receipts)
	if len(receipts) == 0 {
		return nil, nil
	}

	const query = `
		insert into im_message.message_errors
			(domain_id, thread_id, message_id, member_id, failed_at, error, via)
		select m.domain_id, m.thread_id, m.id, u.member_id, u.confirmed_at, u.error, nullif(u.via, '')
		from unnest(@MessageIDs::uuid[], @MemberIDs::uuid[], @ConfirmedAts::timestamptz[], @Errors::jsonb[], @Vias::text[])
			as u(message_id, member_id, confirmed_at, error, via)
		join im_message.messages m on m.id = u.message_id
		where m.sender_id <> u.member_id
		and exists (
			select 1 from im_thread.thread_dialog td
			where td.thread_id = m.thread_id and td.member_id = u.member_id
		)
		on conflict (message_id, member_id) do update set
			failed_at = excluded.failed_at,
			error = excluded.error,
			via = coalesce(excluded.via, message_errors.via),
			updated_at = now()
		returning domain_id, thread_id, message_id, member_id, 4::smallint as status, via, error, updated_at
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
		return make(map[uuid.UUID]int64), nil
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

// advanceReadHorizon moves each member's read horizon (thread_dialog.last_read_seq)
// forward to the receipt's up-to boundary — monotonically, never backward — and
// refreshes the denormalized unread_count from the new horizon: content messages
// after it that the member did not send. Uses seq for watermarks but resolves from
// message_id for legacy receipts. Runs in the same transaction as MarkRead.
func (s *messageStatusStore) advanceReadHorizon(ctx context.Context, receipts []*model.ReadReceipt) error {
	if len(receipts) == 0 {
		return nil
	}

	var (
		threadIDs = make([]uuid.UUID, len(receipts))
		memberIDs = make([]uuid.UUID, len(receipts))
		upToSeqs  = make([]*int64, len(receipts))
		upToMsgs  = make([]uuid.UUID, len(receipts))
	)

	for i, r := range receipts {
		threadIDs[i] = r.ThreadID

		memberIDs[i] = r.MemberID
		if r.UpToSeq != 0 {
			upToSeqs[i] = &r.UpToSeq
		}

		upToMsgs[i] = r.UpToMessageID
	}

	const query = `
		update im_thread.thread_dialog td
		set last_read_seq = case
				when td.last_read_seq is null or r.up_to_seq > td.last_read_seq
				then r.up_to_seq else td.last_read_seq end,
		    unread_count = coalesce((
		        select count(*)
		        from im_message.messages m
		        where m.thread_id = td.thread_id
		          and m.sender_id <> td.member_id
		          and m.type <> @SystemType
		          and m.seq > case
		                when td.last_read_seq is null or r.up_to_seq > td.last_read_seq
		                then r.up_to_seq else td.last_read_seq end
		    ), 0)
		from unnest(@ThreadIDs::uuid[], @MemberIDs::uuid[], @UpToSeqs::bigint[], @UpToMsgs::uuid[])
			as r(thread_id, member_id, up_to_seq, up_to_msg)
		where td.thread_id = r.thread_id
		  and td.member_id = r.member_id
		  and td.deleted_at is null
		  and r.up_to_seq is not null
	`

	args := pgx.NamedArgs{
		"ThreadIDs":  threadIDs,
		"MemberIDs":  memberIDs,
		"UpToSeqs":   upToSeqs,
		"UpToMsgs":   upToMsgs,
		"SystemType": int(model.MessageTypeSystem),
	}

	if _, err := s.db.Exec(ctx, query, args); err != nil {
		return errors.Internal("advancing read horizon", errors.WithCause(err), errors.WithID("postgres.message_status.advance_read_horizon"))
	}

	return nil
}

// advanceDeliveredHorizon moves each member's delivered horizon
// (thread_dialog.last_delivered_seq) forward to the receipt's up-to boundary —
// monotonically, never backward. Unlike advanceReadHorizon, it does not recompute
// unread_count (delivered does not affect unread — only read does).
// Uses seq for watermarks. Runs in the same transaction as MarkDelivered.
func (s *messageStatusStore) advanceDeliveredHorizon(ctx context.Context, receipts []*model.StatusReceipt) error {
	if len(receipts) == 0 {
		return nil
	}

	var (
		threadIDs = make([]uuid.UUID, len(receipts))
		memberIDs = make([]uuid.UUID, len(receipts))
		upToSeqs  = make([]*int64, len(receipts))
	)

	for i, r := range receipts {
		threadIDs[i] = r.ThreadID

		memberIDs[i] = r.MemberID
		if r.UpToSeq != 0 {
			upToSeqs[i] = &r.UpToSeq
		}
	}

	const query = `
		update im_thread.thread_dialog td
		set last_delivered_seq = case
				when td.last_delivered_seq is null or r.up_to_seq > td.last_delivered_seq
				then r.up_to_seq else td.last_delivered_seq end
		from unnest(@ThreadIDs::uuid[], @MemberIDs::uuid[], @UpToSeqs::bigint[])
			as r(thread_id, member_id, up_to_seq)
		where td.thread_id = r.thread_id
		  and td.member_id = r.member_id
		  and td.deleted_at is null
		  and r.up_to_seq is not null
	`

	args := pgx.NamedArgs{
		"ThreadIDs": threadIDs,
		"MemberIDs": memberIDs,
		"UpToSeqs":  upToSeqs,
	}

	if _, err := s.db.Exec(ctx, query, args); err != nil {
		return errors.Internal("advancing delivered horizon", errors.WithCause(err), errors.WithID("postgres.message_status.advance_delivered_horizon"))
	}

	return nil
}

// resolveReadSeqFromMessageID fills in missing UpToSeq for read receipts by
// looking up the message's seq from im_message.messages. For receipts with UpToSeq
// already set, it does nothing. This is used for legacy/envelope receipts that
// have UpToMessageID but not UpToSeq.
func (s *messageStatusStore) resolveReadSeqFromMessageID(ctx context.Context, receipts []*model.ReadReceipt) error {
	// Collect indices of receipts that need seq resolution.
	var needsResolve []int

	for i, r := range receipts {
		if r.UpToSeq == 0 && r.UpToMessageID != uuid.Nil {
			needsResolve = append(needsResolve, i)
		}
	}

	if len(needsResolve) == 0 {
		return nil
	}

	// Extract MessageIDs for the lookup.
	messageIDs := make([]uuid.UUID, len(needsResolve))
	for i, idx := range needsResolve {
		messageIDs[i] = receipts[idx].UpToMessageID
	}

	const query = `
		select id, seq
		from im_message.messages
		where id = any(@MessageIDs::uuid[])
	`

	rows, err := s.db.Query(ctx, query, pgx.NamedArgs{
		"MessageIDs": messageIDs,
	})
	if err != nil {
		return errors.Internal("resolving seq from message IDs", errors.WithCause(err), errors.WithID("postgres.message_status.resolve_read_seq"))
	}

	type msgRow struct {
		ID  uuid.UUID
		Seq int64
	}

	msgs, err := pgx.CollectRows(rows, pgx.RowToStructByName[msgRow])
	if err != nil {
		return errors.Internal("collecting resolved seq values", errors.WithCause(err), errors.WithID("postgres.message_status.resolve_read_seq"))
	}

	// Build a map for quick lookup.
	msgMap := make(map[uuid.UUID]int64, len(msgs))
	for _, m := range msgs {
		msgMap[m.ID] = m.Seq
	}

	// Fill in UpToSeq for receipts that need resolution.
	for _, idx := range needsResolve {
		if seq, ok := msgMap[receipts[idx].UpToMessageID]; ok {
			receipts[idx].UpToSeq = seq
		}
	}

	return nil
}

// resolveSeqFromMessageID fills in missing UpToSeq for watermark receipts by
// looking up the message's seq from im_message.messages. For receipts with UpToSeq
// already set, it does nothing. This is used for legacy/envelope receipts that
// have UpToMessageID but not UpToSeq.
func (s *messageStatusStore) resolveSeqFromMessageID(ctx context.Context, receipts []*model.StatusReceipt) error {
	// Collect indices of receipts that need seq resolution.
	var needsResolve []int

	for i, r := range receipts {
		if r.UpToSeq == 0 && r.UpToMessageID != uuid.Nil {
			needsResolve = append(needsResolve, i)
		}
	}

	if len(needsResolve) == 0 {
		return nil
	}

	// Extract MessageIDs for the lookup.
	messageIDs := make([]uuid.UUID, len(needsResolve))
	for i, idx := range needsResolve {
		messageIDs[i] = receipts[idx].UpToMessageID
	}

	const query = `
		select id, seq
		from im_message.messages
		where id = any(@MessageIDs::uuid[])
	`

	rows, err := s.db.Query(ctx, query, pgx.NamedArgs{
		"MessageIDs": messageIDs,
	})
	if err != nil {
		return errors.Internal("resolving seq from message IDs", errors.WithCause(err), errors.WithID("postgres.message_status.resolve_seq"))
	}

	type msgRow struct {
		ID  uuid.UUID
		Seq int64
	}

	msgs, err := pgx.CollectRows(rows, pgx.RowToStructByName[msgRow])
	if err != nil {
		return errors.Internal("collecting resolved seq values", errors.WithCause(err), errors.WithID("postgres.message_status.resolve_seq"))
	}

	// Build a map for quick lookup.
	msgMap := make(map[uuid.UUID]int64, len(msgs))
	for _, m := range msgs {
		msgMap[m.ID] = m.Seq
	}

	// Fill in UpToSeq for receipts that need resolution.
	for _, idx := range needsResolve {
		if seq, ok := msgMap[receipts[idx].UpToMessageID]; ok {
			receipts[idx].UpToSeq = seq
		}
	}

	return nil
}

// resolveThreadIDsForDelivery fills in missing ThreadID, DomainID, and UpToSeq
// for per-message receipts (provider/bot) by looking them up from im_message.messages.
// This ensures all receipts have ThreadID and seq set before advancing the delivered horizon.
func (s *messageStatusStore) resolveThreadIDsForDelivery(ctx context.Context, receipts []*model.StatusReceipt) error {
	// Collect indices of receipts that need ThreadID/seq resolution.
	var needsResolve []int

	for i, r := range receipts {
		if r.ThreadID == uuid.Nil && r.MessageID != uuid.Nil {
			needsResolve = append(needsResolve, i)
		}
	}

	if len(needsResolve) == 0 {
		return nil
	}

	// Extract MessageIDs for the lookup.
	messageIDs := make([]uuid.UUID, len(needsResolve))
	for i, idx := range needsResolve {
		messageIDs[i] = receipts[idx].MessageID
	}

	const query = `
		select id, thread_id, domain_id, sender_id, seq
		from im_message.messages
		where id = any(@MessageIDs::uuid[])
	`

	rows, err := s.db.Query(ctx, query, pgx.NamedArgs{
		"MessageIDs": messageIDs,
	})
	if err != nil {
		return errors.Internal("resolving thread IDs for delivery receipts", errors.WithCause(err), errors.WithID("postgres.message_status.resolve_thread_ids"))
	}

	type msgRow struct {
		ID       uuid.UUID
		ThreadID uuid.UUID
		DomainID int32
		SenderID uuid.UUID
		Seq      int64
	}

	msgs, err := pgx.CollectRows(rows, pgx.RowToStructByName[msgRow])
	if err != nil {
		return errors.Internal("collecting resolved messages", errors.WithCause(err), errors.WithID("postgres.message_status.resolve_thread_ids"))
	}

	// Build a map for quick lookup.
	msgMap := make(map[uuid.UUID]msgRow, len(msgs))
	for _, m := range msgs {
		msgMap[m.ID] = m
	}

	// Fill in ThreadID, DomainID, and UpToSeq for receipts that need resolution.
	for _, idx := range needsResolve {
		m, ok := msgMap[receipts[idx].MessageID]
		if !ok {
			// If message not found, leave ThreadID as Nil (it will fail validation in advanceDeliveredHorizon).
			continue
		}

		receipts[idx].ThreadID = m.ThreadID
		receipts[idx].DomainID = m.DomainID
		receipts[idx].UpToSeq = m.Seq
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

// dedupStatusReceipts collapses receipts that would touch the same row twice
// within one statement. It handles the two receipt shapes separately:
//   - watermark receipts (UpToMessageID set, MessageID zero) collapse by
//     (thread_id, member_id) keeping the greatest UpToMessageID — the same rule
//     as read receipts, because a member's delivered horizon is per thread, NOT
//     per message. Keying these by message_id (always zero) would wrongly merge
//     a member's watermarks across different threads.
//   - per-message receipts (MessageID set) collapse by (message_id, member_id),
//     keeping the first occurrence.
func dedupStatusReceipts(receipts []*model.StatusReceipt) []*model.StatusReceipt {
	type wmKey struct{ threadID, memberID uuid.UUID }

	type pmKey struct{ messageID, memberID uuid.UUID }

	wmIdx := make(map[wmKey]int, len(receipts))
	pmSeen := make(map[pmKey]struct{}, len(receipts))
	out := make([]*model.StatusReceipt, 0, len(receipts))

	for _, r := range receipts {
		if r == nil {
			continue
		}

		if r.UpToMessageID != uuid.Nil {
			k := wmKey{r.ThreadID, r.MemberID}
			if idx, ok := wmIdx[k]; ok {
				// UUIDv7 ids are time-ordered, so keep the latest boundary.
				if greaterUUID(r.UpToMessageID, out[idx].UpToMessageID) {
					out[idx] = r
				}

				continue
			}

			wmIdx[k] = len(out)
			out = append(out, r)

			continue
		}

		k := pmKey{r.MessageID, r.MemberID}
		if _, ok := pmSeen[k]; ok {
			continue
		}

		pmSeen[k] = struct{}{}

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
