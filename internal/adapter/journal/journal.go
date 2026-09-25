package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrInvalidCursor marks a malformed client cursor (InvalidArgument, not Internal).
var ErrInvalidCursor = errors.New("invalid journal cursor")

const defaultTable = "im_message.thread_updates"

// DefaultTTL bounds the catch-up window; older cursors resync.
const DefaultTTL = 48 * time.Hour

const cleanupBatchSize = 5000

// DB is satisfied by *pgxpool.Pool and pgx.Tx.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Journal is the per-thread update log; rows are served to contacts by transaction id.
type Journal struct {
	db         DB
	table      string
	marksTable string
	trimTable  string
	ttl        time.Duration
	now        func() time.Time
}

type Option func(*Journal)

func WithTTL(d time.Duration) Option {
	return func(j *Journal) {
		if d > 0 {
			j.ttl = d
		}
	}
}

// WithTable points the journal at another table (tests use throwaway schemas).
func WithTable(journalTable string) Option {
	return func(j *Journal) {
		if journalTable != "" {
			j.table = journalTable
		}
	}
}

// WithMarksTable points contact change lookups at other tables (tests use throwaway schemas).
func WithMarksTable(marks, trim string) Option {
	return func(j *Journal) {
		if marks != "" {
			j.marksTable = marks
		}

		if trim != "" {
			j.trimTable = trim
		}
	}
}

func WithClock(now func() time.Time) Option {
	return func(j *Journal) {
		if now != nil {
			j.now = now
		}
	}
}

func New(db DB, opts ...Option) *Journal {
	j := &Journal{db: db, table: defaultTable, marksTable: defaultMarksTable, trimTable: defaultTrimTable, ttl: DefaultTTL, now: time.Now}
	for _, o := range opts {
		o(j)
	}

	return j
}

// Update is one projected mutation.
type Update struct {
	ThreadID string
	Kind     string
	Fields   map[string]any
}

// Append writes one entry in the mutation's transaction; tx_id defaults to it.
func (j *Journal) Append(ctx context.Context, u Update) error {
	fields, err := json.Marshal(u.Fields)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`
		insert into %s (thread_id, kind, fields)
		values ($1, $2, $3)`, j.table)

	_, err = j.db.Exec(ctx, query, u.ThreadID, u.Kind, fields)

	return err
}

// Event is one journal entry served to a catching-up client.
type Event struct {
	Kind   string            `json:"kind"`
	Fields map[string]string `json:"fields"`
}

// IsMember gates catch-up to current members, scoped to the caller's domain so a
// member id cannot match another tenant's thread.
func (j *Journal) IsMember(ctx context.Context, threadID, memberID string, domainID int32) (bool, error) {
	const query = `
		select exists (
			select 1
			from im_thread.thread_dialog
			where thread_id = $1::uuid
			  and member_id = $2::uuid
			  and deleted_at is null
			  and domain_id = $3
		)`

	var ok bool
	if err := j.db.QueryRow(ctx, query, threadID, memberID, domainID).Scan(&ok); err != nil {
		return false, err
	}

	return ok, nil
}

// ReadState is one member's current delivery/read horizon in a thread, as
// per-thread message seq (0 = nothing reached that state yet).
type ReadState struct {
	MemberID         string
	DeliveredUpToSeq int64
	ReadUpToSeq      int64
}

// ReadStates is a snapshot of per-member delivered/read horizons (monotonic, so
// clients apply it directly); members at neither are omitted.
func (j *Journal) ReadStates(ctx context.Context, threadID string) ([]ReadState, error) {
	// Seq-unit horizons advanced by the Mark* receipts; *_message_id columns are legacy.
	const query = `
		select td.member_id::text,
		       greatest(coalesce(td.last_delivered_seq, 0), coalesce(td.last_read_seq, 0)) as delivered_seq,
		       coalesce(td.last_read_seq, 0) as read_seq
		from im_thread.thread_dialog td
		where td.thread_id = $1::uuid
		  and td.deleted_at is null
		  and (td.last_delivered_seq is not null or td.last_read_seq is not null)
		order by td.member_id`

	rows, err := j.db.Query(ctx, query, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ReadState, 0)

	for rows.Next() {
		var rs ReadState
		if err := rows.Scan(&rs.MemberID, &rs.DeliveredUpToSeq, &rs.ReadUpToSeq); err != nil {
			return nil, err
		}

		out = append(out, rs)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// Member is one thread participant as referenced by journal entries.
type Member struct {
	ID        string
	ContactID string
	Role      int32
	IsBot     bool
}

// Members returns the latest membership row per contact in the thread, left members
// included, so entries they authored before leaving still resolve.
func (j *Journal) Members(ctx context.Context, threadID string) ([]Member, error) {
	const query = `
		select distinct on (td.member_id)
		       td.id::text, td.member_id::text, td.thread_role, td.is_bot
		from im_thread.thread_dialog td
		where td.thread_id = $1::uuid
		order by td.member_id, td.created_at desc`

	rows, err := j.db.Query(ctx, query, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Member, 0)

	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.ContactID, &m.Role, &m.IsBot); err != nil {
			return nil, err
		}

		out = append(out, m)
	}

	return out, rows.Err()
}

// Cleanup deletes entries past the TTL in batches. Trimming a quiet thread empty is
// safe: its head stays in thread and the next row is written with the next bump.
func (j *Journal) Cleanup(ctx context.Context) (int64, error) {
	cutoff := j.now().Add(-j.ttl)

	// Record the newest transaction deleted, so GetUpdates can tell a cursor that
	// predates the trim (resync) from one it can still serve.
	query := fmt.Sprintf(`
		with deleted as (
			delete from %[1]s
			where id in (
				select id
				from %[1]s
				where created_at < $1
				order by id asc
				limit $2
			)
			returning tx_id
		), mark as (
			insert into %[2]s (id, tx_id)
			select 1, max(tx_id) from deleted having max(tx_id) is not null
			on conflict (id) do update set tx_id = greatest(%[2]s.tx_id, excluded.tx_id)
		)
		select count(*) from deleted`, j.table, j.trimTable)

	var total int64

	for {
		var n int64
		if err := j.db.QueryRow(ctx, query, cutoff, cleanupBatchSize).Scan(&n); err != nil {
			return total, err
		}

		total += n

		if n < cleanupBatchSize {
			cleanedUp.Add(ctx, total)

			return total, nil
		}
	}
}

func parseCursor(cursor string) (int64, error) {
	seq, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidCursor, cursor)
	}

	return seq, nil
}
