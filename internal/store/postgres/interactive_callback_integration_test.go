//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

// Integration test: exercises the REAL callback save query; POSTGRES_DSN=... go test -tags=integration -run InteractiveCallback ./internal/store/postgres/ -count=1 -v
func setupCallbackSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname IN ('im_thread', 'im_message')`).Scan(&existing); err != nil {
		t.Fatalf("check schemas: %v", err)
	}

	if existing > 0 {
		t.Skip("im_thread/im_message already exist in this database; run against a throwaway database")
	}

	stmts := []string{
		`CREATE SCHEMA im_thread`,
		`CREATE SCHEMA im_message`,
		`CREATE TABLE im_message.messages (
			id        uuid PRIMARY KEY,
			thread_id uuid NOT NULL,
			domain_id int NOT NULL,
			sender_id uuid NOT NULL,
			member_id uuid
		)`,
		`CREATE TABLE im_message.buttons_callback (
			in_reply_to   uuid NOT NULL REFERENCES im_message.messages (id) ON DELETE CASCADE,
			button_code   text NOT NULL,
			callback_data text NOT NULL,
			reacted_at    timestamptz NOT NULL DEFAULT now(),
			reacted_by    uuid NOT NULL,
			PRIMARY KEY (in_reply_to, button_code)
		)`,
		`CREATE TABLE im_thread.thread_dialog (
			id          uuid PRIMARY KEY,
			thread_id   uuid NOT NULL,
			member_id   uuid NOT NULL,
			thread_role int NOT NULL DEFAULT 0,
			is_bot      boolean NOT NULL DEFAULT false,
			deleted_at  timestamptz
		)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP SCHEMA im_thread, im_message CASCADE`); err != nil {
			t.Logf("drop test schemas: %v", err)
		}
	})
}

type callbackDialog struct {
	id      uuid.UUID
	contact uuid.UUID
	role    int
	isBot   bool
	// deletedAgo soft-deletes the row that long ago; 0 keeps it active.
	deletedAgo time.Duration
}

func seedCallbackThread(t *testing.T, pool *pgxpool.Pool, threadID, messageID, senderID uuid.UUID, dialogs ...callbackDialog) {
	t.Helper()

	seedCallbackMessage(t, pool, threadID, messageID, senderID, nil, dialogs...)
}

// seedCallbackMessage seeds a message whose sender membership is senderMemberID
// (nil like messages written before im_message.messages.member_id existed).
func seedCallbackMessage(t *testing.T, pool *pgxpool.Pool, threadID, messageID, senderID uuid.UUID, senderMemberID *uuid.UUID, dialogs ...callbackDialog) {
	t.Helper()

	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO im_message.messages (id, thread_id, domain_id, sender_id, member_id) VALUES ($1, $2, 1, $3, $4)`,
		messageID, threadID, senderID, senderMemberID); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	for _, d := range dialogs {
		var deletedAt *time.Time
		if d.deletedAgo > 0 {
			at := time.Now().Add(-d.deletedAgo)
			deletedAt = &at
		}

		if _, err := pool.Exec(ctx, `INSERT INTO im_thread.thread_dialog (id, thread_id, member_id, thread_role, is_bot, deleted_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			d.id, threadID, d.contact, d.role, d.isBot, deletedAt); err != nil {
			t.Fatalf("seed dialog: %v", err)
		}
	}
}

func TestInteractiveCallbackSave_Integration(t *testing.T) {
	pool := itPool(t)
	setupCallbackSchema(t, pool)

	store := NewInteractiveCallbackStore(pool)
	ctx := context.Background()

	t.Run("returns both memberships", func(t *testing.T) {
		threadID, messageID := uuid.New(), uuid.New()
		reactor := callbackDialog{id: uuid.New(), contact: uuid.New(), role: 1}
		bot := callbackDialog{id: uuid.New(), contact: uuid.New(), role: 2, isBot: true}
		seedCallbackThread(t, pool, threadID, messageID, bot.contact, reactor, bot)

		got, err := store.Save(ctx, &model.InteractiveCallback{
			ReactedBy: reactor.contact, InReplyTo: messageID, ButtonCode: "btn_a", CallbackData: "data_a",
		})
		require.NoError(t, err)

		require.False(t, got.ReactedByIsBot)
		require.True(t, got.ReceiverIsBot)

		require.Equal(t, threadID, got.ThreadID)
		require.Equal(t, 1, got.DomainID)
		require.Equal(t, bot.contact, got.Receiver)
		require.Equal(t, &reactor.id, got.ReactedByMemberID)
		require.Equal(t, model.ThreadRole(1), got.ReactedByRole)
		require.Equal(t, &bot.id, got.ReceiverMemberID)
		require.Equal(t, model.ThreadRole(2), got.ReceiverRole)
	})

	t.Run("receiver without membership yields nil member", func(t *testing.T) {
		threadID, messageID := uuid.New(), uuid.New()
		reactor := callbackDialog{id: uuid.New(), contact: uuid.New(), role: 1}
		seedCallbackThread(t, pool, threadID, messageID, uuid.New(), reactor)

		got, err := store.Save(ctx, &model.InteractiveCallback{
			ReactedBy: reactor.contact, InReplyTo: messageID, ButtonCode: "btn_b", CallbackData: "data_b",
		})
		require.NoError(t, err)

		require.Equal(t, &reactor.id, got.ReactedByMemberID)
		require.Nil(t, got.ReceiverMemberID)
		require.Equal(t, model.ThreadRole(0), got.ReceiverRole)
	})

	t.Run("receiver that left still resolves its membership", func(t *testing.T) {
		threadID, messageID := uuid.New(), uuid.New()
		reactor := callbackDialog{id: uuid.New(), contact: uuid.New(), role: 1}
		bot := callbackDialog{id: uuid.New(), contact: uuid.New(), role: 2, deletedAgo: time.Minute}
		seedCallbackThread(t, pool, threadID, messageID, bot.contact, reactor, bot)

		got, err := store.Save(ctx, &model.InteractiveCallback{
			ReactedBy: reactor.contact, InReplyTo: messageID, ButtonCode: "btn_c", CallbackData: "data_c",
		})
		require.NoError(t, err)

		require.Equal(t, &bot.id, got.ReceiverMemberID)
		require.Equal(t, model.ThreadRole(2), got.ReceiverRole)
	})

	t.Run("rejoined members resolve their active membership", func(t *testing.T) {
		threadID, messageID := uuid.New(), uuid.New()
		reactorContact, botContact := uuid.New(), uuid.New()
		reactorOld := callbackDialog{id: uuid.New(), contact: reactorContact, role: 3, deletedAgo: time.Hour}
		reactorActive := callbackDialog{id: uuid.New(), contact: reactorContact, role: 1}
		botActive := callbackDialog{id: uuid.New(), contact: botContact, role: 2}
		botOld := callbackDialog{id: uuid.New(), contact: botContact, role: 3, deletedAgo: time.Hour}
		seedCallbackThread(t, pool, threadID, messageID, botContact, reactorOld, reactorActive, botActive, botOld)

		got, err := store.Save(ctx, &model.InteractiveCallback{
			ReactedBy: reactorContact, InReplyTo: messageID, ButtonCode: "btn_e", CallbackData: "data_e",
		})
		require.NoError(t, err)

		require.Equal(t, &reactorActive.id, got.ReactedByMemberID)
		require.Equal(t, model.ThreadRole(1), got.ReactedByRole)
		require.Equal(t, &botActive.id, got.ReceiverMemberID)
		require.Equal(t, model.ThreadRole(2), got.ReceiverRole)
	})

	t.Run("receiver that left twice resolves its latest membership", func(t *testing.T) {
		threadID, messageID := uuid.New(), uuid.New()
		reactor := callbackDialog{id: uuid.New(), contact: uuid.New(), role: 1}
		botContact := uuid.New()
		botOlder := callbackDialog{id: uuid.New(), contact: botContact, role: 3, deletedAgo: 2 * time.Hour}
		botLatest := callbackDialog{id: uuid.New(), contact: botContact, role: 2, deletedAgo: time.Hour}
		seedCallbackThread(t, pool, threadID, messageID, botContact, reactor, botOlder, botLatest)

		got, err := store.Save(ctx, &model.InteractiveCallback{
			ReactedBy: reactor.contact, InReplyTo: messageID, ButtonCode: "btn_f", CallbackData: "data_f",
		})
		require.NoError(t, err)

		require.Equal(t, &botLatest.id, got.ReceiverMemberID)
		require.Equal(t, model.ThreadRole(2), got.ReceiverRole)
	})

	t.Run("receiver resolves the membership that sent the message", func(t *testing.T) {
		threadID, messageID := uuid.New(), uuid.New()
		reactor := callbackDialog{id: uuid.New(), contact: uuid.New(), role: 1}
		botContact := uuid.New()
		botSender := callbackDialog{id: uuid.New(), contact: botContact, role: 3, deletedAgo: time.Hour}
		botActive := callbackDialog{id: uuid.New(), contact: botContact, role: 2}
		seedCallbackMessage(t, pool, threadID, messageID, botContact, &botSender.id, reactor, botSender, botActive)

		got, err := store.Save(ctx, &model.InteractiveCallback{
			ReactedBy: reactor.contact, InReplyTo: messageID, ButtonCode: "btn_g", CallbackData: "data_g",
		})
		require.NoError(t, err)

		require.Equal(t, &botSender.id, got.ReceiverMemberID)
		require.Equal(t, model.ThreadRole(3), got.ReceiverRole)
	})

	t.Run("non member is rejected", func(t *testing.T) {
		threadID, messageID := uuid.New(), uuid.New()
		seedCallbackThread(t, pool, threadID, messageID, uuid.New())

		_, err := store.Save(ctx, &model.InteractiveCallback{
			ReactedBy: uuid.New(), InReplyTo: messageID, ButtonCode: "btn_d", CallbackData: "data_d",
		})
		require.Error(t, err)
	})
}
