package postgres

import (
	"context"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/webitel/im-thread-service/internal/domain/model"
	"github.com/webitel/im-thread-service/internal/domain/shared"
	"github.com/webitel/im-thread-service/internal/utils"
	"github.com/webitel/webitel-go-kit/pkg/errors"
)

type (
	directSettingsRecord struct {
		ID             uuid.UUID  `json:"id,omitempty" db:"id"`
		DomainID       int        `json:"domain_id,omitempty" db:"domain_id"`
		ThreadDialogID uuid.UUID  `json:"thread_dialog_id,omitempty" db:"thread_dialog_id"`
		CreatedAt      time.Time  `json:"created_at,omitempty" db:"created_at"`
		UpdatedAt      time.Time  `json:"updated_at,omitempty" db:"updated_at"`
		Title          string     `json:"title,omitempty" db:"title"`
		MemberID       *uuid.UUID `json:"member_id,omitempty" db:"member_id"`
	}

	threadDialogRecord struct {
		ID        uuid.UUID             `json:"id,omitempty" db:"id"`
		DomainID  int                   `json:"domain_id,omitempty" db:"domain_id"`
		CreatedAt time.Time             `json:"created_at" db:"created_at"`
		UpdatedAt time.Time             `json:"updated_at" db:"updated_at"`
		MemberID  uuid.UUID             `json:"member_id,omitempty" db:"member_id"`
		ThreadID  uuid.UUID             `json:"thread_id,omitempty" db:"thread_id"`
		DirectTo  uuid.UUID             `json:"direct_to,omitempty" db:"direct_to"`
		Settings  *directSettingsRecord `json:"settings" db:"settings"`
	}
)

type (
	directThreadDialogOrchestration struct {
		db Querier
	}
)

func NewDirectThreadDialogOrchestration(db Querier) *directThreadDialogOrchestration {
	return &directThreadDialogOrchestration{
		db: db,
	}
}
func (d *directThreadDialogOrchestration) InitializeFullDirectThread(ctx context.Context, req *model.CreateDirectThreadDialogRequest) ([]*model.DirectThreadDialog, error) {
	if req == nil {
		return nil, errors.InvalidArgument("request cannot be nil")
	}
	query := `
		with inserted_dialogs as (
			insert into im_thread.thread_dialog (
				domain_id, created_at, updated_at,
				member_id, thread_id, direct_to
			)
			values
				(@DomainID, now(), now(), @From, @ThreadID, @To),
				(@DomainID, now(), now(), @To, @ThreadID, @From)
			returning
				id, domain_id, created_at, updated_at,
				member_id, thread_id, direct_to
		),
		inserted_settings as (
			insert into im_thread.direct_settings (
				domain_id, thread_dialog_id, created_at, updated_at, title
			)
			select
				d.domain_id,
				d.id,
				now(),
				now(),
				case
					when d.member_id = @From then @TitleFrom
					else @TitleTo
				end as title
			from inserted_dialogs d
			returning id, domain_id, thread_dialog_id, created_at, updated_at, title
		)
		select
			d.id,
			d.domain_id,
			d.created_at,
			d.updated_at,
			d.member_id,
			d.thread_id,
			d.direct_to,
			jsonb_build_object(
				'id', s.id,
				'domain_id', s.domain_id,
				'thread_dialog_id', s.thread_dialog_id,
				'created_at', s.created_at,
				'updated_at', s.updated_at,
				'title', s.title
			) as settings
		from inserted_dialogs d
		inner join inserted_settings s on d.id = s.thread_dialog_id
	`

	args := pgx.NamedArgs{
		"DomainID":  req.DomainID,
		"From":      req.From.ID,
		"To":        req.To.ID,
		"ThreadID":  req.ThreadID,
		"TitleFrom": req.From.Settings.Title,
		"TitleTo":   req.To.Settings.Title,
	}

	rows, err := d.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}

	records, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[threadDialogRecord])
	if err != nil {
		return nil, err
	}

	result := utils.Map(records, func(tdr *threadDialogRecord) *model.DirectThreadDialog {
		return mapThreadDialogRecordToModel(tdr)
	})

	return result, nil
}

func mapThreadDialogRecordToModel(record *threadDialogRecord) *model.DirectThreadDialog {
	threadDialog := &model.ThreadDialogExtended{
		BaseModel: shared.BaseModel{
			ID:        record.ID,
			DomainID:  record.DomainID,
			CreatedAt: record.CreatedAt,
			UpdatedAt: record.UpdatedAt,
		},
		MemberID: record.MemberID,
		ThreadID: record.ThreadID,
		DirectTo: &record.DirectTo,
	}

	directThreadDialog := &model.DirectThreadDialog{
		ThreadDialogExtended: *threadDialog,
		Settings:             mapDirectSettingsRecordToModel(record.Settings),
	}

	return directThreadDialog
}

func mapDirectSettingsRecordToModel(record *directSettingsRecord) *model.DirectThreadSetting {
	return &model.DirectThreadSetting{
		BaseThreadSetting: model.BaseThreadSetting{
			BaseModel: shared.BaseModel{
				ID:        record.ID,
				DomainID:  record.DomainID,
				CreatedAt: record.CreatedAt,
				UpdatedAt: record.UpdatedAt,
			},
			ThreadDialogID: record.ThreadDialogID,
			Title:          record.Title,
		},
	}
}

func (d *directThreadDialogOrchestration) InitializePermissions(ctx context.Context, threadDialogID uuid.UUID, permissions model.ThreadPermissions) (*model.ThreadPermissions, error) {
	query := `
		INSERT INTO im_thread.thread_dialog_permissions (
			thread_id,
			thread_dialog_id,
			can_send_messages,
			can_add_members,
			can_change_members_permissions,
			can_remove_members,
			can_change_thread_info 
		)
		VALUES (
			@ThreadID,
			@ThreadDialogID,
			@CanSendMessages,
			@CanAddMembers,
			@CanChangeMembersPermissions,
			@CanRemoveMembers,
			@CanChangeThreadInfo
		)
		RETURNING 
		id,
		thread_id,
		thread_dialog_id,
		can_send_messages,
		can_add_members,
		can_change_members_permissions,
		can_remove_members,
		can_change_thread_info,
		created_at,
		updated_at
	`
	args := pgx.NamedArgs{
		"ThreadDialogID":              threadDialogID,
		"CanSendMessages":             permissions.CanSendMessages,
		"CanAddMembers":               permissions.CanAddMembers,
		"CanChangeMembersPermissions": permissions.CanChangeMembersPermissions,
		"CanRemoveMembers":            permissions.CanRemoveMembers,
		"CanChangeThreadInfo":         permissions.CanChangeThreadInfo,
	}

	rows, err := d.db.Query(ctx, query, args)
	if err != nil {
		return nil, err
	}
	var perm model.ThreadPermissions
	err = pgxscan.ScanOne(&perm, rows)
	if err != nil {
		return nil, err
	}

	return &perm, nil

}
