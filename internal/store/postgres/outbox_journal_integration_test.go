//go:build integration

package postgres

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/webitel/im-thread-service/internal/adapter/journal"
	"github.com/webitel/im-thread-service/internal/domain/event"
)

func setupJournalSchema(t *testing.T, pool *pgxpool.Pool, thread uuid.UUID) {
	t.Helper()
	setupThreadSchema(t, pool, thread)

	for _, q := range []string{
		`CREATE TABLE ` + updSeqSchema + `.thread_updates (
			id         bigserial primary key,
			thread_id  uuid        not null,
			update_seq bigint      not null,
			kind       text        not null,
			fields     jsonb       not null default '{}'::jsonb,
			created_at timestamptz not null default clock_timestamp(),
			tx_id      bigint      default (pg_current_xact_id()::text::bigint)
		)`,
		`CREATE UNIQUE INDEX ON ` + updSeqSchema + `.thread_updates (thread_id, update_seq)`,
		`CREATE TABLE ` + updSeqSchema + `.thread_dialog (
			thread_id uuid not null, member_id uuid not null,
			is_bot boolean not null default false, deleted_at timestamptz
		)`,
		`CREATE TABLE ` + updSeqSchema + `.contact_updates (
			contact_id uuid not null, thread_id uuid not null, tx_id bigint not null,
			primary key (contact_id, thread_id)
		)`,
		`CREATE TABLE ` + updSeqSchema + `.thread_updates_trim (id smallint primary key default 1, tx_id bigint not null)`,
		`CREATE TABLE ` + updSeqSchema + `.messages_outbox (
			"offset"         bigserial,
			"uuid"           varchar(36) not null,
			"created_at"     timestamp   not null default current_timestamp,
			"payload"        json        default null,
			"metadata"       json        default null,
			"transaction_id" xid8        not null,
			primary key ("transaction_id", "offset")
		)`,
	} {
		if _, err := pool.Exec(context.Background(), q); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
}

// itOutboxStore is the production store bound to one tx, pointed at the test schema.
func itOutboxStore(q Querier) *outboxStore {
	return &outboxStore{
		q:            q,
		wmlogger:     watermill.NopLogger{},
		threadTable:  updSeqSchema + ".thread",
		journalTable: updSeqSchema + ".thread_updates",
		marksTable:   updSeqSchema + ".contact_updates",
		dialogTable:  updSeqSchema + ".thread_dialog",
		config: sql.PublisherConfig{SchemaAdapter: sql.DefaultPostgreSQLSchema{
			GenerateMessagesTableName: func(string) string { return updSeqSchema + ".messages_outbox" },
		}},
	}
}

func itJournal(pool *pgxpool.Pool) *journal.Journal {
	return journal.New(pool,
		journal.WithTables(updSeqSchema+".thread_updates", updSeqSchema+".thread"),
		journal.WithMarksTable(updSeqSchema+".contact_updates", updSeqSchema+".thread_updates_trim"))
}

func publishOnce(ctx context.Context, pool *pgxpool.Pool, thread uuid.UUID) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	e := &event.MessageCreated{MessageID: uuid.New(), ThreadID: thread, Body: "hi"}
	if err := itOutboxStore(tx).Publish(ctx, "t", e); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// K writers call the real Publish while a reader keeps reading settled windows: every seq
// arrives exactly once, and the settled cursor never claims a seq the reader has not seen.
func TestPublish_ConcurrentWritersAndReaderMissNothing(t *testing.T) {
	const writers = 50

	ctx := context.Background()
	pool := itPool(t)
	thread := uuid.New()
	setupJournalSchema(t, pool, thread)

	j := itJournal(pool)

	start, err := j.SettledHorizon(ctx)
	if err != nil {
		t.Fatalf("horizon: %v", err)
	}

	var (
		writersDone atomic.Bool
		readerErr   error
		reader      sync.WaitGroup
	)

	readCtx, cancelRead := context.WithTimeout(ctx, 30*time.Second)
	defer cancelRead()

	reader.Add(1)

	go func() {
		defer reader.Done()

		after := start
		seen := make(map[int64]bool, writers)

		for {
			finished := writersDone.Load()

			horizon, err := j.SettledHorizon(readCtx)
			if err != nil {
				readerErr = err

				return
			}

			events, err := j.ChangesSince(readCtx, thread.String(), after, horizon, writers)
			if err != nil {
				readerErr = err

				return
			}

			for _, e := range events {
				seq, _ := strconv.ParseInt(e.Cursor, 10, 64)
				if seen[seq] {
					readerErr = errGap(seq, seq)

					return
				}

				seen[seq] = true
			}

			settled, err := j.SettledUpTo(readCtx, thread.String(), horizon)
			if err != nil {
				readerErr = err

				return
			}

			for s := int64(1); s <= settled; s++ {
				if !seen[s] {
					readerErr = errGap(settled, s)

					return
				}
			}

			after = max(after, horizon)

			if finished && len(seen) == writers {
				return
			}
		}
	}()

	var wg sync.WaitGroup

	errs := make(chan error, writers)

	for range writers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			errs <- publishOnce(ctx, pool, thread)
		}()
	}

	wg.Wait()
	close(errs)
	writersDone.Store(true)

	for err := range errs {
		if err != nil {
			t.Fatalf("writer: %v", err)
		}
	}

	reader.Wait()

	if readerErr != nil {
		t.Fatalf("reader: %v", readerErr)
	}
}

// A rolled-back mutation leaves no seq, journal row, outbox row or contact mark.
func TestPublish_RollbackLeavesNoTrace(t *testing.T) {
	ctx := context.Background()
	pool := itPool(t)
	thread, member := uuid.New(), uuid.New()
	setupJournalSchema(t, pool, thread)

	if _, err := pool.Exec(ctx, `INSERT INTO `+updSeqSchema+`.thread_dialog (thread_id, member_id) VALUES ($1, $2)`, thread, member); err != nil {
		t.Fatalf("seed member: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	if err := itOutboxStore(tx).Publish(ctx, "t", &event.MessageCreated{MessageID: uuid.New(), ThreadID: thread}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	_ = tx.Rollback(ctx)

	for table, want := range map[string]int{"thread_updates": 0, "messages_outbox": 0, "contact_updates": 0} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+updSeqSchema+`.`+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}

		if n != want {
			t.Fatalf("%s rows = %d after rollback, want %d", table, n, want)
		}
	}
}

// Every human member of a changed thread sees it in ContactChanges; bots and strangers do not.
func TestPublish_MarksContactsForGetUpdates(t *testing.T) {
	ctx := context.Background()
	pool := itPool(t)
	thread := uuid.New()
	setupJournalSchema(t, pool, thread)

	human, bot, stranger := uuid.New(), uuid.New(), uuid.New()

	if _, err := pool.Exec(ctx, `INSERT INTO `+updSeqSchema+`.thread_dialog (thread_id, member_id, is_bot) VALUES ($1, $2, false), ($1, $3, true)`,
		thread, human, bot); err != nil {
		t.Fatalf("seed members: %v", err)
	}

	j := itJournal(pool)

	start, err := j.SettledHorizon(ctx)
	if err != nil {
		t.Fatalf("horizon: %v", err)
	}

	for range 2 {
		if err := publishOnce(ctx, pool, thread); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	cursor := strconv.FormatInt(start, 10)

	got, err := j.ContactChanges(ctx, human.String(), cursor)
	if err != nil {
		t.Fatalf("human changes: %v", err)
	}

	if len(got.Threads) != 1 || got.Threads[0].ThreadID != thread.String() || got.Threads[0].Head != 2 {
		t.Fatalf("human: want thread with head 2 once, got %+v", got.Threads)
	}

	events, err := j.ChangesSince(ctx, thread.String(), got.After, got.Horizon, journal.MaxContactChanges)
	if err != nil || len(events) != 2 {
		t.Fatalf("human: want both changes, got %d %v", len(events), err)
	}

	for name, c := range map[string]uuid.UUID{"bot": bot, "stranger": stranger} {
		other, err := j.ContactChanges(ctx, c.String(), cursor)
		if err != nil || len(other.Threads) != 0 {
			t.Fatalf("%s must not see the thread, got %+v %v", name, other, err)
		}
	}
}

type gapError struct{ settled, seq int64 }

func (e gapError) Error() string {
	return "seq " + strconv.FormatInt(e.seq, 10) + " duplicated or missing below settled " + strconv.FormatInt(e.settled, 10)
}

func errGap(settled, seq int64) error { return gapError{settled: settled, seq: seq} }
