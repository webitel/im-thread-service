//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

// Integration tests for the retry / failure queries against the real im_*
// schema. Everything runs in one transaction that is always rolled back, so a
// shared database is left untouched; all data is synthetic.
//
//	POSTGRES_DSN=postgres://... go test -tags=integration -run 'Retry|RecoveredErrors|UnreportedFailed' ./internal/store/postgres/ -count=1 -v

// itDomainID is a synthetic tenant no real data uses.
const itDomainID int32 = 2147480001

func rolledBackTx(t *testing.T) pgx.Tx {
	t.Helper()

	pool := itPool(t)

	tx, err := pool.Begin(context.Background())
	require.NoError(t, err)

	// Registered after itPool's Close, so it runs first.
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rollback: %v", err)
		}
	})

	return tx
}

type itThread struct {
	tx     pgx.Tx
	id     uuid.UUID
	seq    int64
	member map[uuid.UUID]uuid.UUID // contact -> thread_dialog.id
}

func newITThread(t *testing.T, tx pgx.Tx, contacts ...uuid.UUID) *itThread {
	t.Helper()

	ctx := context.Background()
	th := &itThread{tx: tx, id: uuid.New(), member: make(map[uuid.UUID]uuid.UUID)}

	_, err := tx.Exec(ctx, `insert into im_thread.thread (id, domain_id, kind) values ($1, $2, 0)`, th.id, itDomainID)
	require.NoError(t, err)

	for _, c := range contacts {
		dialogID := uuid.New()
		_, err = tx.Exec(ctx, `insert into im_thread.thread_dialog (id, domain_id, thread_id, member_id) values ($1, $2, $3, $4)`,
			dialogID, itDomainID, th.id, c)
		require.NoError(t, err)

		th.member[c] = dialogID
	}

	return th
}

// message inserts a message as sender; origin is the real author of a send_as message.
func (th *itThread) message(t *testing.T, typ model.MessageType, sender uuid.UUID, origin *uuid.UUID) (uuid.UUID, int64) {
	t.Helper()

	th.seq++
	id := uuid.New()

	_, err := th.tx.Exec(context.Background(), `
		insert into im_message.messages (id, domain_id, thread_id, sender_id, origin_sender, member_id, type, body, seq)
		values ($1, $2, $3, $4, $5, $6, $7, 'synthetic', $8)`,
		id, itDomainID, th.id, sender, origin, th.member[sender], int16(typ), th.seq)
	require.NoError(t, err)

	return id, th.seq
}

func (th *itThread) fail(t *testing.T, messageID, member uuid.UUID, code string) {
	t.Helper()

	_, err := th.tx.Exec(context.Background(), `
		insert into im_message.message_errors (domain_id, thread_id, message_id, member_id, failed_at, error, via)
		values ($1, $2, $3, $4, now(), jsonb_build_object('code', $5::text), 'provider')`,
		itDomainID, th.id, messageID, member, code)
	require.NoError(t, err)
}

func (th *itThread) errorCodes(t *testing.T, member uuid.UUID) map[uuid.UUID]string {
	t.Helper()

	rows, err := th.tx.Query(context.Background(),
		`select message_id, error->>'code' from im_message.message_errors where thread_id = $1 and member_id = $2`, th.id, member)
	require.NoError(t, err)

	out := make(map[uuid.UUID]string)

	for rows.Next() {
		var (
			id   uuid.UUID
			code string
		)

		require.NoError(t, rows.Scan(&id, &code))
		out[id] = code
	}

	require.NoError(t, rows.Err())

	return out
}

func requireNotRetryable(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, errors.Code(err), "%v", err)
}

func TestReplaceFailedMessage_Integration(t *testing.T) {
	tx := rolledBackTx(t)
	store := NewMessageStore(tx)
	ctx := context.Background()

	author, recipient, stranger := uuid.New(), uuid.New(), uuid.New()

	// newFailed is a failed text of author in a fresh thread plus the replacement row the
	// retries table must reference.
	newFailed := func(t *testing.T) (*itThread, uuid.UUID, uuid.UUID) {
		t.Helper()

		th := newITThread(t, tx, author, recipient, stranger)
		original, _ := th.message(t, model.MessageTypeText, author, nil)
		th.fail(t, original, recipient, model.DeliveryErrorSendFailed)
		replacement, _ := th.message(t, model.MessageTypeText, author, nil)

		return th, original, replacement
	}

	t.Run("own failed message is replaced", func(t *testing.T) {
		th, original, replacement := newFailed(t)

		got, err := store.ReplaceFailedMessage(ctx, original, replacement, th.id, author)
		require.NoError(t, err)
		assert.Equal(t, original, got.ID)
		assert.NotNil(t, got.DeletedAt)

		var deletedBy, retriedBy uuid.UUID
		require.NoError(t, tx.QueryRow(ctx, `select deleted_by from im_message.messages where id = $1`, original).Scan(&deletedBy))
		require.NoError(t, tx.QueryRow(ctx, `select retried_by from im_message.message_retries where message_id = $1`, original).Scan(&retriedBy))
		assert.Equal(t, author, deletedBy)
		assert.Equal(t, replacement, retriedBy)

		target, err := store.GetRetryTarget(ctx, original, itDomainID)
		require.NoError(t, err)
		require.NotNil(t, target.RetriedBy)
		assert.True(t, target.Deleted)
		assert.Equal(t, replacement, *target.RetriedBy)
		assert.Equal(t, author, *target.RetriedByAuthor)
		assert.Equal(t, th.id, *target.RetriedByThread)

		t.Run("a second retry of it is rejected", func(t *testing.T) {
			another, _ := th.message(t, model.MessageTypeText, author, nil)

			_, err := store.ReplaceFailedMessage(ctx, original, another, th.id, author)
			requireNotRetryable(t, err)
		})
	})

	t.Run("a message that did not fail is rejected", func(t *testing.T) {
		th := newITThread(t, tx, author, recipient)
		original, _ := th.message(t, model.MessageTypeText, author, nil)
		replacement, _ := th.message(t, model.MessageTypeText, author, nil)

		_, err := store.ReplaceFailedMessage(ctx, original, replacement, th.id, author)
		requireNotRetryable(t, err)
	})

	t.Run("another member's failed message is rejected", func(t *testing.T) {
		th, original, replacement := newFailed(t)

		_, err := store.ReplaceFailedMessage(ctx, original, replacement, th.id, stranger)
		requireNotRetryable(t, err)
	})

	t.Run("a retry in another thread is rejected", func(t *testing.T) {
		_, original, replacement := newFailed(t)
		other := newITThread(t, tx, author)

		_, err := store.ReplaceFailedMessage(ctx, original, replacement, other.id, author)
		requireNotRetryable(t, err)
	})

	t.Run("a deleted message is rejected", func(t *testing.T) {
		th, original, replacement := newFailed(t)

		_, err := tx.Exec(ctx, `update im_message.messages set deleted_at = now(), deleted_by = $2 where id = $1`, original, author)
		require.NoError(t, err)

		_, err = store.ReplaceFailedMessage(ctx, original, replacement, th.id, author)
		requireNotRetryable(t, err)
	})

	t.Run("a failed message of a type without retry_of is rejected", func(t *testing.T) {
		th := newITThread(t, tx, author, recipient)
		original, _ := th.message(t, model.MessageTypeInteractive, author, nil)
		th.fail(t, original, recipient, model.DeliveryErrorSendFailed)
		replacement, _ := th.message(t, model.MessageTypeText, author, nil)

		_, err := store.ReplaceFailedMessage(ctx, original, replacement, th.id, author)
		requireNotRetryable(t, err)

		target, err := store.GetRetryTarget(ctx, original, itDomainID)
		require.NoError(t, err)
		assert.False(t, target.Failed)
	})

	t.Run("send_as: only the agent who really sent it may retry", func(t *testing.T) {
		shared, agentA, agentB := uuid.New(), uuid.New(), uuid.New()

		th := newITThread(t, tx, shared, recipient)
		original, _ := th.message(t, model.MessageTypeText, shared, &agentA)
		th.fail(t, original, recipient, model.DeliveryErrorSendFailed)
		replacement, _ := th.message(t, model.MessageTypeText, shared, &agentA)

		target, err := store.GetRetryTarget(ctx, original, itDomainID)
		require.NoError(t, err)
		assert.Equal(t, agentA, target.AuthorID, "the real author, not the shared identity")

		_, err = store.ReplaceFailedMessage(ctx, original, replacement, th.id, agentB)
		requireNotRetryable(t, err)

		_, err = store.ReplaceFailedMessage(ctx, original, replacement, th.id, shared)
		requireNotRetryable(t, err)

		_, err = store.ReplaceFailedMessage(ctx, original, replacement, th.id, agentA)
		require.NoError(t, err)

		var deletedBy uuid.UUID
		require.NoError(t, tx.QueryRow(ctx, `select deleted_by from im_message.messages where id = $1`, original).Scan(&deletedBy))
		assert.Equal(t, agentA, deletedBy, "deleted_by is the real actor, like an ordinary delete")
	})

	t.Run("unknown or foreign-domain message has no retry target", func(t *testing.T) {
		_, original, _ := newFailed(t)

		target, err := store.GetRetryTarget(ctx, uuid.New(), itDomainID)
		require.NoError(t, err)
		assert.Nil(t, target)

		target, err = store.GetRetryTarget(ctx, original, itDomainID-1)
		require.NoError(t, err)
		assert.Nil(t, target)
	})
}

// A replaced original disappears from every history-like reader instead of
// turning into a "deleted" tombstone; an ordinary delete still does.
func TestReplacedOriginalIsHidden_Integration(t *testing.T) {
	tx := rolledBackTx(t)
	ctx := context.Background()
	messages := NewMessageStore(tx)

	author, recipient := uuid.New(), uuid.New()
	th := newITThread(t, tx, author, recipient)

	original, _ := th.message(t, model.MessageTypeText, author, nil)
	th.fail(t, original, recipient, model.DeliveryErrorSendFailed)
	replacement, _ := th.message(t, model.MessageTypeText, author, nil)
	deleted, _ := th.message(t, model.MessageTypeText, author, nil)

	_, err := messages.ReplaceFailedMessage(ctx, original, replacement, th.id, author)
	require.NoError(t, err)

	_, err = messages.DeleteMessages(ctx, []uuid.UUID{deleted}, author)
	require.NoError(t, err)

	ids := func(msgs []*model.Message) []uuid.UUID {
		out := make([]uuid.UUID, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, m.ID)
		}

		return out
	}

	history := NewMessageHistoryStore(tx)

	t.Run("thread history", func(t *testing.T) {
		got, err := history.Search(ctx, queryobject.NewMessageHistoryQuery().
			WithFields([]string{"id"}).
			WithDomainIDsFilter(int(itDomainID)).
			WithThreadIDsFilter(th.id).
			WithoutRetried())
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{replacement, deleted}, ids(got))
	})

	t.Run("left-thread history", func(t *testing.T) {
		got, err := history.Search(ctx, queryobject.NewLeftThreadsMessageHistoryQueryObject().
			WithFields([]string{"id"}).
			WithDomainIDFilter(int(itDomainID)).
			WithThreadIDFilter(th.id).
			WithoutRetried())
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{replacement, deleted}, ids(got))
	})

	t.Run("revisions", func(t *testing.T) {
		revisions := NewMessageRevisionStore(tx)

		_, err := revisions.Search(ctx, original, itDomainID, author)
		require.Error(t, err, "a replaced original has no change log")

		entries, err := revisions.Search(ctx, deleted, itDomainID, author)
		require.NoError(t, err)
		assert.Equal(t, model.MessageRevisionActionDeleted, entries[len(entries)-1].Action)
	})
}

func TestClearRecoveredErrors_Integration(t *testing.T) {
	tx := rolledBackTx(t)
	ctx := context.Background()
	statuses := NewMessageStatusStore(tx).(*messageStatusStore)

	author, recipient := uuid.New(), uuid.New()

	// three failed messages to recipient, seq 1..3
	setup := func(t *testing.T) (*itThread, [3]uuid.UUID) {
		t.Helper()

		th := newITThread(t, tx, author, recipient)

		var ids [3]uuid.UUID
		for i := range ids {
			ids[i], _ = th.message(t, model.MessageTypeText, author, nil)
			th.fail(t, ids[i], recipient, model.DeliveryErrorSendFailed)
		}

		return th, ids
	}

	seq := func(n int64) []*int64 { return []*int64{&n} }

	t.Run("an exact receipt recovers only its own message", func(t *testing.T) {
		th, ids := setup(t)

		require.NoError(t, statuses.clearRecoveredErrors(ctx, []uuid.UUID{th.id}, []uuid.UUID{recipient}, seq(2), []bool{true}))

		left := th.errorCodes(t, recipient)
		assert.Contains(t, left, ids[0])
		assert.NotContains(t, left, ids[1])
		assert.Contains(t, left, ids[2])
	})

	t.Run("a watermark recovers everything up to it", func(t *testing.T) {
		th, ids := setup(t)

		require.NoError(t, statuses.clearRecoveredErrors(ctx, []uuid.UUID{th.id}, []uuid.UUID{recipient}, seq(2), []bool{false}))

		left := th.errorCodes(t, recipient)
		assert.NotContains(t, left, ids[0])
		assert.NotContains(t, left, ids[1])
		assert.Contains(t, left, ids[2])
	})

	t.Run("delivered receipts: provider is exact, ws is a watermark", func(t *testing.T) {
		th, ids := setup(t)

		_, err := statuses.MarkDelivered(ctx, []*model.StatusReceipt{
			{DomainID: itDomainID, MessageID: ids[1], MemberID: recipient, Via: "provider"},
		})
		require.NoError(t, err)

		left := th.errorCodes(t, recipient)
		assert.Contains(t, left, ids[0], "a later channel delivery says nothing about an earlier rejection")
		assert.NotContains(t, left, ids[1])
		assert.Contains(t, left, ids[2])

		_, err = statuses.MarkDelivered(ctx, []*model.StatusReceipt{
			{DomainID: itDomainID, ThreadID: th.id, MemberID: recipient, UpToSeq: 3, Via: "ws"},
		})
		require.NoError(t, err)
		assert.Empty(t, th.errorCodes(t, recipient))
	})
}

func TestMarkUnreportedFailed_Integration(t *testing.T) {
	tx := rolledBackTx(t)
	ctx := context.Background()
	statuses := NewMessageStatusStore(tx)

	author, recipient := uuid.New(), uuid.New()
	th := newITThread(t, tx, author, recipient)

	reported, _ := th.message(t, model.MessageTypeText, author, nil)
	th.fail(t, reported, recipient, model.DeliveryErrorFileTooLarge)
	unreported, _ := th.message(t, model.MessageTypeText, author, nil)

	fallback := map[string]any{"code": model.DeliveryErrorChannelUnavailable, "message": "providers unreachable"}

	changes, err := statuses.MarkUnreportedFailed(ctx, []*model.StatusReceipt{
		{MessageID: reported, MemberID: recipient, Via: "provider", Error: fallback},
		{MessageID: unreported, MemberID: recipient, Via: "provider", Error: fallback},
		// The sender is never a failed recipient of their own message.
		{MessageID: unreported, MemberID: author, Via: "provider", Error: fallback},
	})
	require.NoError(t, err)

	require.Len(t, changes, 1, "only the row actually inserted comes back")
	assert.Equal(t, unreported, changes[0].MessageID)

	stored := th.errorCodes(t, recipient)
	assert.Equal(t, model.DeliveryErrorFileTooLarge, stored[reported], "the channel's own report is kept")
	assert.Equal(t, model.DeliveryErrorChannelUnavailable, stored[unreported])

	again, err := statuses.MarkUnreportedFailed(ctx, []*model.StatusReceipt{
		{MessageID: unreported, MemberID: recipient, Via: "provider", Error: fallback},
	})
	require.NoError(t, err)
	assert.Empty(t, again, "a repeated fallback changes nothing")
}
