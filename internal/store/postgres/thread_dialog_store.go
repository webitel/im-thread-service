package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

type threadDialogStore struct {
	db Querier
}

func NewThreadDialogStore(db Querier) *threadDialogStore {
	return &threadDialogStore{
		db: db,
	}
}

// Resolve resolves a direct thread by peers.
// It returns the thread id if found, or nil and the error if not found.
// If the operation succeeds, it returns the resolved thread id.
// If the operation fails, it returns nil and the error.
func (t *threadDialogStore) Resolve(ctx context.Context, search *dto.SearchThreadDialogRequest) (uuid.UUID, error) {
	var (
		query = `
			select
				thread_id
			from im_thread.thread_dialog
			where domain_id = @DomainId
				and (member_id, direct_to) = (@FromId, @DirectTo)
			limit 1
		`
		args = pgx.NamedArgs{
			"DomainId": search.DomainID,
			"FromId":   search.From.ID,
			"DirectTo": search.To.ID,
		}
		resolvedId uuid.UUID
	)

	if err := t.db.QueryRow(ctx, query, args).Scan(&resolvedId); err != nil {
		return uuid.Nil, err
	}

	return resolvedId, nil
}

// CreateDirectPair creates two new thread dialogs for peers within one transaction.
// It returns two newly created thread dialogs or an error if the operation fails.
// If the operation succeeds, it returns two newly created thread dialogs with the id set.
// If the operation fails, it returns nil and the error.
func (t *threadDialogStore) CreateDirectPair(ctx context.Context, dialog *model.ThreadDialog) ([]*model.ThreadDialog, error) {
	var (
		query = `
			insert into im_thread.thread_dialog (
				domain_id, created_at, updated_at,
				member_id, thread_id, direct_to
			)
			values (
				@DomainId, @CreatedAt, @UpdatedAt,
				@From, @ThreadId, @To 
			), (
				@DomainId, @CreatedAt, @UpdatedAt,
				@To, @ThreadId, @From
			)
			returning
				id,
				domain_id,
				created_at,
				updated_at,
				member_id,
				thread_id,
				direct_to
		`
		args = pgx.NamedArgs{
			"DomainId":  dialog.DomainID,
			"CreatedAt": dialog.CreatedAt,
			"UpdatedAt": dialog.UpdatedAt,
			"From":      dialog.MemberID,
			"ThreadId":  dialog.ThreadID,
			"To":        dialog.DirectTo,
		}
		result = make([]*model.ThreadDialog, 0, 2)
	)

	rows, err := t.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		createdDialog := &model.ThreadDialog{}
		err = rows.Scan(
			&createdDialog.ID,
			&createdDialog.DomainID,
			&createdDialog.CreatedAt,
			&createdDialog.UpdatedAt,
			&createdDialog.MemberID,
			&createdDialog.ThreadID,
			&createdDialog.DirectTo,
		)
		if err != nil {
			return nil, err
		}

		result = append(result, createdDialog)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
