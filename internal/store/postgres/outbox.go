package postgres

import (
	"context"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/adapter/journal"
	"github.com/webitel/im-thread-service/internal/domain/event"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

const (
	defaultOutboxTable  = "im_message.messages_outbox"
	defaultOffsetsTable = "im_message.messages_offsets"
	defaultThreadTable  = "im_thread.thread"
	defaultJournalTable = "im_message.thread_updates"
	defaultMarksTable   = "im_thread.contact_updates"
	defaultDialogTable  = "im_thread.thread_dialog"
)

type outboxStore struct {
	q        Querier
	config   sql.PublisherConfig
	wmlogger watermill.LoggerAdapter

	// Fully-qualified table names, overridable by tests to run the real
	// cleanup / update_seq queries against a throwaway schema.
	outboxTable  string
	offsetsTable string
	threadTable  string
	journalTable string
	marksTable   string
	dialogTable  string
}

func NewOutboxStore(q Querier, wmlogger watermill.LoggerAdapter) store.OutboxStore {
	return &outboxStore{
		q:            q,
		wmlogger:     wmlogger,
		outboxTable:  defaultOutboxTable,
		offsetsTable: defaultOffsetsTable,
		threadTable:  defaultThreadTable,
		journalTable: defaultJournalTable,
		marksTable:   defaultMarksTable,
		dialogTable:  defaultDialogTable,
		config: sql.PublisherConfig{
			SchemaAdapter: sql.DefaultPostgreSQLSchema{
				GenerateMessagesTableName: func(_ string) string {
					return defaultOutboxTable
				},
			},
		},
	}
}

var _ store.OutboxStore = (*outboxStore)(nil)

func (o *outboxStore) Publish(ctx context.Context, topic string, evt event.Outboxer) error {
	// Must run within an active transaction to ensure business logic and events commit together.
	tx, ok := o.q.(pgx.Tx)
	if !ok {
		return errors.New("outbox publish: transaction required (querier is not pgx.Tx)")
	}

	// Stamp per-thread monotonic cursor on journal-worthy mutations; row lock
	// serializes concurrent writers so sequence is gap-free and ordered by commit.
	je, journaled := evt.(event.JournalEvent)
	if journaled {
		seq, err := o.nextUpdateSeq(ctx, tx, je.JournalThreadID())
		if err != nil {
			return err
		}

		je.SetUpdateSeq(seq)
	}

	ev, err := evt.ToOutbox()
	if err != nil {
		return fmt.Errorf("outbox publish: %w", err)
	}

	// Write journal row in same transaction under thread lock; head and row
	// commit atomically so journal is contiguous and catch-up never depends on async relay.
	if journaled {
		if err := o.appendJournal(ctx, tx, ev); err != nil {
			return err
		}

		if err := o.markContacts(ctx, tx, je); err != nil {
			return err
		}
	}

	publisher, err := sql.NewPublisher(sql.TxFromPgx(tx), o.config, o.wmlogger)
	if err != nil {
		return fmt.Errorf("failed to create outbox publisher: %w", err)
	}

	msg := message.NewMessage(ev.ID.String(), ev.Payload)
	for k, v := range ev.Metadata {
		msg.Metadata.Set(k, v)
	}

	msg.Metadata.Set("x-routing-key", topic)

	return publisher.Publish(topic, msg)
}

// appendJournal writes the event's journal row. A
// JournalEvent without a projection is a bug: its seq would become a hole.
func (o *outboxStore) appendJournal(ctx context.Context, tx pgx.Tx, ev event.OutboxEvent) error {
	upd, ok, err := journal.ProjectEvent(ev.Metadata["event_type"], ev.Payload)
	if err != nil {
		return errors.Internal("projecting journal entry",
			errors.WithCause(err), errors.WithID("postgres.outbox.journal_project"))
	}

	if !ok {
		return errors.Internal("journal event has no projection",
			errors.WithID("postgres.outbox.journal_unprojectable"), errors.WithValue("event_type", ev.Metadata["event_type"]))
	}

	j := journal.New(tx, journal.WithTables(o.journalTable, o.threadTable))

	if err := j.Append(ctx, upd); err != nil {
		return errors.Internal("appending journal entry",
			errors.WithCause(err), errors.WithID("postgres.outbox.journal_append"))
	}

	return nil
}

// markContacts records that the thread changed in this transaction for every human member
// (plus a contact who just left). Bots are skipped: they never catch up via GetUpdates.
func (o *outboxStore) markContacts(ctx context.Context, tx pgx.Tx, je event.JournalEvent) error {
	var extra *uuid.UUID

	if s, ok := je.(interface{ JournalSubject() uuid.UUID }); ok {
		id := s.JournalSubject()
		extra = &id
	}

	query := fmt.Sprintf(`
		insert into %[1]s (contact_id, thread_id, tx_id)
		select c.contact_id, $1, pg_current_xact_id()::text::bigint
		from (
			select td.member_id as contact_id
			from %[2]s td
			where td.thread_id = $1 and td.deleted_at is null and not td.is_bot
			union
			select $2::uuid where $2::uuid is not null
		) c
		order by c.contact_id
		on conflict (contact_id, thread_id) do update set tx_id = excluded.tx_id`, o.marksTable, o.dialogTable)

	if _, err := tx.Exec(ctx, query, je.JournalThreadID(), extra); err != nil {
		return errors.Internal("marking contact updates", errors.WithCause(err), errors.WithID("postgres.outbox.mark_contacts"))
	}

	return nil
}

// nextUpdateSeq advances thread's update_seq under row lock to stay gap-free.
func (o *outboxStore) nextUpdateSeq(ctx context.Context, tx pgx.Tx, threadID uuid.UUID) (int64, error) {
	query := fmt.Sprintf(`
		update %s
		set last_update_seq = last_update_seq + 1
		where id = $1
		returning last_update_seq`, o.threadTable)

	var seq int64
	if err := tx.QueryRow(ctx, query, threadID).Scan(&seq); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, errors.NotFound("thread not found", errors.WithID("postgres.outbox.next_update_seq"))
		}

		return 0, errors.Internal("stamping update_seq",
			errors.WithCause(err), errors.WithID("postgres.outbox.next_update_seq"))
	}

	return seq, nil
}

func (o *outboxStore) Cleanup(ctx context.Context, opt *model.OutboxCleanupOptions) (int64, error) {
	if opt.BatchSize <= 0 {
		opt.BatchSize = 5000
	}

	args := pgx.NamedArgs{
		"retention_days":  opt.RetentionDays,
		"batch_size":      opt.BatchSize,
		"consumer_groups": opt.ConsumerGroups,
		"topic":           opt.Topic,
	}

	// Delete rows only when all consumer groups have confirmed consumption (frontier = min(group_txid)).
	// Only deletes when all groups registered (count(*) = cardinality) to avoid data loss on deploy.
	query := fmt.Sprintf(`
        WITH offs AS (
            SELECT last_processed_transaction_id AS txid
            FROM %[2]s
            WHERE consumer_group = ANY(@consumer_groups)
              AND topic = @topic
        ),
        frontier AS (
            SELECT txid
            FROM offs
            WHERE (SELECT count(*) FROM offs) = cardinality(@consumer_groups::text[])
            ORDER BY txid ASC
            LIMIT 1
        ),
        to_delete AS (
            SELECT transaction_id, "offset"
            FROM %[1]s
            WHERE transaction_id < (SELECT txid FROM frontier)
              AND created_at < now() - (@retention_days * interval '1 day')
            ORDER BY transaction_id, "offset"
            LIMIT @batch_size
        )
        DELETE FROM %[1]s
        USING to_delete
        WHERE %[1]s.transaction_id = to_delete.transaction_id
          AND %[1]s."offset" = to_delete."offset"`, o.outboxTable, o.offsetsTable)

	var totalDeleted int64

	for {
		result, err := o.q.Exec(ctx, query, args)
		if err != nil {
			return totalDeleted, fmt.Errorf("outbox cleanup: %w", err)
		}

		rowsAffected := result.RowsAffected()
		totalDeleted += rowsAffected

		if rowsAffected < int64(opt.BatchSize) {
			break
		}

		select {
		case <-ctx.Done():
			return totalDeleted, ctx.Err()
		default:
		}
	}

	return totalDeleted, nil
}
