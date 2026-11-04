//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration test: exercises the REAL AdoptUntrackedBot query; POSTGRES_DSN=... go test -tags=integration -run AdoptUntrackedBot ./internal/store/postgres/ -count=1 -v
func setupAdoptSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname IN ('im_thread', 'im_contact')`).Scan(&existing); err != nil {
		t.Fatalf("check schemas: %v", err)
	}

	if existing > 0 {
		t.Skip("im_thread/im_contact already exist in this database; run against a throwaway database")
	}

	stmts := []string{
		`CREATE SCHEMA im_thread`,
		`CREATE SCHEMA im_contact`,
		`CREATE TABLE im_contact.contact (
			id     uuid PRIMARY KEY,
			is_bot boolean NOT NULL DEFAULT false
		)`,
		`CREATE TABLE im_thread.thread_dialog (
			id         uuid PRIMARY KEY,
			thread_id  uuid NOT NULL,
			member_id  uuid NOT NULL,
			is_bot     boolean NOT NULL DEFAULT false,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			deleted_at timestamptz
		)`,
		`CREATE TABLE im_thread.bot_control_stack (
			id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			thread_id uuid NOT NULL,
			member_id uuid,
			position  smallint NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP SCHEMA im_thread, im_contact CASCADE`); err != nil {
			t.Logf("drop test schemas: %v", err)
		}
	})
}

type adoptMember struct {
	dialogID   uuid.UUID
	contactID  uuid.UUID
	contactBot bool
	dialogBot  bool
	deleted    bool
}

func seedAdoptThread(t *testing.T, pool *pgxpool.Pool, threadID uuid.UUID, members ...adoptMember) {
	t.Helper()

	ctx := context.Background()

	for i, m := range members {
		if _, err := pool.Exec(ctx, `INSERT INTO im_contact.contact (id, is_bot) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`, m.contactID, m.contactBot); err != nil {
			t.Fatalf("seed contact: %v", err)
		}

		if _, err := pool.Exec(ctx, `INSERT INTO im_thread.thread_dialog (id, thread_id, member_id, is_bot, created_at, deleted_at)
			VALUES ($1, $2, $3, $4, now() + make_interval(secs => $5), CASE WHEN $6 THEN now() END)`,
			m.dialogID, threadID, m.contactID, m.dialogBot, i, m.deleted); err != nil {
			t.Fatalf("seed dialog: %v", err)
		}
	}
}

func dialogIsBot(t *testing.T, pool *pgxpool.Pool, dialogID uuid.UUID) bool {
	t.Helper()

	var isBot bool
	if err := pool.QueryRow(context.Background(), `SELECT is_bot FROM im_thread.thread_dialog WHERE id = $1`, dialogID).Scan(&isBot); err != nil {
		t.Fatalf("read dialog: %v", err)
	}

	return isBot
}

func TestAdoptUntrackedBot_Integration(t *testing.T) {
	pool := itPool(t)
	setupAdoptSchema(t, pool)

	store := NewThreadDialogStore(pool)
	ctx := context.Background()

	newMember := func() adoptMember {
		return adoptMember{dialogID: uuid.New(), contactID: uuid.New(), contactBot: true, dialogBot: true}
	}
	customer := func() adoptMember {
		return adoptMember{dialogID: uuid.New(), contactID: uuid.New()}
	}
	untrackedBot := func() adoptMember {
		return adoptMember{dialogID: uuid.New(), contactID: uuid.New(), contactBot: true}
	}

	t.Run("untracked bot on empty stack is adopted", func(t *testing.T) {
		threadID := uuid.New()
		bot, incoming := untrackedBot(), newMember()
		seedAdoptThread(t, pool, threadID, customer(), bot, incoming)

		got, err := store.AdoptUntrackedBot(ctx, threadID, incoming.dialogID)
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}

		if got != bot.dialogID {
			t.Fatalf("adopted %s, want %s", got, bot.dialogID)
		}

		if !dialogIsBot(t, pool, bot.dialogID) {
			t.Fatal("adopted dialog is not marked as a bot")
		}
	})

	t.Run("stack already holds an entry, nothing adopted", func(t *testing.T) {
		threadID := uuid.New()
		bot, incoming := untrackedBot(), newMember()
		seedAdoptThread(t, pool, threadID, customer(), bot, incoming)

		if _, err := pool.Exec(ctx, `INSERT INTO im_thread.bot_control_stack (thread_id, member_id, position) VALUES ($1, $2, 0)`, threadID, uuid.New()); err != nil {
			t.Fatalf("seed stack: %v", err)
		}

		got, err := store.AdoptUntrackedBot(ctx, threadID, incoming.dialogID)
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}

		if got != uuid.Nil {
			t.Fatalf("adopted %s on a tracked stack", got)
		}

		if dialogIsBot(t, pool, bot.dialogID) {
			t.Fatal("dialog was marked as a bot on a tracked stack")
		}
	})

	t.Run("no bot contact among members, nothing adopted", func(t *testing.T) {
		threadID := uuid.New()
		incoming := newMember()
		seedAdoptThread(t, pool, threadID, customer(), customer(), incoming)

		got, err := store.AdoptUntrackedBot(ctx, threadID, incoming.dialogID)
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}

		if got != uuid.Nil {
			t.Fatalf("adopted %s without a bot contact", got)
		}
	})

	t.Run("deleted untracked bot is ignored", func(t *testing.T) {
		threadID := uuid.New()
		bot, incoming := untrackedBot(), newMember()
		bot.deleted = true
		seedAdoptThread(t, pool, threadID, customer(), bot, incoming)

		got, err := store.AdoptUntrackedBot(ctx, threadID, incoming.dialogID)
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}

		if got != uuid.Nil {
			t.Fatalf("adopted deleted dialog %s", got)
		}
	})

	t.Run("incoming bot itself is never adopted", func(t *testing.T) {
		threadID := uuid.New()
		incoming := untrackedBot()
		seedAdoptThread(t, pool, threadID, customer(), incoming)

		got, err := store.AdoptUntrackedBot(ctx, threadID, incoming.dialogID)
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}

		if got != uuid.Nil {
			t.Fatalf("adopted the incoming member %s", got)
		}
	})

	t.Run("several untracked bots, the earliest member is adopted", func(t *testing.T) {
		threadID := uuid.New()
		first, second, incoming := untrackedBot(), untrackedBot(), newMember()
		seedAdoptThread(t, pool, threadID, customer(), first, second, incoming)

		got, err := store.AdoptUntrackedBot(ctx, threadID, incoming.dialogID)
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}

		if got != first.dialogID {
			t.Fatalf("adopted %s, want earliest %s", got, first.dialogID)
		}

		if dialogIsBot(t, pool, second.dialogID) {
			t.Fatal("more than one dialog was marked as a bot")
		}
	})

	t.Run("ticket thread: imported portal bot adopted on transfer to Tier 1 Level 2", func(t *testing.T) {
		threadID := uuid.MustParse("85a27368-d03f-41e3-8f96-92d64c30c23e")
		portal := adoptMember{
			dialogID:   uuid.MustParse("3fda8f79-1655-4830-bd4c-2aa85370d605"),
			contactID:  uuid.MustParse("01a01e29-3ed2-7caf-9056-47c29c95619a"),
			contactBot: true,
		}
		client := adoptMember{
			dialogID:  uuid.MustParse("ded995a6-fa68-4a69-8a24-c5ebc4c61aec"),
			contactID: uuid.MustParse("6e12db43-9b75-41b5-87fb-45cbe3c233a5"),
		}
		agent := adoptMember{
			dialogID:  uuid.MustParse("01a17822-d75f-755f-8cdf-dfba43978c8d"),
			contactID: uuid.MustParse("01a16202-b7b6-7256-87b8-6cf83bb578a9"),
			deleted:   true,
		}
		tier1 := adoptMember{
			dialogID:   uuid.MustParse("01a17828-d2b9-7709-8da7-30a96856c196"),
			contactID:  uuid.MustParse("01a04158-06a0-7203-81d5-726641371d72"),
			contactBot: true,
			dialogBot:  true,
		}
		seedAdoptThread(t, pool, threadID, portal, client, agent, tier1)

		got, err := store.AdoptUntrackedBot(ctx, threadID, tier1.dialogID)
		if err != nil {
			t.Fatalf("adopt: %v", err)
		}

		if got != portal.dialogID {
			t.Fatalf("adopted %s, want portal bot %s", got, portal.dialogID)
		}

		if !dialogIsBot(t, pool, portal.dialogID) {
			t.Fatal("portal bot is not marked as a bot")
		}

		if dialogIsBot(t, pool, client.dialogID) {
			t.Fatal("customer was marked as a bot")
		}
	})
}
