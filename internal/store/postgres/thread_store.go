package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	queryobject "github.com/webitel/im-thread-service/internal/store/query_object"
)

type threadStore struct {
	db Querier
}

func NewThreadStore(db Querier) *threadStore {
	return &threadStore{
		db: db,
	}
}

func (s *threadStore) Create(ctx context.Context, req *model.Thread) (*model.Thread, error) {
	var (
		query = `
			insert into im_thread.thread (
				domain_id, created_at, updated_at, 
				kind, owner, subject, description
			)
			values (@DomainId, @CreatedAt, 
				@UpdatedAt, @Kind, @Owner, @Subject,
				@Description
			)
			returning id;
		`
		args = pgx.NamedArgs{
			"DomainId":    req.DomainID,
			"CreatedAt":   req.CreatedAt,
			"UpdatedAt":   req.UpdatedAt,
			"Kind":        req.Kind,
			"Owner":       req.Owner,
			"Subject":     req.Subject,
			"Description": req.Description,
		}
	)

	if err := s.db.QueryRow(ctx, query, args).Scan(&req.ID); err != nil {
		return nil, err
	}

	return req, nil
}

func (s *threadStore) Search(ctx context.Context, query queryobject.QueryObject) ([]*model.Thread, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	records, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[model.Thread])
	if err != nil {
		return nil, err
	}

	return records, nil
}
