//go:build integration

package postgres

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

// Integration test: exercises REAL cleanup query against live Postgres; POSTGRES_DSN=... go test -tags=integration -run Cleanup ./internal/store/postgres/ -count=1 -v
const itSchema = "catchup_cleanup_it"

func itPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set; skipping cleanup integration test")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func setupCleanupSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	stmts := []string{
		`DROP SCHEMA IF EXISTS ` + itSchema + ` CASCADE`,
		`CREATE SCHEMA ` + itSchema,
		`CREATE TABLE ` + itSchema + `.messages_outbox (
			"offset"       bigint,
			transaction_id xid8,
			created_at     timestamptz
		)`,
		`CREATE TABLE ` + itSchema + `.messages_offsets (
			consumer_group                text,
			topic                         text,
			last_processed_transaction_id xid8,
			offset_acked                  bigint
		)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+itSchema+` CASCADE`)
	})
}

func remainingOffsets(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()

	rows, err := pool.Query(context.Background(),
		`SELECT "offset" FROM `+itSchema+`.messages_outbox ORDER BY "offset"`)
	if err != nil {
		t.Fatalf("select remaining: %v", err)
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var o int64
		if err := rows.Scan(&o); err != nil {
			t.Fatalf("scan: %v", err)
		}

		out = append(out, o)
	}

	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })

	return out
}

func newITStore(pool *pgxpool.Pool) *outboxStore {
	return &outboxStore{
		q:            pool,
		outboxTable:  itSchema + ".messages_outbox",
		offsetsTable: itSchema + ".messages_offsets",
	}
}

// Deletes only rows strictly below the slowest group's committed frontier AND
// older than retention; keeps boundary-txid, above-frontier, and fresh rows.
func TestCleanup_TwoGroups_DeletesOnlyBelowSlowestFrontier(t *testing.T) {
	ctx := context.Background()
	pool := itPool(t)
	setupCleanupSchema(t, pool)

	const topic = "im.messages"

	// fwd ahead (200), journal behind (150) → frontier = min = 150.
	if _, err := pool.Exec(ctx, `INSERT INTO `+itSchema+`.messages_offsets
		(consumer_group, topic, last_processed_transaction_id, offset_acked) VALUES
		('fwd', $1, '200'::xid8, 200),
		('jrn', $1, '150'::xid8, 150)`, topic); err != nil {
		t.Fatalf("seed offsets: %v", err)
	}

	// Rows 1-2: old & below frontier → DELETE. Rows 3-5: above frontier or fresh → KEEP.
	if _, err := pool.Exec(ctx, `INSERT INTO `+itSchema+`.messages_outbox
		("offset", transaction_id, created_at) VALUES
		(1, '100'::xid8, now() - interval '10 days'),
		(2, '140'::xid8, now() - interval '10 days'),
		(3, '160'::xid8, now() - interval '10 days'),
		(4, '100'::xid8, now()),
		(5, '150'::xid8, now() - interval '10 days')`); err != nil {
		t.Fatalf("seed outbox: %v", err)
	}

	n, err := newITStore(pool).Cleanup(ctx, &model.OutboxCleanupOptions{
		RetentionDays:  3,
		BatchSize:      100,
		ConsumerGroups: []string{"fwd", "jrn"},
		Topic:          topic,
	})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if n != 2 {
		t.Fatalf("want 2 deleted, got %d", n)
	}

	got := remainingOffsets(t, pool)
	want := []int64{3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("remaining = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("remaining = %v, want %v", got, want)
		}
	}
}

// Missing group: cleanup deletes nothing (never outrun an unregistered group).
func TestCleanup_MissingGroup_DeletesNothing(t *testing.T) {
	ctx := context.Background()
	pool := itPool(t)
	setupCleanupSchema(t, pool)

	const topic = "im.messages"

	// Only the forwarder has an offset row; the journal group is absent.
	if _, err := pool.Exec(ctx, `INSERT INTO `+itSchema+`.messages_offsets
		(consumer_group, topic, last_processed_transaction_id, offset_acked)
		VALUES ('fwd', $1, '200'::xid8, 200)`, topic); err != nil {
		t.Fatalf("seed offsets: %v", err)
	}

	// All rows below fwd's frontier and old — would be deleted if the guard
	// were missing.
	if _, err := pool.Exec(ctx, `INSERT INTO `+itSchema+`.messages_outbox
		("offset", transaction_id, created_at) VALUES
		(1, '10'::xid8,  now() - interval '10 days'),
		(2, '20'::xid8,  now() - interval '10 days'),
		(3, '30'::xid8,  now() - interval '10 days')`); err != nil {
		t.Fatalf("seed outbox: %v", err)
	}

	n, err := newITStore(pool).Cleanup(ctx, &model.OutboxCleanupOptions{
		RetentionDays:  3,
		BatchSize:      100,
		ConsumerGroups: []string{"fwd", "jrn"},
		Topic:          topic,
	})
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	if n != 0 {
		t.Fatalf("missing group must delete nothing, deleted %d", n)
	}

	if got := remainingOffsets(t, pool); len(got) != 3 {
		t.Fatalf("want all 3 rows retained, got %v", got)
	}
}
