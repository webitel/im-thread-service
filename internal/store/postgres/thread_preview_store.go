package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	werrors "github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type threadPreviewStore struct {
	db Querier
}

var _ store.ThreadPreviewStore = (*threadPreviewStore)(nil)

const threadPreviewColumns = "id, domain_id, thread_id, contact_id, initiator_id, created_at, expires_at, revoked_at, revoke_reason"

func NewThreadPreviewStore(db Querier) *threadPreviewStore {
	return &threadPreviewStore{db: db}
}

func (s *threadPreviewStore) GetActiveForUpdate(ctx context.Context, threadID, contactID uuid.UUID, domainID int) (*model.ThreadPreview, error) {
	query := `
		SELECT ` + threadPreviewColumns + `
		FROM im_thread.thread_preview
		WHERE thread_id = @ThreadID
			AND contact_id = @ContactID
			AND (@DomainID::bigint = 0 or domain_id = @DomainID)
			AND revoked_at is null
			AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`

	args := pgx.NamedArgs{
		"ThreadID":  threadID,
		"ContactID": contactID,
		"DomainID":  domainID,
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, werrors.Internal(
			"error executing preview select query",
			werrors.WithCause(err),
			werrors.WithID("postgres.thread_preview.get_active_for_update"),
		)
	}

	result, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.ThreadPreview])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrPreviewNotActive
		}

		return nil, werrors.Internal(
			"error collecting preview select result",
			werrors.WithCause(err),
			werrors.WithID("postgres.thread_preview.get_active_for_update"),
		)
	}

	return result, nil
}

func (s *threadPreviewStore) Upsert(ctx context.Context, preview *model.ThreadPreview, ttl time.Duration) (*model.ThreadPreview, error) {
	// A lapsed but unrevoked row still holds the unique (thread_id, contact_id) slot; close it first
	// so the insert below starts a new grant instead of reviving it. Kept as a separate statement:
	// in one statement the INSERT's conflict check would not see a CTE's update.
	expireQuery := `
		UPDATE im_thread.thread_preview
		SET revoked_at = now(), revoke_reason = @Reason
		WHERE thread_id = @ThreadID
			AND contact_id = @ContactID
			AND revoked_at is null
			AND expires_at <= now()
	`

	expireArgs := pgx.NamedArgs{
		"ThreadID":  preview.ThreadID,
		"ContactID": preview.ContactID,
		"Reason":    string(model.PreviewRevokeReasonExpired),
	}

	if _, err := s.db.Exec(ctx, expireQuery, expireArgs); err != nil {
		return nil, werrors.Internal(
			"error executing preview expire query",
			werrors.WithCause(err),
			werrors.WithID("postgres.thread_preview.upsert"),
		)
	}

	upsertQuery := `
		INSERT INTO im_thread.thread_preview (domain_id, thread_id, contact_id, initiator_id, expires_at)
		VALUES (@DomainID, @ThreadID, @ContactID, @InitiatorID, now() + (@TTLMillis::bigint * interval '1 millisecond'))
		ON CONFLICT (thread_id, contact_id) WHERE revoked_at is null
		DO UPDATE SET expires_at = excluded.expires_at
		RETURNING ` + threadPreviewColumns

	upsertArgs := pgx.NamedArgs{
		"DomainID":    preview.DomainID,
		"ThreadID":    preview.ThreadID,
		"ContactID":   preview.ContactID,
		"InitiatorID": preview.InitiatorID,
		"TTLMillis":   ttl.Milliseconds(),
	}

	rows, err := s.db.Query(ctx, upsertQuery, upsertArgs)
	if err != nil {
		return nil, werrors.Internal(
			"error executing preview upsert query",
			werrors.WithCause(err),
			werrors.WithID("postgres.thread_preview.upsert"),
		)
	}

	result, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.ThreadPreview])
	if err != nil {
		if err := foreignKeyViolation(err, "thread not found"); err != nil {
			return nil, werrors.Wrap(err, werrors.WithID("postgres.thread_preview.upsert"))
		}

		return nil, werrors.Internal(
			"error collecting preview upsert result",
			werrors.WithCause(err),
			werrors.WithID("postgres.thread_preview.upsert"),
		)
	}

	return result, nil
}

func (s *threadPreviewStore) Revoke(ctx context.Context, previewID uuid.UUID, reason model.PreviewRevokeReason) error {
	query := `
		UPDATE im_thread.thread_preview
		SET revoked_at = now(), revoke_reason = @Reason
		WHERE id = @ID AND revoked_at is null
	`

	args := pgx.NamedArgs{
		"ID":     previewID,
		"Reason": string(reason),
	}

	cmd, err := s.db.Exec(ctx, query, args)
	if err != nil {
		return werrors.Internal(
			"error executing preview revoke query",
			werrors.WithCause(err),
			werrors.WithID("postgres.thread_preview.revoke"),
		)
	}

	if cmd.RowsAffected() == 0 {
		return store.ErrPreviewNotActive
	}

	return nil
}

func (s *threadPreviewStore) CanRead(ctx context.Context, threadID, callerID uuid.UUID, domainID int) (bool, error) {
	query := `
		SELECT exists (
			SELECT 1 FROM im_thread.thread_dialog td
			WHERE td.thread_id = @ThreadID
				AND td.member_id = @CallerID
				AND td.deleted_at is null
				AND (@DomainID::bigint = 0 or td.domain_id = @DomainID)
		) or exists (
			SELECT 1 FROM im_thread.thread_preview pv
			WHERE pv.thread_id = @ThreadID
				AND pv.contact_id = @CallerID
				AND pv.revoked_at is null
				AND pv.expires_at > now()
				AND (@DomainID::bigint = 0 or pv.domain_id = @DomainID)
		)
	`

	args := pgx.NamedArgs{
		"ThreadID": threadID,
		"CallerID": callerID,
		"DomainID": domainID,
	}

	var ok bool
	err := s.db.QueryRow(ctx, query, args).Scan(&ok)
	if err != nil {
		return false, werrors.Internal(
			"error executing can_read query",
			werrors.WithCause(err),
			werrors.WithID("postgres.thread_preview.can_read"),
		)
	}

	return ok, nil
}
