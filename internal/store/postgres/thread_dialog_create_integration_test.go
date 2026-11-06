//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
)

// Integration test: exercises the REAL ThreadDialogStore.Create query; POSTGRES_DSN=... go test -tags=integration -run ThreadDialogCreate ./internal/store/postgres/ -count=1 -v
func setupDialogCreateSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname = 'im_thread'`).Scan(&existing); err != nil {
		t.Fatalf("check schema: %v", err)
	}

	if existing > 0 {
		t.Skip("im_thread already exists in this database; run against a throwaway database")
	}

	stmts := []string{
		`CREATE SCHEMA im_thread`,
		`CREATE TABLE im_thread.thread (
			id        uuid PRIMARY KEY,
			domain_id bigint NOT NULL
		)`,
		`CREATE TABLE im_thread.thread_dialog (
			id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			domain_id    bigint NOT NULL,
			created_at   timestamptz NOT NULL DEFAULT now(),
			updated_at   timestamptz NOT NULL DEFAULT now(),
			deleted_at   timestamptz,
			member_id    uuid NOT NULL,
			thread_id    uuid NOT NULL REFERENCES im_thread.thread (id) ON DELETE CASCADE,
			thread_role  int NOT NULL DEFAULT 0,
			invited_by   uuid,
			leave_reason text,
			via          text,
			is_bot       boolean NOT NULL DEFAULT false,
			auto_leave   boolean NOT NULL DEFAULT false
		)`,
		`CREATE TABLE im_thread.thread_permission (
			id                             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			thread_id                      uuid NOT NULL,
			thread_dialog_id               uuid NOT NULL REFERENCES im_thread.thread_dialog (id) ON DELETE CASCADE,
			can_send_messages              boolean NOT NULL DEFAULT true,
			can_add_members                boolean NOT NULL DEFAULT true,
			can_remove_members             boolean NOT NULL DEFAULT false,
			can_change_members_permissions boolean NOT NULL DEFAULT false,
			can_change_thread_info         boolean NOT NULL DEFAULT false,
			can_delete_messages            boolean NOT NULL DEFAULT false
		)`,
		`CREATE TABLE im_thread.direct_settings (
			id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			domain_id        bigint NOT NULL,
			thread_dialog_id uuid NOT NULL UNIQUE REFERENCES im_thread.thread_dialog (id) ON DELETE CASCADE,
			title            varchar(255) NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}

	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP SCHEMA im_thread CASCADE`); err != nil {
			t.Logf("drop test schema: %v", err)
		}
	})
}

// A freshly created direct thread is dispatched to external providers straight
// from the dialogs Create returns, so a dropped via silently skips the first
// outbound message.
func TestThreadDialogCreate_ReturnsVia_Integration(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set; skipping thread dialog create integration test")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	setupDialogCreateSchema(t, pool)

	const domainID = 1

	threadID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO im_thread.thread (id, domain_id) VALUES ($1, $2)`, threadID, domainID); err != nil {
		t.Fatalf("seed thread: %v", err)
	}

	store := NewThreadDialogStore(pool)
	gateID := "test-gate-" + uuid.NewString()

	tests := []struct {
		name string
		via  *string
	}{
		{name: "external member keeps via", via: &gateID},
		{name: "member without via stays nil", via: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := store.Create(ctx, &model.ThreadDialogExtended{
				BaseModel:  shared.BaseModel{DomainID: domainID},
				ThreadID:   threadID,
				ContactID:  uuid.New(),
				ThreadRole: model.RoleOwner,
				Via:        tt.via,
				Settings:   model.BaseThreadSetting{Title: "test-title"},
			})
			if err != nil {
				t.Fatalf("create: %v", err)
			}

			switch {
			case tt.via == nil && created.Via != nil:
				t.Fatalf("via = %q, want nil", *created.Via)
			case tt.via != nil && created.Via == nil:
				t.Fatalf("via = nil, want %q", *tt.via)
			case tt.via != nil && *created.Via != *tt.via:
				t.Fatalf("via = %q, want %q", *created.Via, *tt.via)
			}
		})
	}
}
