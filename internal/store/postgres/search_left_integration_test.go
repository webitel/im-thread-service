//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

// A left thread's preview skips deleted messages, retried originals included.
//
//	POSTGRES_DSN=postgres://... go test -tags=integration -run SearchLeft ./internal/store/postgres/ -count=1 -v
func TestSearchLeft_LastMessageSkipsDeleted_Integration(t *testing.T) {
	tx := rolledBackTx(t)
	ctx := context.Background()

	member, other := uuid.New(), uuid.New()
	th := newITThread(t, tx, member, other)

	joined := time.Now().Add(-time.Hour)
	left := joined.Add(30 * time.Minute)

	_, err := tx.Exec(ctx, `update im_thread.thread_dialog set created_at = $2, deleted_at = $3 where id = $1`,
		th.member[member], joined, left)
	require.NoError(t, err)

	insert := func(at time.Time, deleted bool) uuid.UUID {
		th.seq++
		id := uuid.New()

		var deletedAt *time.Time
		if deleted {
			deletedAt = &left
		}

		_, err := tx.Exec(ctx, `
			insert into im_message.messages (id, domain_id, thread_id, sender_id, member_id, type, body, seq, created_at, deleted_at)
			values ($1, $2, $3, $4, $5, 1, 'synthetic', $6, $7, $8)`,
			id, itDomainID, th.id, other, th.member[other], th.seq, at, deletedAt)
		require.NoError(t, err)

		return id
	}

	live := insert(joined.Add(5*time.Minute), false)
	insert(joined.Add(10*time.Minute), true)

	threads, err := NewThreadStore(tx).SearchLeft(ctx,
		queryobject.NewSearchLeftQueryObject(member).
			WithFields([]string{"id", "last_msg"}).
			WithDomainIDFilter(int(itDomainID)))
	require.NoError(t, err)
	require.Len(t, threads, 1)
	require.NotNil(t, threads[0].LastMessage, "the live message is the preview")
	require.Equal(t, live, threads[0].LastMessage.ID)
}
