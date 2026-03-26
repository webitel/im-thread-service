package postgres

import (
	"context"
	"errors"
	"fmt"

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
            select thread_id
            from im_thread.thread_dialog
            where domain_id = @DomainId
                and member_id = @FromId
                and direct_to = @DirectTo
            limit 1
        `
		args = pgx.NamedArgs{
			"DomainId": search.DomainID,
			"FromId":   search.From.ID,
			"DirectTo": search.To.ID,
		}
		resolvedId uuid.UUID
	)

	err := t.db.QueryRow(ctx, query, args).Scan(&resolvedId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil
		}

		return uuid.Nil, fmt.Errorf("resolve thread dialog: %w", err)
	}

	return resolvedId, nil
}

func (t *threadDialogStore) ThreadMembers(ctx context.Context, threadID, memberID uuid.UUID, domainID int32) (*dto.ThreadMembersResponse, error) {
	query := `
		with thread_members as (
			select array_agg(member_id) as ids
			from (
				select distinct td.member_id
				from im_thread.thread_dialog td
				where (td.domain_id, td.thread_id) = (@DomainID, @ThreadID)
			)
		)
		select
			ids as members
		from thread_members
		where @MemberID = any(ids);
	`

	args := pgx.NamedArgs{
		"DomainID": domainID,
		"ThreadID": threadID,
		"MemberID": memberID,
	}

	rows, err := t.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	members, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[dto.ThreadMembersResponse])
	if err != nil {
		return nil, err
	}

	return members, nil
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
