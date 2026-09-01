package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store"
)

type threadTagStore struct {
	db Querier
}

var _ store.ThreadTagStore = (*threadTagStore)(nil)

func NewThreadTagStore(db Querier) *threadTagStore {
	return &threadTagStore{db: db}
}

func (s *threadTagStore) Add(ctx context.Context, tag *model.ThreadTag) (*model.ThreadTag, error) {
	query := `
		INSERT INTO im_thread.thread_tag (thread_id, contact_id, tag)
		VALUES (@ThreadID, @ContactID, @Tag)
		RETURNING id, thread_id, contact_id, tag, created_at
	`

	args := pgx.NamedArgs{
		"ThreadID":  tag.ThreadID,
		"ContactID": tag.ContactID,
		"Tag":       tag.Tag,
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"error executing tag insert query",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.add"),
		)
	}

	result, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.ThreadTag])
	if err != nil {
		if err := uniqueViolation(err, "tag already exists for this chat and contact"); err != nil {
			return nil, errors.Wrap(err, errors.WithID("postgres.thread_tag.add"))
		}

		if err := foreignKeyViolation(err, "thread not found"); err != nil {
			return nil, errors.Wrap(err, errors.WithID("postgres.thread_tag.add"))
		}

		return nil, errors.Internal(
			"error collecting tag insert result",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.add"),
		)
	}

	return result, nil
}

func (s *threadTagStore) Remove(ctx context.Context, tagID, contactID uuid.UUID) error {
	query := `
		DELETE FROM im_thread.thread_tag
		WHERE id = @ID AND contact_id = @ContactID
	`

	args := pgx.NamedArgs{
		"ID":        tagID,
		"ContactID": contactID,
	}

	cmd, err := s.db.Exec(ctx, query, args)
	if err != nil {
		return errors.Internal(
			"error executing tag delete query",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.remove"),
		)
	}

	if cmd.RowsAffected() == 0 {
		return errors.NotFound(
			"tag not found",
			errors.WithID("postgres.thread_tag.remove"),
		)
	}

	return nil
}

func (s *threadTagStore) ListForContact(ctx context.Context, contactID uuid.UUID, threadIDs []uuid.UUID) (map[uuid.UUID][]*model.ThreadTag, error) {
	if contactID == uuid.Nil || len(threadIDs) == 0 {
		return make(map[uuid.UUID][]*model.ThreadTag), nil
	}

	query := `
		SELECT id, thread_id, contact_id, tag, created_at
		FROM im_thread.thread_tag
		WHERE contact_id = @ContactID AND thread_id = ANY(@ThreadIDs)
		ORDER BY created_at
	`

	args := pgx.NamedArgs{
		"ContactID": contactID,
		"ThreadIDs": threadIDs,
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"error executing tag list query",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.list_for_contact"),
		)
	}

	tags, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.ThreadTag])
	if err != nil {
		return nil, errors.Internal(
			"error collecting tag list result",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.list_for_contact"),
		)
	}

	result := make(map[uuid.UUID][]*model.ThreadTag)
	for _, tag := range tags {
		result[tag.ThreadID] = append(result[tag.ThreadID], tag)
	}

	return result, nil
}

func (s *threadTagStore) SearchTags(ctx context.Context, contactID uuid.UUID, page, size int) ([]string, error) {
	if contactID == uuid.Nil {
		return nil, errors.InvalidArgument("contact_id is required", errors.WithID("postgres.thread_tag.search_tags"))
	}

	limit := size

	if limit <= 0 {
		limit = limitDefault
	}

	if limit > maxLimit {
		limit = maxLimit
	}

	offset := 0

	if page > 1 {
		offset = (page - 1) * limit
	}

	query := `
		SELECT DISTINCT tag
		FROM im_thread.thread_tag
		WHERE contact_id = @ContactID
		ORDER BY tag
		LIMIT @Limit OFFSET @Offset
	`

	args := pgx.NamedArgs{
		"ContactID": contactID,
		"Limit":     limit + 1,
		"Offset":    offset,
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"error executing tag search query",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.search_tags"),
		)
	}

	tags, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, errors.Internal(
			"error collecting tag search result",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.search_tags"),
		)
	}

	return tags, nil
}
