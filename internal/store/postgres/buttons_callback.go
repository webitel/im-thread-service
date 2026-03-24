package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store/querier/sqlbuilder"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type buttonsCallback struct {
	db Querier
}

func NewButtonsCallback(db Querier) *buttonsCallback {
	return &buttonsCallback{
		db: db,
	}
}

func (r *buttonsCallback) Create(ctx context.Context, callback *model.ButtonsCallback) (*model.ButtonsCallback, error) {
	sql, args := sqlbuilder.PrepareButtonsCallbackInsertQuery(callback)

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Internal("postgres.buttons_callback.create", errors.WithCause(err))
	}

	saved, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByNameLax[model.ButtonsCallback])
	if err != nil {
		if errors.Is(err,pgx.ErrNoRows) {
			return nil, prepareBadRequestError("zero rows affected in result set", err)
		}

		if IsConflict(err) {
			return nil, prepareConflictError("conflict: interaction for this message button pair already exists", err)
		}

		return nil, err
	}

	return saved, nil
}
