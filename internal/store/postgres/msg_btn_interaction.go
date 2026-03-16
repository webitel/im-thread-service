package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/store/querier"
)

type messageButtonInteraction struct {
	db Querier
	mbiQuerier querier.MessageButtonInteractionQuerier
}

func NewMessageButtonInteraction(db Querier, mbiQuerier querier.MessageButtonInteractionQuerier) *messageButtonInteraction {
	return &messageButtonInteraction{
		db:         db,
		mbiQuerier: mbiQuerier,
	}
}

func (s *messageButtonInteraction) Create(ctx context.Context, mbi *model.MessageButtonInteraction) (*model.MessageButtonInteraction, error) {
	query, args, err := s.mbiQuerier.Insert(mbi)
	if err != nil {
		return nil, err
	}

	var createdMbi model.MessageButtonInteraction

	scanFuncs, err := s.mbiQuerier.ScanFn(&createdMbi, mbi.Result)
	if err != nil {
		return nil, err
	}

	if err := s.db.QueryRow(ctx, query, args...).Scan(scanFuncs...); err != nil {		
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505"{
			return nil, model.ErrInteractionConflict
		}
		
		return nil, err
	}

	createdMbi.Result = mbi.Result

	return &createdMbi, nil
}