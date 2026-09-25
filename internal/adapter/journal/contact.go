package journal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

const (
	defaultMarksTable = "im_thread.contact_updates"
	defaultTrimTable  = "im_message.thread_updates_trim"
)

// MaxContactChanges caps the changes GetUpdates serves at once; past it the client
// reloads its thread list instead (Telegram's differenceTooLong).
const MaxContactChanges = 1000

// ChangedThread is a thread changed for a contact, with its current head.
type ChangedThread struct {
	ThreadID string
	Head     int64
}

// ContactChanges is every thread changed for a contact since a cursor.
type ContactChanges struct {
	Threads []ChangedThread
	Cursor  string
	// After and Horizon bound the transactions served: (After, Horizon].
	After   int64
	Horizon int64
	// TooMany is set when more than MaxContactChanges threads changed.
	TooMany bool
}

// ContactChanges lists threads changed for a contact after cursor. Only transactions older
// than every in-flight one are served, so a late commit comes on a later call, never skipped.
func (j *Journal) ContactChanges(ctx context.Context, contactID, cursor string) (*ContactChanges, error) {
	horizon, err := j.SettledHorizon(ctx)
	if err != nil {
		return nil, err
	}

	after, err := parseCursor(cursor)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		select cu.thread_id::text, coalesce(t.last_update_seq, 0)
		from %s cu
		left join %s t on t.id = cu.thread_id
		where cu.contact_id = $1::uuid and cu.tx_id > $2 and cu.tx_id <= $3
		order by cu.tx_id, cu.thread_id
		limit $4`, j.marksTable, j.threadTable)

	rows, err := j.db.Query(ctx, query, contactID, after, horizon, MaxContactChanges+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &ContactChanges{Cursor: strconv.FormatInt(max(after, horizon), 10), After: after, Horizon: horizon}

	for rows.Next() {
		var t ChangedThread
		if err := rows.Scan(&t.ThreadID, &t.Head); err != nil {
			return nil, err
		}

		out.Threads = append(out.Threads, t)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(out.Threads) > MaxContactChanges {
		out.TooMany = true
		out.Threads = nil
	}

	return out, nil
}

// SettledHorizon is the newest transaction id older than every in-flight one; a fresh
// client starts GetUpdates from it.
func (j *Journal) SettledHorizon(ctx context.Context) (int64, error) {
	var horizon int64
	if err := j.db.QueryRow(ctx, `select pg_snapshot_xmin(pg_current_snapshot())::text::bigint - 1`).Scan(&horizon); err != nil {
		return 0, err
	}

	return horizon, nil
}

// Unread is the contact's unread counter in a thread; 0 when not a member.
func (j *Journal) Unread(ctx context.Context, threadID, contactID string) (int64, error) {
	const query = `
		select coalesce(max(unread_count), 0)
		from im_thread.thread_dialog
		where thread_id = $1::uuid and member_id = $2::uuid and deleted_at is null`

	var n int64
	if err := j.db.QueryRow(ctx, query, threadID, contactID).Scan(&n); err != nil {
		return 0, err
	}

	return n, nil
}

// ChangesSince returns a thread's journal entries written by transactions in (after, horizon],
// oldest first, at most limit+1 so the caller can tell it overflowed.
func (j *Journal) ChangesSince(ctx context.Context, threadID string, after, horizon int64, limit int) ([]Event, error) {
	query := fmt.Sprintf(`
		select update_seq, kind, fields
		from %s
		where thread_id = $1 and tx_id > $2 and tx_id <= $3
		order by update_seq
		limit $4`, j.table)

	rows, err := j.db.Query(ctx, query, threadID, after, horizon, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event

	for rows.Next() {
		var (
			seq    int64
			kind   string
			raw    []byte
			fields = make(map[string]string)
		)

		if err := rows.Scan(&seq, &kind, &raw); err != nil {
			return nil, err
		}

		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &fields); err != nil {
				return nil, err
			}
		}

		out = append(out, Event{Cursor: strconv.FormatInt(seq, 10), Kind: kind, Fields: fields})
	}

	return out, rows.Err()
}

// TrimHorizon is the newest transaction whose journal rows retention deleted; 0 if none.
func (j *Journal) TrimHorizon(ctx context.Context) (int64, error) {
	var tx int64

	query := fmt.Sprintf(`select coalesce(max(tx_id), 0) from %s`, j.trimTable)
	if err := j.db.QueryRow(ctx, query).Scan(&tx); err != nil {
		return 0, err
	}

	return tx, nil
}

// SettledUpTo is the highest update_seq of a thread whose whole prefix is served by windows
// up to horizon. Seqs commit in order per thread, but a later seq can have an older
// transaction id, so a row with tx_id past horizon caps the prefix just below it.
func (j *Journal) SettledUpTo(ctx context.Context, threadID string, horizon int64) (int64, error) {
	query := fmt.Sprintf(`
		select coalesce(
			(select min(update_seq) - 1 from %[1]s where thread_id = $1 and tx_id > $2),
			(select last_update_seq from %[2]s where id = $1::uuid),
			0)`, j.table, j.threadTable)

	var seq int64
	if err := j.db.QueryRow(ctx, query, threadID, horizon).Scan(&seq); err != nil {
		return 0, err
	}

	return seq, nil
}
