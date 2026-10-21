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

func (s *threadTagStore) Add(ctx context.Context, tag *model.ThreadTag) (*model.ThreadTag, bool, error) {
	query := `
		WITH inserted AS (
			INSERT INTO im_thread.thread_tag (thread_id, contact_id, tag)
			VALUES (@ThreadID, @ContactID, @Tag)
			ON CONFLICT (thread_id, contact_id, tag) DO NOTHING
			RETURNING id, thread_id, contact_id, tag, created_at
		)
		SELECT id, thread_id, contact_id, tag, created_at, true AS inserted FROM inserted
		UNION ALL
		SELECT id, thread_id, contact_id, tag, created_at, false
		FROM im_thread.thread_tag
		WHERE thread_id = @ThreadID AND contact_id = @ContactID AND tag = @Tag
		  AND NOT EXISTS (SELECT 1 FROM inserted)
	`

	args := pgx.NamedArgs{
		"ThreadID":  tag.ThreadID,
		"ContactID": tag.ContactID,
		"Tag":       tag.Tag,
	}

	var (
		result   model.ThreadTag
		inserted bool
	)

	err := s.db.QueryRow(ctx, query, args).Scan(&result.ID, &result.ThreadID, &result.ContactID, &result.Tag, &result.CreatedAt, &inserted)
	if err != nil {
		if err := foreignKeyViolation(err, "thread not found"); err != nil {
			return nil, false, errors.Wrap(err, errors.WithID("postgres.thread_tag.add"))
		}

		return nil, false, errors.Internal(
			"error executing tag insert query",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.add"),
		)
	}

	return &result, inserted, nil
}

func (s *threadTagStore) Remove(ctx context.Context, tagID, contactID uuid.UUID) (uuid.UUID, error) {
	query := `
		DELETE FROM im_thread.thread_tag
		WHERE id = @ID AND contact_id = @ContactID
		RETURNING thread_id
	`

	args := pgx.NamedArgs{
		"ID":        tagID,
		"ContactID": contactID,
	}

	var threadID uuid.UUID

	err := s.db.QueryRow(ctx, query, args).Scan(&threadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errors.NotFound(
			"tag not found",
			errors.WithID("postgres.thread_tag.remove"),
		)
	}

	if err != nil {
		return uuid.Nil, errors.Internal(
			"error executing tag delete query",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.remove"),
		)
	}

	return threadID, nil
}

func (s *threadTagStore) RemoveByName(ctx context.Context, threadID, contactID uuid.UUID, tag string) (bool, error) {
	query := `
		DELETE FROM im_thread.thread_tag
		WHERE thread_id = @ThreadID AND contact_id = @ContactID AND tag = @Tag
	`

	args := pgx.NamedArgs{
		"ThreadID":  threadID,
		"ContactID": contactID,
		"Tag":       tag,
	}

	cmd, err := s.db.Exec(ctx, query, args)
	if err != nil {
		return false, errors.Internal(
			"error executing tag delete by name query",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.remove_by_name"),
		)
	}

	return cmd.RowsAffected() > 0, nil
}

func (s *threadTagStore) RemoveForContacts(ctx context.Context, threadID uuid.UUID, contactIDs []uuid.UUID, tags []string) ([]uuid.UUID, error) {
	if threadID == uuid.Nil || len(contactIDs) == 0 {
		return nil, nil
	}

	query := `
		WITH removed AS (
			DELETE FROM im_thread.thread_tag
			WHERE thread_id = @ThreadID
			  AND contact_id = ANY(@ContactIDs)
			  AND (cardinality(@Tags::text[]) = 0 OR tag = ANY(@Tags::text[]))
			RETURNING contact_id
		)
		SELECT DISTINCT contact_id FROM removed
	`

	if tags == nil {
		tags = make([]string, 0)
	}

	args := pgx.NamedArgs{
		"ThreadID":   threadID,
		"ContactIDs": contactIDs,
		"Tags":       tags,
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"error executing tag delete for contacts query",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.remove_for_contacts"),
		)
	}

	contacts, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errors.Internal(
			"error collecting removed tag contacts",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.remove_for_contacts"),
		)
	}

	return contacts, nil
}

func (s *threadTagStore) ListNames(ctx context.Context, threadID, contactID uuid.UUID) ([]string, error) {
	query := `
		SELECT tag
		FROM im_thread.thread_tag
		WHERE thread_id = @ThreadID AND contact_id = @ContactID
		ORDER BY created_at, tag
	`

	args := pgx.NamedArgs{
		"ThreadID":  threadID,
		"ContactID": contactID,
	}

	rows, err := s.db.Query(ctx, query, args)
	if err != nil {
		return nil, errors.Internal(
			"error executing tag names query",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.list_names"),
		)
	}

	tags, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, errors.Internal(
			"error collecting tag names",
			errors.WithCause(err),
			errors.WithID("postgres.thread_tag.list_names"),
		)
	}

	return tags, nil
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
