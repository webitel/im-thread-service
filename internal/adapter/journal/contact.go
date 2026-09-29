package journal

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMarksTable = "im_thread.contact_updates"
	defaultTrimTable  = "im_message.thread_updates_trim"
)

// MaxContactChanges caps the changes GetUpdates serves at once; past it the client
// reloads its thread list instead (Telegram's differenceTooLong).
const MaxContactChanges = 1000

// LiveOverlap is how far back GetUpdates replays for a cursor taken from a live event:
// events reach the socket through per-type queues, so an earlier change can arrive later.
const LiveOverlap = time.Minute

// LiveCursor is the cursor a live event carries: the position before its transaction and
// the database time, so GetUpdates can replay the reorder overlap.
func LiveCursor(after, unixMs int64) string {
	return strconv.FormatInt(after, 10) + "." + strconv.FormatInt(unixMs, 10)
}

// ContactChanges is every thread changed for a contact since a cursor.
type ContactChanges struct {
	Threads []string
	Cursor  string
	// After and Horizon bound the transactions served: (After, Horizon]. A live-event
	// cursor also replays everything written since Since (zero for an exact cursor).
	After   int64
	Since   time.Time
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

	after, since, err := parseCursor(cursor)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		select thread_id::text
		from %s
		where contact_id = $1::uuid and tx_id <= $3
		  and (tx_id > $2 or updated_at >= $5::timestamptz)
		order by tx_id, thread_id
		limit $4`, j.marksTable)

	rows, err := j.db.Query(ctx, query, contactID, after, horizon, MaxContactChanges+1, sinceArg(since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &ContactChanges{Cursor: strconv.FormatInt(max(after, horizon), 10), After: after, Since: since, Horizon: horizon}

	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
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

// ChangesSince returns a thread's journal entries in the contact's window (see ContactChanges),
// oldest first, at most limit+1 so the caller can tell it overflowed.
func (j *Journal) ChangesSince(ctx context.Context, threadID string, w *ContactChanges, limit int) ([]Event, error) {
	query := fmt.Sprintf(`
		select kind, fields
		from %s
		where thread_id = $1 and tx_id <= $3
		  and (tx_id > $2 or created_at >= $5::timestamptz)
		order by tx_id, id
		limit $4`, j.table)

	rows, err := j.db.Query(ctx, query, threadID, w.After, w.Horizon, limit+1, sinceArg(w.Since))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event

	for rows.Next() {
		var (
			kind   string
			raw    []byte
			fields = make(map[string]string)
		)

		if err := rows.Scan(&kind, &raw); err != nil {
			return nil, err
		}

		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &fields); err != nil {
				return nil, err
			}
		}

		out = append(out, Event{Kind: kind, Fields: fields})
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

// sinceArg is the overlap bound for SQL; an exact cursor replays nothing extra.
func sinceArg(since time.Time) any {
	if since.IsZero() {
		return nil
	}

	return since
}

// parseCursor reads "<tx>" (exact, from an API response) or "<tx>.<unix ms>" (from a live event).
func parseCursor(cursor string) (int64, time.Time, error) {
	txPart, msPart, live := strings.Cut(cursor, ".")

	after, err := strconv.ParseInt(txPart, 10, 64)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidCursor, cursor)
	}

	if !live {
		return after, time.Time{}, nil
	}

	ms, err := strconv.ParseInt(msPart, 10, 64)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("%w: %q", ErrInvalidCursor, cursor)
	}

	return after, time.UnixMilli(ms).Add(-LiveOverlap), nil
}
