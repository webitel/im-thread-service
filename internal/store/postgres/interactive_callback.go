package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/webitel/webitel-go-kit/pkg/errors"

	"github.com/webitel/im-thread-service/internal/domain/model"
)

type interactiveCallbackStore struct {
	db Querier
}

func NewInteractiveCallbackStore(db Querier) *interactiveCallbackStore {
	return &interactiveCallbackStore{db: db}
}

func (s *interactiveCallbackStore) Save(ctx context.Context, callback *model.InteractiveCallback) (*model.InteractiveCallback, error) {
	sql, args := prepareInteractiveCallbackQuery(callback)

	rows, err := s.db.Query(ctx, sql, args)
	if err != nil {
		return nil, errors.Internal(
			"error executing callback save query",
			errors.WithCause(err),
			errors.WithID("postgres.interactive_callback.save"),
		)
	}

	result, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.InteractiveCallback])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.Forbidden(
				"replying to a not existing message or not member of the thread",
				errors.WithCause(err),
				errors.WithID("postgres.interactive_callback.save"),
			)
		}

		if err := uniqueViolation(err); err != nil {
			return nil, errors.Wrap(err, errors.WithID("postgres.interactive_callback.save"))
		}

		return nil, errors.Internal(
			"error collecting callback save result",
			errors.WithCause(err),
			errors.WithID("postgres.interactive_callback.save"),
		)
	}

	return result, nil
}

func prepareInteractiveCallbackQuery(callback *model.InteractiveCallback) (string, pgx.NamedArgs) {
	query := `
		insert into im_message.buttons_callback (
			in_reply_to,
			reacted_by,
			button_code,
			callback_data
		)
		select
			m.id,
			@ReactedBy,
			@ButtonCode,
			@CallbackData
		from im_message.messages m
		inner join im_thread.thread t on m.thread_id = t.id
		where m.id = @MessageID
		and exists (
			select 1
			from im_thread.thread_dialog td
			where (td.member_id, td.thread_id) = (@ReactedBy, t.id)
		)
		limit 1
		returning *;
	`

	args := pgx.NamedArgs{
		"ReactedBy":    callback.ReactedBy,
		"ButtonCode":   callback.ButtonCode,
		"CallbackData": callback.CallbackData,
		"MessageID":    callback.InReplyTo,
	}

	return query, args
}
