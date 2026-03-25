package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/service/dto"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
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
		return nil, err
	}

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	messages, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[dto.HistoryMessage])
	if err != nil {
		return nil, err
	}

	return messages, nil
}
