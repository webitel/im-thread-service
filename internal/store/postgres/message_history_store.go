package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/service/dto"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type messageHistoryStore struct {
	db Querier
}

func NewMessageHistoryStore(db Querier) *messageHistoryStore {
	return &messageHistoryStore{
		db: db,
	}
}

func (s *messageHistoryStore) Search(ctx context.Context, query queryobject.QueryObject) ([]*dto.HistoryMessage, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, errors.Internal("preparing history message query", errors.WithCause(err), errors.WithID("postgres.message_history_store.search"))
	}

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Internal(
			"executing query for message history query",
			errors.WithCause(err),
			errors.WithID("postgres.message_history_store.search"),
			errors.WithValue("query", sql),
		)
	}

	messages, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[dto.HistoryMessage])
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, errors.NotFound("no messages found for passed filters", errors.WithCause(err), errors.WithID("postgres.message_history_store.search"))
		}
		return nil, errors.Internal("collecting messages records", errors.WithCause(err), errors.WithID("postgres.message_history_store.search"))
	}

	return messages, nil
}
