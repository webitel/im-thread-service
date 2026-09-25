//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration test: exercises REAL per-thread update_seq stamping query against live Postgres; POSTGRES_DSN=... go test -tags=integration -run UpdateSeq ./internal/store/postgres/ -count=1 -v
const updSeqSchema = "update_seq_it"

func setupThreadSchema(t *testing.T, pool *pgxpool.Pool, threadIDs ...uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	stmts := []string{
		`DROP SCHEMA IF EXISTS ` + updSeqSchema + ` CASCADE`,
		`CREATE SCHEMA ` + updSeqSchema,
		`CREATE TABLE ` + updSeqSchema + `.thread (
			id              uuid primary key,
			last_update_seq bigint not null default 0
		)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	for _, id := range threadIDs {
		if _, err := pool.Exec(ctx, `INSERT INTO `+updSeqSchema+`.thread (id) VALUES ($1)`, id); err != nil {
			t.Fatalf("seed thread: %v", err)
		}
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+updSeqSchema+` CASCADE`)
	})
}

func updSeqStore() *outboxStore {
	return &outboxStore{threadTable: updSeqSchema + ".thread"}
}

// The counter advances by exactly 1 per call and is independent per thread.
func TestUpdateSeq_MonotonicPerThread(t *testing.T) {
	ctx := context.Background()
	pool := itPool(t)

	threadA := uuid.New()
	threadB := uuid.New()
	setupThreadSchema(t, pool, threadA, threadB)

	store := updSeqStore()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Two bumps on A → 1, 2; one bump on B → 1 (independent counter).
	for i, want := range []int64{1, 2} {
		got, err := store.nextUpdateSeq(ctx, tx, threadA)
		if err != nil {
			t.Fatalf("A bump %d: %v", i, err)
		}

		if got != want {
			t.Fatalf("A bump %d = %d, want %d", i, got, want)
		}
	}

	got, err := store.nextUpdateSeq(ctx, tx, threadB)
	if err != nil {
		t.Fatalf("B bump: %v", err)
	}

	if got != 1 {
		t.Fatalf("B first bump = %d, want 1", got)
	}
}

// A rolled-back transaction never commits its bump: the next transaction sees
// the counter unchanged, so committed seqs are gap-free.
func TestUpdateSeq_RollbackLeavesNoGap(t *testing.T) {
	ctx := context.Background()
	pool := itPool(t)

	thread := uuid.New()
	setupThreadSchema(t, pool, thread)

	store := updSeqStore()

	// First tx bumps to 1 then rolls back.
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}

	if seq, err := store.nextUpdateSeq(ctx, tx1, thread); err != nil || seq != 1 {
		t.Fatalf("tx1 bump = %d, err %v; want 1", seq, err)
	}

	if err := tx1.Rollback(ctx); err != nil {
		t.Fatalf("rollback tx1: %v", err)
	}

	// Second tx must also get 1 — the rolled-back bump left no gap.
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2: %v", err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()

	if seq, err := store.nextUpdateSeq(ctx, tx2, thread); err != nil || seq != 1 {
		t.Fatalf("tx2 bump = %d, err %v; want 1 (no gap after rollback)", seq, err)
	}
}
