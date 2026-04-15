package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type threadDialogStore struct {
	db Querier
}

func NewThreadDialogStore(db Querier) *threadDialogStore {
	return &threadDialogStore{
		db: db,
	}
}

type threadDialog struct {
	ID        uuid.UUID `db:"id"`
	DomainID  int
	CreatedAt time.Time
	UpdatedAt time.Time
	MemberID  uuid.UUID        `db:"member_id"`
	ThreadID  uuid.UUID        `db:"thread_id"`
	Role      model.ThreadRole `db:"thread_role"`

	DirectTo                    *uuid.UUID `db:"direct_to,omitempty"`
	MemberOf                    *uuid.UUID `db:"member_of,omitempty"`
	CanSendMessages             bool       `db:"can_send_messages"`
	CanAddMembers               bool       `db:"can_add_members"`
	CanChangeMembersPermissions bool       `db:"can_change_members_permissions"`
	CanRemoveMembers            bool       `db:"can_remove_members"`
	CanChangeThreadInfo         bool       `db:"can_change_thread_info"`

	Title string `db:"title"`
}

func (t *threadDialogStore) Create(ctx context.Context, member *model.ThreadDialogExtended) (*model.ThreadDialogExtended, error) {

	if member == nil {
		return nil, errors.New("new member required")
	}

	if member.ThreadID == uuid.Nil {
		return nil, errors.New("threadID cannot be nil")
	}

	if member.MemberID == uuid.Nil {
		return nil, errors.New("newMemberID cannot be nil")
	}

	var (
		query = `
		WITH inserted_dialog AS
		(
			 INSERT INTO im_thread.thread_dialog(domain_id, member_id, thread_id, thread_role, direct_to, member_of)
			(SELECT domain_id, @MemberID, id, @ThreadRole, @DirectTo, @MemberOf FROM im_thread.thread WHERE id = @ThreadID LIMIT 1)
			RETURNING *
		),
		inserted_permissions AS 
		(
			INSERT INTO im_thread.thread_permission(thread_id, thread_dialog_id, can_send_messages, can_add_members, can_change_members_permissions, can_remove_members, can_change_thread_info)
			(SELECT thread_id, id, 
			@CanSendMessages,
			@CanAddMembers,
			@CanChangeMembersPermissions,
			@CanRemoveMembers,
			@CanChangeThreadInfo
		  FROM inserted_dialog)
		  	RETURNING *
		),
		inserted_thread_direct_settings AS (
			INSERT INTO im_thread.direct_settings (thread_dialog_id, domain_id, title)
			(SELECT id, domain_id, @ThreadTitle FROM inserted_dialog)
			RETURNING * 
		)

		SELECT dial.id, dial.domain_id, dial.created_at, dial.updated_at, dial.member_id, dial.thread_id,
		perm.can_send_messages, perm.can_add_members, perm.can_change_members_permissions, perm.can_remove_members, perm.can_change_thread_info,
		sett.title
		FROM inserted_dialog dial
		LEFT JOIN inserted_permissions perm ON dial.id = perm.thread_dialog_id
		LEFT JOIN inserted_thread_direct_settings sett ON dial.id = sett.thread_dialog_id
		`
	)

	rows, err := t.db.Query(ctx, query, pgx.NamedArgs{
		"MemberID":                    member.MemberID,
		"ThreadID":                    member.ThreadID,
		"ThreadRole":                  member.ThreadRole,
		"DirectTo":                    member.DirectTo,
		"MemberOf":                    member.MemberOf,
		"CanSendMessages":             member.Permissions.CanSendMessages,
		"CanAddMembers":               member.Permissions.CanAddMembers,
		"CanChangeMembersPermissions": member.Permissions.CanChangeMembersPermissions,
		"CanRemoveMembers":            member.Permissions.CanRemoveMembers,
		"CanChangeThreadInfo":         member.Permissions.CanChangeThreadInfo,
		"ThreadTitle":                 member.Settings.Title,
	})
	if err != nil {
		return nil, err
	}

	result, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByNameLax[threadDialog])
	if err != nil {
		return nil, err
	}

	return mapToThreadDialogExtendedModel(result)
}

func mapToThreadDialogExtendedModel(dialog *threadDialog) (*model.ThreadDialogExtended, error) {
	if dialog == nil {
		return nil, errors.New("thread dialog cannot be nil")
	}

	return &model.ThreadDialogExtended{
		BaseModel: shared.BaseModel{
			ID:        dialog.ID,
			DomainID:  dialog.DomainID,
			CreatedAt: dialog.CreatedAt,
			UpdatedAt: dialog.UpdatedAt,
		},
		MemberID:   dialog.MemberID,
		ThreadID:   dialog.ThreadID,
		ThreadRole: dialog.Role,

		DirectTo: dialog.DirectTo,
		MemberOf: dialog.MemberOf,
		Permissions: model.ThreadPermissions{
			CanSendMessages:             dialog.CanSendMessages,
			CanAddMembers:               dialog.CanAddMembers,
			CanChangeMembersPermissions: dialog.CanChangeMembersPermissions,
			CanRemoveMembers:            dialog.CanRemoveMembers,
			CanChangeThreadInfo:         dialog.CanChangeThreadInfo,
		},
		Settings: model.BaseThreadSetting{
			Title: dialog.Title,
		},
	}, nil
}

func mapToThreadDialogModel(dialog *threadDialog) (*model.ThreadDialog, error) {
	if dialog == nil {
		return nil, errors.New("thread dialog cannot be nil")
	}

	return &model.ThreadDialog{
		BaseModel: shared.BaseModel{
			ID:        dialog.ID,
			DomainID:  dialog.DomainID,
			CreatedAt: dialog.CreatedAt,
			UpdatedAt: dialog.UpdatedAt,
		},
		ContactID:  dialog.MemberID,
		ThreadID:   dialog.ThreadID,
		ThreadRole: dialog.Role,

		DirectTo: dialog.DirectTo,
		MemberOf: dialog.MemberOf,
	}, nil
}

func (t *threadDialogStore) Delete(ctx context.Context, threadID, memberID uuid.UUID) error {

	if threadID == uuid.Nil {
		return errors.New("threadID cannot be nil")
	}

	if memberID == uuid.Nil {
		return errors.New("newMemberID cannot be nil")
	}

	var (
		query = `DELETE FROM im_thread.thread_dialog
		WHERE thread_id = $1 AND member_id = $2`
	)

	_, err := t.db.Exec(ctx, query, threadID, memberID)
	if err != nil {
		return err
	}

	return nil
}

func (t *threadDialogStore) GetQuickView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialog, error) {
	if filter == nil {
		return nil, errors.New("filter cannot be nil")
	}

	var (
		query = `SELECT
	-- basic thread dialog fields
	 dial.id, dial.domain_id, dial.created_at, dial.updated_at, dial.member_id, dial.thread_id, dial.direct_to, dial.member_of, dial.thread_role


	FROM im_thread.thread_dialog dial
	WHERE (@ThreadIDs::uuid[] IS NULL OR dial.thread_id = ANY(@ThreadIDs))
	AND (@ContactIDs::uuid[] IS NULL OR dial.member_id = ANY(@ContactIDs))
	
	OFFSET @Offset`
		args = pgx.NamedArgs{
			"ThreadIDs":  filter.ContactIDs,
			"ContactIDs": filter.ContactIDs,
			"Offset":     filter.Offset,
		}
	)
	if filter.Limit > 0 {
		query += " LIMIT @Limit"
		args["Limit"] = filter.Limit
	}

	rows, err := t.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return collectRows(rows, mapToThreadDialogModel)
}

func (t *threadDialogStore) GetFullView(ctx context.Context, filter *model.ThreadDialogStoreFilter) ([]*model.ThreadDialogExtended, error) {
	query := `
	SELECT
	-- basic thread dialog fields
	 dial.id, dial.domain_id, dial.created_at, dial.updated_at, dial.member_id, dial.thread_id, dial.direct_to, dial.member_of, dial.thread_role,

	-- permissions fields
	perm.can_send_messages, perm.can_add_members, perm.can_change_members_permissions, perm.can_remove_members, perm.can_change_thread_info,

	-- settings fields
	sett.title

	FROM im_thread.thread_dialog dial
	LEFT JOIN im_thread.thread_permission perm ON perm.thread_dialog_id = dial.id
	LEFT JOIN im_thread.direct_settings sett ON sett.thread_dialog_id = dial.id

	WHERE (@ThreadIDs::uuid[] IS NULL OR dial.thread_id = ANY(@ThreadIDs))
	AND (@ContactIDs::uuid[] IS NULL OR dial.member_id = ANY(@ContactIDs))
	AND (@IDS::uuid[] IS NULL OR dial.id = ANY(@IDS))

	OFFSET @Offset 
	`

	args := pgx.NamedArgs{
		"ThreadIDs":  filter.ThreadIDs,
		"ContactIDs": filter.ContactIDs,
		"Offset":     filter.Offset,
		"IDS":        filter.IDs,
	}

	if filter.Limit > 0 {
		query += " LIMIT @Limit"
		args["Limit"] = filter.Limit
	}

	rows, err := t.db.Query(ctx, query, args)
	if err != nil {
		// return nil, errors.Internal("failed to get full view", errors.WithCause(err))
		return nil, err
	}

	return collectRows(rows, mapToThreadDialogExtendedModel)

}

func (t *threadDialogStore) FindActorsPair(ctx context.Context, initiatorContactID, targetMemberID uuid.UUID) (*model.ThreadDialogExtended, *model.ThreadDialogExtended, error) {
	query := `
	SELECT
	-- basic thread dialog fields
	 dial.id, dial.domain_id, dial.created_at, dial.updated_at, dial.member_id, dial.thread_id, dial.direct_to, dial.member_of, dial.thread_role,

	-- permissions fields
	perm.can_send_messages, perm.can_add_members, perm.can_change_members_permissions, perm.can_remove_members, perm.can_change_thread_info,

	-- settings fields
	sett.title

	FROM im_thread.thread_dialog dial
	LEFT JOIN im_thread.thread_permission perm ON perm.thread_dialog_id = dial.id
	LEFT JOIN im_thread.direct_settings sett ON sett.thread_dialog_id = dial.id

	WHERE dial.thread_id IN (
		SELECT thread_id
		FROM im_thread.thread_dialog
		WHERE id = @TargetMemberID
		LIMIT 1
	)
	 AND (member_id = @InitiatorContactID OR id = @TargetMemberID)
	`

	args := pgx.NamedArgs{
		"InitiatorContactID": initiatorContactID,
		"TargetMemberID":     targetMemberID,
	}

	rows, err := t.db.Query(ctx, query, args)
	if err != nil {
		// return nil, errors.Internal("failed to get full view", errors.WithCause(err))
		return nil, nil, err
	}

	threadDialogs, err := collectRows(rows, mapToThreadDialogExtendedModel)
	if err != nil {
		return nil, nil, err
	}

	var initiatorDialog, targetDialog *model.ThreadDialogExtended
	for _, dialog := range threadDialogs {
		if initiatorDialog != nil && targetDialog != nil {
			break
		}
		if dialog.MemberID == initiatorContactID {
			initiatorDialog = dialog
		} else if dialog.ID == targetMemberID {
			targetDialog = dialog
		}
	}

	return initiatorDialog, targetDialog, nil

}

// CreateDirectPair creates two new thread dialogs for peers within one transaction.
// It returns two newly created thread dialogs or an error if the operation fails.
// If the operation succeeds, it returns two newly created thread dialogs with the id set.
// If the operation fails, it returns nil and the error.
func (t *threadDialogStore) CreateDirectPair(ctx context.Context, dialog *model.ThreadDialogExtended) ([]*model.ThreadDialogExtended, error) {
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
		result = make([]*model.ThreadDialogExtended, 0, 2)
	)

	rows, err := t.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		createdDialog := &model.ThreadDialogExtended{}
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
