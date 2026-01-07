package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
)

type threadStore struct {
	db Querier
}

func NewThreadStore(db Querier) *threadStore {
	return &threadStore{
		db: db,
	}
}

// Create creates a new thread with the given request.
// It returns the newly created thread or an error if the operation fails.
// If the operation succeeds, it returns the newly created thread with the id set.
// If the operation fails, it returns nil and the error.
func (t *threadStore) Create(ctx context.Context, req *model.Thread) (*model.Thread, error) {
	var (
		query = `
			insert into im_thread.thread (
				domain_id, created_by, created_at, updated_by, updated_at, 
				kind, owner, subject, description
			)
			values (@DomainId, @CreatedBy, @CreatedAt, @UpdatedBy, 
				@UpdatedAt, @Kind, @Owner, @Subject,
				@Description
			)
			returning id;
		`
		args = pgx.NamedArgs{
			"DomainId":    req.DomainID,
			"CreatedBy":   req.CreatedBy,
			"CreatedAt":   req.CreatedAt,
			"UpdatedBy":   req.UpdatedBy,
			"UpdatedAt":   req.UpdatedAt,
			"Kind":        req.Kind,
			"Owner":       req.Owner,
			"Subject":     req.Subject,
			"Description": req.Description,
		}
	)

	// SCAN ONLY ID AS OTHER PARAMS ALREADY HAS BEEN SET ON APP LEVEL
	if err := t.db.QueryRow(ctx, query, args).Scan(&req.ID); err != nil {
		return nil, err
	}

	return req, nil
}
