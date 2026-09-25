//go:build integration

package journal

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real journal queries against a throwaway schema:
//
//	POSTGRES_DSN=postgres://... go test -tags=integration ./internal/adapter/journal/ -count=1 -v
const itSchema = "thread_updates_it"

func newTestJournal(t *testing.T, ttl time.Duration, now func() time.Time) *Journal {
	t.Helper()
	ctx := context.Background()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set; skipping journal integration test")
	}

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	for _, s := range []string{
		`DROP SCHEMA IF EXISTS ` + itSchema + ` CASCADE`,
		`CREATE SCHEMA ` + itSchema,
		`CREATE TABLE ` + itSchema + `.thread_updates (
			id         bigserial primary key,
			thread_id  uuid        not null,
			kind       text        not null,
			fields     jsonb       not null default '{}'::jsonb,
			created_at timestamptz not null default clock_timestamp(),
			tx_id      bigint      default (pg_current_xact_id()::text::bigint)
		)`,
		`CREATE TABLE ` + itSchema + `.contact_updates (
			contact_id uuid not null, thread_id uuid not null, tx_id bigint not null,
			primary key (contact_id, thread_id)
		)`,
		`CREATE TABLE ` + itSchema + `.thread_updates_trim (id smallint primary key default 1, tx_id bigint not null)`,
	} {
		_, err := pool.Exec(ctx, s)
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+itSchema+` CASCADE`)
	})

	opts := []Option{
		WithTTL(ttl),
		WithTable(itSchema + ".thread_updates"),
		WithMarksTable(itSchema+".contact_updates", itSchema+".thread_updates_trim"),
	}
	if now != nil {
		opts = append(opts, WithClock(now))
	}

	return New(pool, opts...)
}

func appendMsg(t *testing.T, j *Journal, thread, msgID string) {
	t.Helper()
	require.NoError(t, j.Append(context.Background(), Update{ThreadID: thread, Kind: KindMessageNew, Fields: map[string]any{FieldMsgID: msgID}}))
}

// Only entries written after the cursor's transaction come back, oldest first.
func TestJournal_ChangesSince(t *testing.T) {
	ctx := context.Background()
	j := newTestJournal(t, DefaultTTL, nil)
	thread := uuid.NewString()

	appendMsg(t, j, thread, "m1")
	appendMsg(t, j, thread, "m2")

	cursor, err := j.SettledHorizon(ctx)
	require.NoError(t, err)

	appendMsg(t, j, thread, "m3")
	appendMsg(t, j, thread, "m4")

	horizon, err := j.SettledHorizon(ctx)
	require.NoError(t, err)

	events, err := j.ChangesSince(ctx, thread, cursor, horizon, MaxContactChanges)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "m3", events[0].Fields[FieldMsgID])
	assert.Equal(t, "m4", events[1].Fields[FieldMsgID])

	limited, err := j.ChangesSince(ctx, thread, 0, horizon, 1)
	require.NoError(t, err)
	assert.Len(t, limited, 2, "limit+1 rows so the caller sees the overflow")
}

// A contact sees each changed thread once, from its own marks only.
func TestJournal_ContactChanges(t *testing.T) {
	ctx := context.Background()
	j := newTestJournal(t, DefaultTTL, nil)
	me, other := uuid.NewString(), uuid.NewString()
	t1, t2 := uuid.NewString(), uuid.NewString()

	_, err := j.db.Exec(ctx, `INSERT INTO `+itSchema+`.contact_updates VALUES ($1, $2, 10), ($1, $3, 20), ($4, $2, 30)`, me, t1, t2, other)
	require.NoError(t, err)

	got, err := j.ContactChanges(ctx, me, "10")
	require.NoError(t, err)
	require.Len(t, got.Threads, 1, "t1 was marked at the cursor itself")
	assert.Equal(t, t2, got.Threads[0])
	assert.Equal(t, int64(10), got.After)

	next, err := j.ContactChanges(ctx, me, got.Cursor)
	require.NoError(t, err)
	assert.Empty(t, next.Threads)

	_, err = j.ContactChanges(ctx, me, "abc")
	require.ErrorIs(t, err, ErrInvalidCursor)
}

func TestJournal_ContactChangesTooMany(t *testing.T) {
	ctx := context.Background()
	j := newTestJournal(t, DefaultTTL, nil)
	me := uuid.NewString()

	_, err := j.db.Exec(ctx, `INSERT INTO `+itSchema+`.contact_updates
		SELECT $1, gen_random_uuid(), 1 FROM generate_series(1, $2)`, me, MaxContactChanges+1)
	require.NoError(t, err)

	got, err := j.ContactChanges(ctx, me, "0")
	require.NoError(t, err)
	assert.True(t, got.TooMany)
	assert.Empty(t, got.Threads)
}

// Retention remembers the newest transaction it deleted, so older cursors resync.
func TestJournal_CleanupRecordsTrimHorizon(t *testing.T) {
	ctx := context.Background()
	nowP := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	j := newTestJournal(t, 48*time.Hour, func() time.Time { return nowP })
	thread := uuid.NewString()

	none, err := j.TrimHorizon(ctx)
	require.NoError(t, err)
	assert.Zero(t, none)

	_, err = j.db.Exec(ctx, `INSERT INTO `+itSchema+`.thread_updates (thread_id, kind, fields, created_at, tx_id) VALUES
		($1, 'message.new', '{"msg_id":"m1"}', $2, 111), ($1, 'message.new', '{"msg_id":"m2"}', $2, 222),
		($1, 'message.new', '{"msg_id":"m3"}', $3, 333)`,
		thread, nowP.Add(-5*24*time.Hour), nowP)
	require.NoError(t, err)

	n, err := j.Cleanup(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	trimmed, err := j.TrimHorizon(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(222), trimmed)

	left, err := j.ChangesSince(ctx, thread, 0, 1000, MaxContactChanges)
	require.NoError(t, err)
	require.Len(t, left, 1)
	assert.Equal(t, "m3", left[0].Fields[FieldMsgID])
}
